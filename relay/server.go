package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/schollz/croc/v11/src/models"
	"github.com/schollz/croc/v11/src/tcp"
)

// server is a croc relay running inside this process on PortCount
// consecutive ports. The first is the control port; its banner tells clients
// which transfer ports to use, exactly like the croc CLI relay.
type server struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
}

// portError says which port couldn't be opened.
type portError struct {
	port int
	err  error
}

func (e *portError) Error() string { return fmt.Sprintf("can't listen on port %d: %v", e.port, e.err) }
func (e *portError) Unwrap() error { return e.err }

// startServer binds every port first, so a port that's already taken is
// reported before anything starts, then serves them all.
func startServer(base int, password string) (*server, error) {
	ports := Ports(base)
	listeners := make([]net.Listener, 0, len(ports))
	for _, p := range ports {
		l, err := net.Listen("tcp", ":"+strconv.Itoa(p))
		if err != nil {
			for _, o := range listeners {
				o.Close()
			}
			return nil, &portError{port: p, err: err}
		}
		listeners = append(listeners, l)
	}

	transferPorts := make([]string, 0, len(ports)-1)
	for _, p := range ports[1:] {
		transferPorts = append(transferPorts, strconv.Itoa(p))
	}
	banner := strings.Join(transferPorts, ",")

	ctx, cancel := context.WithCancel(context.Background())
	s := &server{cancel: cancel}
	for i, l := range listeners {
		b := ""
		if i == 0 {
			b = banner
		}
		l := l
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			_ = tcp.RunWithListener(l, password, tcp.WithCtx(ctx), tcp.WithLogLevel("error"), tcp.WithBanner(b))
		}()
	}
	return s, nil
}

// close stops the relay and waits briefly for it to finish.
func (s *server) close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.cancel()
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})
}

// Test connects to a relay the way a transfer would: it signs in to the
// control port with the password, then checks each transfer port the relay
// advertises can be reached. Errors are plain English.
func Test(ctx context.Context, addr, password string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("That address isn't valid. It should look like 203.0.113.5:9009.")
	}
	pw := password
	if pw == "" {
		pw = models.DEFAULT_PASSPHRASE
	}
	room := "tt-test-" + randomHex(6)
	conn, banner, _, _, err := tcp.ConnectToTCPServerControlContext(ctx, addr, pw, room, 8*time.Second)
	if err != nil {
		return explainTestError(addr, err)
	}
	conn.Close()

	var unreachable []string
	for _, p := range strings.Split(banner, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		d := net.Dialer{Timeout: 4 * time.Second}
		c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, p))
		if err != nil {
			unreachable = append(unreachable, p)
			continue
		}
		c.Close()
	}
	if len(unreachable) > 0 {
		return fmt.Errorf("Reached the relay, but not all of its ports: %s can't be reached. "+
			"Ask whoever is hosting to forward all five ports (TCP %s and the four after it), not just the first one.",
			strings.Join(unreachable, ", "), portOf(addr))
	}
	return nil
}

func explainTestError(addr string, err error) error {
	msg := strings.ToLower(err.Error())
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(msg, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("bad password", "admission rejected", "incorrect password"):
		return errors.New("The relay is running, but the password is wrong. Check the part after the / in the connection code.")
	case has("no such host", "lookup"):
		return fmt.Errorf("Couldn't find %s. Check the connection code for typos.", addr)
	case has("connection refused", "actively refused"):
		return fmt.Errorf("Nothing answered at %s. Make sure your friend has clicked Start hosting, and that the address and port are right.", addr)
	case has("timeout", "deadline exceeded", "timed out", "i/o timeout"):
		return fmt.Errorf("Couldn't reach %s. Your friend's router or firewall is probably blocking it. "+
			"They should check the Host tab for instructions on opening the ports, and click Allow if Windows asks about the firewall.", addr)
	case has("network is unreachable", "no route"):
		return errors.New("Couldn't reach the relay. Check your internet connection.")
	}
	return fmt.Errorf("Couldn't connect to the relay at %s (%v).", addr, err)
}

func portOf(addr string) string {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return strconv.Itoa(DefaultPort)
	}
	return p
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
