package relay

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/troopers/transfer-service/transfer"
)

func TestParseCode(t *testing.T) {
	cases := []struct{ in, addr, pass string }{
		{"203.0.113.5:9009/amber-falcon-ridge", "203.0.113.5:9009", "amber-falcon-ridge"},
		{"  203.0.113.5:9109/pw  ", "203.0.113.5:9109", "pw"},
		{"203.0.113.5:9009", "203.0.113.5:9009", ""},
		{"myrelay.example.com/pw", "myrelay.example.com:9009", "pw"},
		{"tt://10.0.0.2:9009/x-y-z", "10.0.0.2:9009", "x-y-z"},
		{"[2001:db8::1]:9009/pw", "[2001:db8::1]:9009", "pw"},
	}
	for _, c := range cases {
		addr, pass, err := ParseCode(c.in)
		if err != nil || addr != c.addr || pass != c.pass {
			t.Errorf("ParseCode(%q) = %q, %q, %v; want %q, %q", c.in, addr, pass, err, c.addr, c.pass)
		}
	}
	for _, bad := range []string{"", "host:abc/pw", "host:80/pw", "host:65535/pw", "two words:9009"} {
		if _, _, err := ParseCode(bad); err == nil {
			t.Errorf("ParseCode(%q) accepted", bad)
		}
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	pw := NewPassword()
	if len(strings.Split(pw, "-")) != 3 {
		t.Fatalf("password %q isn't three words", pw)
	}
	addr, pass, err := ParseCode(FormatCode("198.51.100.7", 9009, pw))
	if err != nil || addr != "198.51.100.7:9009" || pass != pw {
		t.Fatalf("round trip: %q %q %v", addr, pass, err)
	}
}

func TestAddressProblem(t *testing.T) {
	f := netFacts{localIP: "192.168.1.42", routerIP: "192.168.1.1", ports: Ports(9009)}
	cases := []struct {
		routerExt, public string
		want              ProblemKind
	}{
		{"100.72.1.9", "203.0.113.5", ProblemCGNAT},
		{"192.168.0.20", "203.0.113.5", ProblemDoubleNAT},
		{"198.51.100.1", "203.0.113.5", ProblemAddressMismatch},
		{"203.0.113.5", "203.0.113.5", 0},
		{"", "203.0.113.5", 0},
	}
	for _, c := range cases {
		p := addressProblem(f, c.routerExt, c.public)
		got := ProblemKind(0)
		if p != nil {
			got = p.Kind
		}
		if got != c.want {
			t.Errorf("addressProblem(%q, %q) = %v, want %v", c.routerExt, c.public, got, c.want)
		}
	}
}

func TestMapProblem(t *testing.T) {
	f := netFacts{localIP: "192.168.1.42", routerIP: "192.168.1.1", ports: Ports(9009)}
	cases := []struct {
		err  error
		want ProblemKind
	}{
		{&mapError{noRouterAnswer: true, err: errors.New("no igd")}, ProblemNoUPnP},
		{&mapError{refused: true, port: 9010, err: errors.New("606 action not authorized")}, ProblemRefused},
		{&mapError{refused: true, conflict: true, port: 9009, err: errors.New("718")}, ProblemConflict},
		{&mapError{refused: true, port: 9009, externalIP: "100.64.3.3", err: errors.New("x")}, ProblemCGNAT},
		{errors.New("something else"), ProblemNoUPnP},
	}
	for _, c := range cases {
		if p := mapProblem(f, c.err, "203.0.113.5"); p.Kind != c.want {
			t.Errorf("mapProblem(%v) = %v, want %v", c.err, p.Kind, c.want)
		}
	}
}

func TestManualStepsFilledIn(t *testing.T) {
	f := netFacts{localIP: "192.168.1.42", routerIP: "192.168.1.1", ports: Ports(9009)}
	text := problemNoUPnP(f, "").Text()
	for _, want := range []string{"http://192.168.1.1", "192.168.1.42", "9009–9013", "TCP", "1. ", "5. "} {
		if !strings.Contains(text, want) {
			t.Errorf("manual steps missing %q:\n%s", want, text)
		}
	}
	// Without a router address we still tell the user where to look.
	f.routerIP = ""
	if !strings.Contains(problemNoUPnP(f, "").Text(), "sticker") {
		t.Error("steps without router IP should mention the sticker")
	}
}

func freeBase(t *testing.T) int {
	t.Helper()
	for base := 20009; base < 40000; base += 10 {
		ok := true
		for _, p := range Ports(base) {
			l, err := net.Listen("tcp", ":"+strconv.Itoa(p))
			if err != nil {
				ok = false
				break
			}
			l.Close()
		}
		if ok {
			return base
		}
	}
	t.Fatal("no free ports")
	return 0
}

func TestHostPortInUse(t *testing.T) {
	base := freeBase(t)
	l, err := net.Listen("tcp", ":"+strconv.Itoa(base+2))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, err = Start(context.Background(), Options{Port: base, Password: "pw"})
	var se *StartError
	if !errors.As(err, &se) || se.Problem.Kind != ProblemPortInUse || !strings.Contains(se.Problem.Summary, strconv.Itoa(base+2)) {
		t.Fatalf("want port-in-use problem for %d, got %v", base+2, err)
	}
	// The ports it did open must have been released.
	l2, err := net.Listen("tcp", ":"+strconv.Itoa(base))
	if err != nil {
		t.Fatalf("port %d left open after failed start: %v", base, err)
	}
	l2.Close()
}

func TestHostAndTestConnection(t *testing.T) {
	base := freeBase(t)
	h, err := Start(context.Background(), Options{Port: base, Password: "amber-falcon-ridge"})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()

	addr := "127.0.0.1:" + strconv.Itoa(base)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := Test(ctx, addr, "amber-falcon-ridge"); err != nil {
		t.Fatalf("Test with right password: %v", err)
	}
	err = Test(ctx, addr, "wrong-password")
	if err == nil || !strings.Contains(err.Error(), "password is wrong") {
		t.Fatalf("Test with wrong password: %v", err)
	}
	h.Stop()
	// Stop closes the ports before returning, so this must fail right away.
	err = Test(ctx, addr, "amber-falcon-ridge")
	if err == nil || !strings.Contains(err.Error(), "Nothing answered") {
		t.Fatalf("Test after stop: %v", err)
	}
	// Ports are free again after Stop.
	h2, err := Start(context.Background(), Options{Port: base, Password: "x"})
	if err != nil {
		t.Fatalf("restart after stop: %v", err)
	}
	h2.Stop()
}

// TestTransferThroughHostedRelay sends a real file through a relay this
// package hosts, with LAN discovery off so the relay must carry it.
func TestTransferThroughHostedRelay(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	base := freeBase(t)
	h, err := Start(context.Background(), Options{Port: base, Password: "test-relay-pw"})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	st := h.Status()

	src := t.TempDir()
	payload := strings.Repeat("troopers transfer relay test\n", 40000) // ~1.1 MB
	must(t, os.WriteFile(filepath.Join(src, "game-dump.bin"), []byte(payload), 0o644))
	dest := t.TempDir()

	settings := transfer.Settings{RelayAddress: st.OwnAddress(), RelayPassword: st.Password, NoLocal: true}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	codeCh := make(chan string, 1)
	sendErr := make(chan error, 1)
	go func() {
		_, err := transfer.Send(ctx, transfer.SendRequest{Paths: []string{filepath.Join(src, "game-dump.bin")}, Settings: settings},
			transfer.Callbacks{OnCode: func(c string) { codeCh <- c }})
		sendErr <- err
	}()
	var code string
	select {
	case code = <-codeCh:
	case err := <-sendErr:
		t.Fatalf("send failed before code: %v", err)
	case <-ctx.Done():
		t.Fatal("no code")
	}

	// Send and Receive share a process-wide lock (croc keeps global state),
	// so the receiver runs in a child copy of this test binary.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHelperReceive$")
	cmd.Env = append(os.Environ(),
		"TT_HELPER_RECEIVE=1", "TT_CODE="+code, "TT_DEST="+dest,
		"TT_RELAY="+settings.RelayAddress, "TT_PASS="+settings.RelayPassword)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("receive: %v\n%s", err, out)
	}
	if err := <-sendErr; err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "game-dump.bin"))
	if err != nil || string(got) != payload {
		t.Fatalf("received file mismatch (err=%v, %d bytes)", err, len(got))
	}
}

// TestHelperReceive is the child side of TestTransferThroughHostedRelay.
func TestHelperReceive(t *testing.T) {
	if os.Getenv("TT_HELPER_RECEIVE") != "1" {
		t.Skip("helper process")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	_, err := transfer.Receive(ctx, transfer.ReceiveRequest{
		Code:        os.Getenv("TT_CODE"),
		Destination: os.Getenv("TT_DEST"),
		Settings:    transfer.Settings{RelayAddress: os.Getenv("TT_RELAY"), RelayPassword: os.Getenv("TT_PASS"), NoLocal: true},
	}, transfer.Callbacks{OnOffer: func(transfer.Offer) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
