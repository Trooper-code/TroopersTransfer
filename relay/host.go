// Package relay lets the app host its own croc relay for friends: it runs
// the relay server in-process, asks the router to open the ports (UPnP or
// NAT-PMP), and explains in plain English what to do when that fails.
package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Options configure a hosted relay.
type Options struct {
	Port     int    // first of the PortCount consecutive ports
	Password string // friends need this to use the relay
	AutoOpen bool   // try UPnP / NAT-PMP to open the router ports
}

// Status describes a running relay and whether the internet can reach it.
type Status struct {
	Port     int
	Password string
	LocalIP  string // this PC on the home network
	PublicIP string // what the internet sees; "" if unknown
	RouterIP string

	// Opened is true when the router accepted the port mappings.
	Opened bool
	Method MapMethod
	// Problem explains why friends outside this network may not get
	// through. nil means it looks good (or AutoOpen was off and nothing
	// was checked, see Checked).
	Problem *Problem
	// Checked is false when AutoOpen was off: we don't know whether the
	// user forwarded the ports by hand.
	Checked bool
}

// LocalCode is the connection code for friends on the same network.
func (s Status) LocalCode() string {
	if s.LocalIP == "" {
		return ""
	}
	return FormatCode(s.LocalIP, s.Port, s.Password)
}

// InternetCode is the connection code for friends elsewhere. Empty when the
// public address is unknown.
func (s Status) InternetCode() string {
	if s.PublicIP == "" {
		return ""
	}
	return FormatCode(s.PublicIP, s.Port, s.Password)
}

// OwnAddress is the address this PC uses for its own transfers through the
// relay it hosts. Going through 127.0.0.1 avoids needing the router to
// support "hairpin" connections back to itself.
func (s Status) OwnAddress() string {
	return net.JoinHostPort("127.0.0.1", fmt.Sprint(s.Port))
}

// Host is a running relay. Create one with Start.
type Host struct {
	opts Options

	mu     sync.Mutex
	server *server
	mapper *mapper
	status Status
}

// StartError is returned when the relay can't run at all on this PC.
type StartError struct {
	Problem *Problem
	Err     error
}

func (e *StartError) Error() string { return e.Problem.Summary }
func (e *StartError) Unwrap() error { return e.Err }

// Start runs the relay and, if asked, opens the router ports. The relay keeps
// running even when the ports can't be opened (friends on the same network
// can still use it); Status.Problem says what went wrong.
func Start(ctx context.Context, opts Options) (*Host, error) {
	if !ValidPort(opts.Port) {
		return nil, fmt.Errorf("choose a starting port between 1024 and %d", 65535-PortCount+1)
	}
	if opts.Password == "" {
		opts.Password = NewPassword()
	}
	srv, err := startServer(opts.Port, opts.Password)
	if err != nil {
		port := opts.Port
		var pe *portError
		if errors.As(err, &pe) {
			port = pe.port
		}
		return nil, &StartError{Problem: problemPortInUse(port, err.Error()), Err: err}
	}
	h := &Host{opts: opts, server: srv}
	h.status = Status{Port: opts.Port, Password: opts.Password}
	h.check(ctx)
	return h, nil
}

// Status returns the latest status.
func (h *Host) Status() Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// Retry repeats the router setup and checks, for example after the user has
// forwarded the ports by hand or switched UPnP on.
func (h *Host) Retry(ctx context.Context) Status {
	h.mu.Lock()
	m := h.mapper
	h.mapper = nil
	h.mu.Unlock()
	m.close()
	h.check(ctx)
	return h.Status()
}

// check gathers network facts, tries to open the ports, and records a
// plain-English Problem if friends outside the network likely can't connect.
func (h *Host) check(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	st := Status{Port: h.opts.Port, Password: h.opts.Password}
	st.LocalIP, _ = LocalIP()
	st.RouterIP = RouterIP()

	pubCh := make(chan string, 1)
	go func() {
		ip, _ := PublicIP(ctx)
		pubCh <- ip
	}()

	facts := netFacts{localIP: st.LocalIP, routerIP: st.RouterIP, ports: Ports(h.opts.Port)}
	if facts.localIP == "" {
		facts.localIP = "this PC's IP address (run ipconfig to find it)"
	}

	var m *mapper
	var mapErr error
	if h.opts.AutoOpen && st.LocalIP != "" {
		m, mapErr = openPorts(ctx, facts.ports, st.LocalIP)
	}
	st.PublicIP = <-pubCh

	if h.opts.AutoOpen {
		st.Checked = true
		switch {
		case m != nil:
			st.Opened, st.Method = true, m.method
			st.Problem = addressProblem(facts, m.externalIP, st.PublicIP)
			if st.PublicIP == "" && st.Problem == nil {
				// The lookup services were unreachable, but the router's
				// own answer is a real public address, so use that.
				st.PublicIP = m.externalIP
			}
		default:
			st.Problem = mapProblem(facts, mapErr, st.PublicIP)
		}
	}

	h.mu.Lock()
	h.mapper = m
	h.status = st
	h.mu.Unlock()
}

// addressProblem checks the router's idea of its outside address against
// what the internet sees, catching carrier-grade NAT, double routers and VPNs
// that make opened ports useless.
func addressProblem(f netFacts, routerExternal, public string) *Problem {
	details := fmt.Sprintf("router outside address: %q, public address: %q", routerExternal, public)
	switch {
	case routerExternal == "":
		return nil
	case IsCGNAT(routerExternal):
		return problemCGNAT(details)
	case IsPrivate(routerExternal):
		return problemDoubleNAT(f, routerExternal, details)
	case public != "" && routerExternal != public:
		return problemMismatch(routerExternal, public, details)
	}
	return nil
}

func mapProblem(f netFacts, err error, public string) *Problem {
	details := ""
	if err != nil {
		details = err.Error()
	}
	var me *mapError
	if !errors.As(err, &me) {
		return problemNoUPnP(f, details)
	}
	// Even if the mapping failed, an outside address the router reported can
	// show the real cause, which forwarding by hand wouldn't fix.
	if p := addressProblem(f, me.externalIP, public); p != nil {
		p.Details = details + "\n" + p.Details
		return p
	}
	switch {
	case me.conflict:
		return problemConflict(f, me.port, details)
	case me.refused:
		return problemRefused(f, me.port, details)
	}
	return problemNoUPnP(f, details)
}

// Stop closes the router ports and shuts the relay down.
func (h *Host) Stop() {
	if h == nil {
		return
	}
	h.mu.Lock()
	m, srv := h.mapper, h.server
	h.mapper, h.server = nil, nil
	h.mu.Unlock()
	m.close()
	srv.close()
}
