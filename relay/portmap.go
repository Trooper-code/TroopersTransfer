package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/huin/goupnp/dcps/internetgateway2"
	"github.com/jackpal/gateway"
	natpmp "github.com/jackpal/go-nat-pmp"
)

// leaseSeconds is how long each router mapping lasts before it must be
// renewed. Keeping it short means a crash never leaves ports open for long.
const leaseSeconds = 3600

// renewEvery is how often active mappings are refreshed.
const renewEvery = 20 * time.Minute

// MapMethod says how the ports were opened.
type MapMethod string

const (
	MethodUPnP   MapMethod = "UPnP"
	MethodNATPMP MapMethod = "NAT-PMP"
)

// igdClient is the subset of the UPnP WAN connection services we use. All
// three generated clients (WANIPConnection1/2, WANPPPConnection1) satisfy it.
type igdClient interface {
	AddPortMappingCtx(ctx context.Context, remoteHost string, externalPort uint16, protocol string,
		internalPort uint16, internalClient string, enabled bool, description string, leaseDuration uint32) error
	DeletePortMappingCtx(ctx context.Context, remoteHost string, externalPort uint16, protocol string) error
	GetExternalIPAddressCtx(ctx context.Context) (string, error)
}

// mapper holds an active set of router port mappings and keeps them alive.
type mapper struct {
	method     MapMethod
	ports      []int
	localIP    string
	externalIP string

	igd igdClient
	pmp *natpmp.Client

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// mapError records why automatic port opening failed, so the caller can
// explain it in plain English.
type mapError struct {
	noRouterAnswer bool   // neither UPnP nor NAT-PMP answered
	refused        bool   // a router answered but would not add the mapping
	conflict       bool   // refused because another device already uses the port
	port           int    // the port that failed, when known
	externalIP     string // what the router reported, if it answered
	err            error
}

func (e *mapError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return "port mapping failed"
}
func (e *mapError) Unwrap() error { return e.err }

// openPorts asks the router to forward each TCP port to localIP. It tries
// UPnP first and falls back to NAT-PMP (Apple routers and some others).
func openPorts(ctx context.Context, ports []int, localIP, routerIP string) (*mapper, error) {
	var upnpErr error
	if m, err := openUPnP(ctx, ports, localIP, routerIP); err == nil {
		m.startRenew()
		return m, nil
	} else {
		upnpErr = err
		var me *mapError
		if errors.As(err, &me) && !me.noRouterAnswer {
			// UPnP is there but said no. NAT-PMP is very unlikely to do
			// better, and its answer would only hide the real reason.
			return nil, err
		}
	}
	if m, err := openNATPMP(ports); err == nil {
		m.startRenew()
		return m, nil
	} else {
		var me *mapError
		if errors.As(err, &me) && !me.noRouterAnswer {
			return nil, err
		}
	}
	return nil, &mapError{noRouterAnswer: true, err: fmt.Errorf("no UPnP or NAT-PMP router answered: %w", upnpErr)}
}

// discoverTimeout bounds each UPnP search. A search always listens for its
// full 2-second window and then fetches the router's description, so each
// one gets its own generous deadline.
const discoverTimeout = 8 * time.Second

// discoverIGD looks for the router's port-mapping service. Routers offer one
// of three service types (most ISP gateways only the older WANIPConnection1),
// so all three searches run at the same time and the newest one found wins.
func discoverIGD(ctx context.Context) (igdClient, error) {
	type result struct {
		rank   int // lower is preferred
		client igdClient
		err    error
	}
	searches := []func(context.Context) (igdClient, error){
		func(ctx context.Context) (igdClient, error) {
			cs, _, err := internetgateway2.NewWANIPConnection2ClientsCtx(ctx)
			if err == nil && len(cs) > 0 {
				return cs[0], nil
			}
			return nil, searchErr("WANIPConnection2", err)
		},
		func(ctx context.Context) (igdClient, error) {
			cs, _, err := internetgateway2.NewWANIPConnection1ClientsCtx(ctx)
			if err == nil && len(cs) > 0 {
				return cs[0], nil
			}
			return nil, searchErr("WANIPConnection1", err)
		},
		func(ctx context.Context) (igdClient, error) {
			cs, _, err := internetgateway2.NewWANPPPConnection1ClientsCtx(ctx)
			if err == nil && len(cs) > 0 {
				return cs[0], nil
			}
			return nil, searchErr("WANPPPConnection1", err)
		},
	}
	results := make(chan result, len(searches))
	for i, search := range searches {
		go func() {
			sctx, cancel := context.WithTimeout(ctx, discoverTimeout)
			defer cancel()
			c, err := search(sctx)
			results <- result{rank: i, client: c, err: err}
		}()
	}
	var best *result
	var errs []string
	for range searches {
		r := <-results
		if r.client == nil {
			errs = append(errs, r.err.Error())
			continue
		}
		if best == nil || r.rank < best.rank {
			best = &r
		}
	}
	if best != nil {
		return best.client, nil
	}
	return nil, fmt.Errorf("no UPnP internet gateway found on this network (%s)", strings.Join(errs, "; "))
}

func searchErr(service string, err error) error {
	if err == nil {
		return fmt.Errorf("%s: no answer", service)
	}
	return fmt.Errorf("%s: %v", service, err)
}

// findIGD runs the library's search and our direct search at the same time
// and uses whichever finds the router.
func findIGD(ctx context.Context, localIP, routerIP string) (igdClient, error) {
	type res struct {
		c   igdClient
		err error
	}
	lib := make(chan res, 1)
	direct := make(chan res, 1)
	go func() {
		c, err := discoverIGD(ctx)
		lib <- res{c, err}
	}()
	go func() {
		c, rep, err := discoverIGDDirect(ctx, localIP, routerIP)
		if err != nil {
			err = fmt.Errorf("%v (%s)", err, rep)
		}
		direct <- res{c, err}
	}()
	var errs []string
	for range 2 {
		var r res
		select {
		case r = <-lib:
		case r = <-direct:
		}
		if r.c != nil {
			return r.c, nil
		}
		errs = append(errs, r.err.Error())
	}
	return nil, errors.New(strings.Join(errs, "\n"))
}

func openUPnP(ctx context.Context, ports []int, localIP, routerIP string) (*mapper, error) {
	c, err := findIGD(ctx, localIP, routerIP)
	if err != nil {
		return nil, &mapError{noRouterAnswer: true, err: err}
	}
	m := &mapper{method: MethodUPnP, ports: ports, localIP: localIP, igd: c}
	if ip, err := c.GetExternalIPAddressCtx(ctx); err == nil {
		m.externalIP = strings.TrimSpace(ip)
	}
	for i, p := range ports {
		if err := addUPnP(ctx, c, p, localIP); err != nil {
			// Undo the ones that worked so we don't leave half a set open.
			for _, q := range ports[:i] {
				_ = c.DeletePortMappingCtx(context.Background(), "", uint16(q), "TCP")
			}
			return nil, &mapError{
				refused:    true,
				conflict:   isConflict(err),
				port:       p,
				externalIP: m.externalIP,
				err:        fmt.Errorf("router refused to open port %d: %w", p, err),
			}
		}
	}
	return m, nil
}

func addUPnP(ctx context.Context, c igdClient, port int, localIP string) error {
	desc := fmt.Sprintf("Troopers Transfer relay %d", port)
	err := c.AddPortMappingCtx(ctx, "", uint16(port), "TCP", uint16(port), localIP, true, desc, leaseSeconds)
	if err != nil && strings.Contains(err.Error(), "725") {
		// 725 OnlyPermanentLeasesSupported: some routers reject timed leases.
		err = c.AddPortMappingCtx(ctx, "", uint16(port), "TCP", uint16(port), localIP, true, desc, 0)
	}
	return err
}

// isConflict reports UPnP error 718 (ConflictInMappingEntry): another device
// on the network already has that port forwarded to it.
func isConflict(err error) bool {
	s := err.Error()
	return strings.Contains(s, "718") || strings.Contains(strings.ToLower(s), "conflict")
}

func openNATPMP(ports []int) (*mapper, error) {
	gw, err := gateway.DiscoverGateway()
	if err != nil {
		return nil, &mapError{noRouterAnswer: true, err: err}
	}
	c := natpmp.NewClientWithTimeout(gw, 3*time.Second)
	ext, err := c.GetExternalAddress()
	if err != nil {
		return nil, &mapError{noRouterAnswer: true, err: fmt.Errorf("NAT-PMP: %w", err)}
	}
	m := &mapper{method: MethodNATPMP, ports: ports, pmp: c}
	m.externalIP = net.IP(ext.ExternalIPAddress[:]).String()
	for i, p := range ports {
		res, err := c.AddPortMapping("tcp", p, p, leaseSeconds)
		if err == nil && int(res.MappedExternalPort) != p {
			// The router gave us a different outside port; friends would
			// not find the relay on the expected one.
			_, _ = c.AddPortMapping("tcp", p, 0, 0)
			err = fmt.Errorf("router assigned outside port %d instead of %d", res.MappedExternalPort, p)
		}
		if err != nil {
			for _, q := range ports[:i] {
				_, _ = c.AddPortMapping("tcp", q, 0, 0)
			}
			return nil, &mapError{refused: true, port: p, externalIP: m.externalIP,
				err: fmt.Errorf("router refused to open port %d: %w", p, err)}
		}
	}
	return m, nil
}

func (m *mapper) startRenew() {
	m.stop = make(chan struct{})
	m.done = make(chan struct{})
	go func() {
		defer close(m.done)
		t := time.NewTicker(renewEvery)
		defer t.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-t.C:
				m.renew()
			}
		}
	}()
}

func (m *mapper) renew() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, p := range m.ports {
		switch m.method {
		case MethodUPnP:
			_ = addUPnP(ctx, m.igd, p, m.localIP)
		case MethodNATPMP:
			_, _ = m.pmp.AddPortMapping("tcp", p, p, leaseSeconds)
		}
	}
}

// close removes the mappings from the router. Safe to call more than once.
func (m *mapper) close() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		if m.stop != nil {
			close(m.stop)
			<-m.done
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, p := range m.ports {
			switch m.method {
			case MethodUPnP:
				_ = m.igd.DeletePortMappingCtx(ctx, "", uint16(p), "TCP")
			case MethodNATPMP:
				_, _ = m.pmp.AddPortMapping("tcp", p, 0, 0)
			}
		}
	})
}
