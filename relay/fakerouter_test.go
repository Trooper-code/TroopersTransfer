package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRouter is a minimal UPnP internet gateway: it answers SSDP searches
// sent straight to it (never multicast, like routers whose replies get lost
// on Windows) and handles the WANIPConnection:1 actions we use.
type fakeRouter struct {
	t        *testing.T
	ssdp     net.PacketConn
	http     *httptest.Server
	conflict int // AddPortMapping on this port fails with 718

	mu       sync.Mutex
	mappings map[string]string // "port" -> internal client
	searches int
}

func newFakeRouter(t *testing.T) *fakeRouter {
	t.Helper()
	r := &fakeRouter{t: t, mappings: map[string]string{}}
	r.http = httptest.NewServer(http.HandlerFunc(r.serveHTTP))
	t.Cleanup(r.http.Close)

	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r.ssdp = pc
	t.Cleanup(func() { pc.Close() })
	go r.serveSSDP()

	_, port, _ := net.SplitHostPort(pc.LocalAddr().String())
	old := ssdpPort
	ssdpPort = port
	t.Cleanup(func() { ssdpPort = old })
	return r
}

func (r *fakeRouter) serveSSDP() {
	buf := make([]byte, 2048)
	for {
		n, from, err := r.ssdp.ReadFrom(buf)
		if err != nil {
			return
		}
		req := string(buf[:n])
		if !strings.HasPrefix(req, "M-SEARCH") {
			continue
		}
		// Like many ISP gateways: only answer the generic device search.
		if !strings.Contains(req, "ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1") {
			continue
		}
		r.mu.Lock()
		r.searches++
		r.mu.Unlock()
		resp := "HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\nEXT:\r\n" +
			"LOCATION: " + r.http.URL + "/rootDesc.xml\r\n" +
			"SERVER: FakeOS/1.0 UPnP/1.0 FakeIGD/1.0\r\n" +
			"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n" +
			"USN: uuid:fake-igd::urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"
		_, _ = r.ssdp.WriteTo([]byte(resp), from)
	}
}

const fakeRootDesc = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
<specVersion><major>1</major><minor>0</minor></specVersion>
<device>
<deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
<friendlyName>Fake gateway</friendlyName><UDN>uuid:fake-igd</UDN>
<deviceList><device>
<deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType><UDN>uuid:fake-wan</UDN>
<deviceList><device>
<deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType><UDN>uuid:fake-conn</UDN>
<serviceList><service>
<serviceType>urn:schemas-upnp-org:service:WANIPConnection:1</serviceType>
<serviceId>urn:upnp-org:serviceId:WANIPConn1</serviceId>
<controlURL>/ctl</controlURL><eventSubURL>/evt</eventSubURL><SCPDURL>/scpd.xml</SCPDURL>
</service></serviceList>
</device></deviceList>
</device></deviceList>
</device>
</root>`

var tagRe = func(tag string) *regexp.Regexp { return regexp.MustCompile("<" + tag + ">([^<]*)</" + tag + ">") }

func (r *fakeRouter) serveHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/rootDesc.xml":
		w.Header().Set("Content-Type", "text/xml")
		io.WriteString(w, fakeRootDesc)
		return
	case "/ctl":
	default:
		http.NotFound(w, req)
		return
	}
	body, _ := io.ReadAll(req.Body)
	action := req.Header.Get("SOAPAction")
	field := func(tag string) string {
		if m := tagRe(tag).FindSubmatch(body); m != nil {
			return string(m[1])
		}
		return ""
	}
	reply := func(inner string) {
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>%s</s:Body></s:Envelope>`, inner)
	}
	const ns = `xmlns:u="urn:schemas-upnp-org:service:WANIPConnection:1"`
	switch {
	case strings.Contains(action, "GetExternalIPAddress"):
		reply(`<u:GetExternalIPAddressResponse ` + ns + `><NewExternalIPAddress>203.0.113.5</NewExternalIPAddress></u:GetExternalIPAddressResponse>`)
	case strings.Contains(action, "AddPortMapping"):
		port := field("NewExternalPort")
		if fmt.Sprint(r.conflict) == port {
			w.WriteHeader(http.StatusInternalServerError)
			reply(`<s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>718</errorCode><errorDescription>ConflictInMappingEntry</errorDescription></UPnPError></detail></s:Fault>`)
			return
		}
		r.mu.Lock()
		r.mappings[port] = field("NewInternalClient")
		r.mu.Unlock()
		reply(`<u:AddPortMappingResponse ` + ns + `/>`)
	case strings.Contains(action, "DeletePortMapping"):
		r.mu.Lock()
		delete(r.mappings, field("NewExternalPort"))
		r.mu.Unlock()
		reply(`<u:DeletePortMappingResponse ` + ns + `/>`)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (r *fakeRouter) mapped() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for k, v := range r.mappings {
		out[k] = v
	}
	return out
}

// TestDirectDiscoveryOpensPorts covers the case from a real ARRIS gateway on
// Windows: the router never answers multicast, only a search sent straight
// to it, and only for the generic InternetGatewayDevice:1 target.
func TestDirectDiscoveryOpensPorts(t *testing.T) {
	r := newFakeRouter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	start := time.Now()
	m, err := openPorts(ctx, Ports(9009), "127.0.0.1", "127.0.0.1")
	if err != nil {
		t.Fatalf("openPorts: %v", err)
	}
	t.Logf("found the router and opened the ports in %v", time.Since(start).Round(time.Millisecond))
	if m.method != MethodUPnP || m.externalIP != "203.0.113.5" {
		t.Fatalf("method=%s externalIP=%q", m.method, m.externalIP)
	}
	got := r.mapped()
	for _, p := range []string{"9009", "9010", "9011", "9012", "9013"} {
		if got[p] != "127.0.0.1" {
			t.Fatalf("port %s not forwarded to this PC: %v", p, got)
		}
	}
	m.close()
	if left := r.mapped(); len(left) != 0 {
		t.Fatalf("ports still forwarded after close: %v", left)
	}
}

func TestDirectDiscoveryConflictRollsBack(t *testing.T) {
	r := newFakeRouter(t)
	r.conflict = 9011
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, err := openPorts(ctx, Ports(9009), "127.0.0.1", "127.0.0.1")
	var me *mapError
	if !errors.As(err, &me) || !me.conflict || me.port != 9011 {
		t.Fatalf("want a conflict on 9011, got %v", err)
	}
	if left := r.mapped(); len(left) != 0 {
		t.Fatalf("ports 9009-9010 should have been removed again: %v", left)
	}
	p := mapProblem(netFacts{localIP: "127.0.0.1", ports: Ports(9009)}, err, "203.0.113.5")
	if p.Kind != ProblemConflict {
		t.Fatalf("problem = %v, want conflict", p.Kind)
	}
}

func TestNoRouterReportsWhatDiscoverySaw(t *testing.T) {
	old := ssdpPort
	ssdpPort = "9" // discard port: nothing answers
	defer func() { ssdpPort = old }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := openPorts(ctx, Ports(9009), "127.0.0.1", "127.0.0.1")
	var me *mapError
	if !errors.As(err, &me) || !me.noRouterAnswer {
		t.Fatalf("want no-router error, got %v", err)
	}
	if !strings.Contains(err.Error(), "direct replies from router: 0") {
		t.Fatalf("details should say what the direct search saw: %v", err)
	}
}
