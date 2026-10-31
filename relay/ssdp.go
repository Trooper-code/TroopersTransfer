package relay

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/huin/goupnp/dcps/internetgateway2"
	"golang.org/x/net/ipv4"
)

// Our own SSDP discovery. The UPnP library's search binds to each local
// address and sends to the multicast group, but on Windows the packet then
// leaves through the OS's default multicast adapter, which on a PC with WSL,
// Hyper-V or a VPN is often a virtual adapter that never reaches the router.
// Windows Firewall can also drop replies to multicast. So:
//
//  1. ask the router directly (unicast to gateway:1900): it can't take the
//     wrong adapter, and the firewall allows the reply because it answers a
//     request we sent to that exact address;
//  2. multicast with the outgoing adapter pinned to the one that reaches the
//     router;
//  3. use several search targets, because some ISP gateways only answer the
//     generic ones.

const ssdpGroup = "239.255.255.250:1900"

// ssdpPort is where routers listen for SSDP. A variable so tests can point
// the direct search at a fake router.
var ssdpPort = "1900"

var searchTargets = []string{
	"urn:schemas-upnp-org:device:InternetGatewayDevice:1",
	"urn:schemas-upnp-org:device:InternetGatewayDevice:2",
	"urn:schemas-upnp-org:service:WANIPConnection:1",
	"urn:schemas-upnp-org:service:WANIPConnection:2",
	"urn:schemas-upnp-org:service:WANPPPConnection:1",
	"upnp:rootdevice",
	"ssdp:all",
}

// ssdpReport counts what discovery saw, for the technical details.
type ssdpReport struct {
	unicastReplies   int
	multicastReplies int
	locations        []string
	notes            []string
}

func (r *ssdpReport) String() string {
	s := fmt.Sprintf("direct replies from router: %d, multicast replies: %d", r.unicastReplies, r.multicastReplies)
	if len(r.locations) > 0 {
		s += "; devices: " + strings.Join(r.locations, ", ")
	}
	if len(r.notes) > 0 {
		s += "; " + strings.Join(r.notes, "; ")
	}
	return s
}

// discoverIGDDirect finds the router's port-mapping service using unicast and
// pinned multicast SSDP. localIP is this PC's address on the router's
// network; routerIP is the default gateway.
func discoverIGDDirect(ctx context.Context, localIP, routerIP string) (igdClient, *ssdpReport, error) {
	rep := &ssdpReport{}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	// Each new device description is checked as soon as it's heard, and the
	// first one with a port-mapping service ends the search. A device on the
	// router's own address wins over anything else that answered.
	var (
		mu        sync.Mutex
		seen      = map[string]bool{}
		checks    sync.WaitGroup
		fromOther igdClient
		winner    = make(chan igdClient, 1)
	)
	add := func(loc string, unicast bool) {
		mu.Lock()
		defer mu.Unlock()
		if unicast {
			rep.unicastReplies++
		} else {
			rep.multicastReplies++
		}
		if loc == "" || seen[loc] {
			return
		}
		seen[loc] = true
		rep.locations = append(rep.locations, loc)
		u, err := url.Parse(loc)
		if err != nil {
			return
		}
		onRouter := routerIP != "" && u.Hostname() == routerIP
		checks.Add(1)
		go func() {
			defer checks.Done()
			c, err := clientFromLocation(ctx, u)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				rep.notes = append(rep.notes, fmt.Sprintf("%s: %v", loc, err))
			case onRouter:
				select {
				case winner <- c:
					cancel()
				default:
				}
			case fromOther == nil:
				fromOther = c
			}
		}()
	}

	var searches sync.WaitGroup
	if routerIP != "" {
		searches.Add(1)
		go func() {
			defer searches.Done()
			if err := ssdpSearch(ctx, localIP, net.JoinHostPort(routerIP, ssdpPort), nil, func(l string) { add(l, true) }); err != nil {
				mu.Lock()
				rep.notes = append(rep.notes, "direct search: "+err.Error())
				mu.Unlock()
			}
		}()
	}
	if localIP != "" {
		searches.Add(1)
		go func() {
			defer searches.Done()
			ifi := interfaceFor(localIP)
			if err := ssdpSearch(ctx, localIP, ssdpGroup, ifi, func(l string) { add(l, false) }); err != nil {
				mu.Lock()
				rep.notes = append(rep.notes, "multicast search: "+err.Error())
				mu.Unlock()
			}
		}()
	}
	searches.Wait()
	checks.Wait()

	select {
	case c := <-winner:
		return c, rep, nil
	default:
	}
	mu.Lock()
	defer mu.Unlock()
	if fromOther != nil {
		return fromOther, rep, nil
	}
	if len(rep.locations) == 0 {
		return nil, rep, errors.New("no UPnP device answered")
	}
	return nil, rep, errors.New("UPnP devices answered, but none offers port mapping")
}

// clientFromLocation reads a device description and returns its
// port-mapping service, newest version first.
func clientFromLocation(ctx context.Context, u *url.URL) (igdClient, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if cs, err := internetgateway2.NewWANIPConnection2ClientsByURLCtx(ctx, u); err == nil && len(cs) > 0 {
		return cs[0], nil
	}
	if cs, err := internetgateway2.NewWANIPConnection1ClientsByURLCtx(ctx, u); err == nil && len(cs) > 0 {
		return cs[0], nil
	}
	cs, err := internetgateway2.NewWANPPPConnection1ClientsByURLCtx(ctx, u)
	if err == nil && len(cs) > 0 {
		return cs[0], nil
	}
	if err == nil {
		err = errors.New("no WANIPConnection or WANPPPConnection service")
	}
	return nil, err
}

// interfaceFor finds the network adapter that owns ip.
func interfaceFor(ip string) *net.Interface {
	target := net.ParseIP(ip)
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for i := range ifaces {
		addrs, err := ifaces[i].Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.Equal(target) {
				return &ifaces[i]
			}
		}
	}
	return nil
}

// ssdpSearch sends M-SEARCH for every target to dst (the multicast group or
// the router directly) a couple of times and reports each LOCATION it hears
// until ctx ends. ifi pins the outgoing adapter for multicast.
func ssdpSearch(ctx context.Context, localIP, dst string, ifi *net.Interface, found func(location string)) error {
	bind := ":0"
	if localIP != "" {
		bind = net.JoinHostPort(localIP, "0")
	}
	conn, err := net.ListenPacket("udp4", bind)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	addr, err := net.ResolveUDPAddr("udp4", dst)
	if err != nil {
		return err
	}
	if addr.IP.IsMulticast() {
		p := ipv4.NewPacketConn(conn)
		_ = p.SetMulticastTTL(2)
		_ = p.SetMulticastLoopback(false)
		if ifi != nil {
			if err := p.SetMulticastInterface(ifi); err != nil {
				return fmt.Errorf("pin adapter %s: %w", ifi.Name, err)
			}
		}
	}

	host := dst
	if addr.IP.IsMulticast() {
		host = ssdpGroup
	}
	send := func() {
		for _, st := range searchTargets {
			msg := "M-SEARCH * HTTP/1.1\r\n" +
				"HOST: " + host + "\r\n" +
				"MAN: \"ssdp:discover\"\r\n" +
				"MX: 2\r\n" +
				"ST: " + st + "\r\n" +
				"USER-AGENT: Windows/10 UPnP/1.1 TroopersTransfer/1\r\n\r\n"
			_, _ = conn.WriteTo([]byte(msg), addr)
		}
	}

	go func() {
		send()
		for range 2 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(700 * time.Millisecond):
				send()
			}
		}
	}()

	buf := make([]byte, 4096)
	var sendErr error
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return sendErr
			}
			// On Windows an ICMP "port unreachable" from the router shows up
			// as a read error; keep listening for the other replies.
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			if strings.Contains(strings.ToLower(err.Error()), "forcibly closed") ||
				strings.Contains(strings.ToLower(err.Error()), "connection refused") {
				sendErr = errors.New("router refused the request on port 1900")
				continue
			}
			return err
		}
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(buf[:n])), nil)
		if err != nil {
			continue
		}
		resp.Body.Close()
		found(resp.Header.Get("Location"))
	}
}
