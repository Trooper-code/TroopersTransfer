package ui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/troopers/transfer-service/relay"
	"github.com/troopers/transfer-service/transfer"
)

// TestScreens builds the real window with Fyne's software renderer and walks
// through each tab, the progress panel and the accept dialog. Set
// TTS_SCREENSHOTS=dir to save PNGs for a visual check.
func TestScreens(t *testing.T) {
	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(theme.DefaultTheme())
	a := newApp(fa)
	a.win.Resize(fyne.NewSize(760, 620))
	a.win.Show()

	dir := os.Getenv("TTS_SCREENSHOTS")
	shot := func(name string) {
		img := a.win.Canvas().Capture()
		if dir == "" {
			return
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}

	shot("1-send-empty")

	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "holiday-photos.zip"), make([]byte, 3_200_000), 0o644)
	os.MkdirAll(filepath.Join(tmp, "Project files"), 0o755)
	os.WriteFile(filepath.Join(tmp, "Project files", "a.txt"), make([]byte, 12_000), 0o644)
	a.send.add(filepath.Join(tmp, "holiday-photos.zip"))
	a.send.add(filepath.Join(tmp, "Project files"))
	if len(a.send.items) != 2 || a.send.sendBtn.Disabled() {
		t.Fatalf("items=%d sendDisabled=%v", len(a.send.items), a.send.sendBtn.Disabled())
	}
	shot("2-send-items")

	a.begin("Sending")
	a.send.showCode("curry-salsa-blunt")
	a.progress.update(transfer.Progress{Phase: transfer.PhaseTransferring, CurrentFile: "holiday-photos.zip",
		FileIndex: 1, FileCount: 2, BytesDone: 1_400_000, BytesTotal: 3_212_000, Speed: 5_900_000, ETA: 74e9})
	if !a.send.sendBtn.Disabled() || !a.recv.recvBtn.Disabled() {
		t.Fatal("buttons should be disabled while busy")
	}
	shot("3-send-progress")
	a.end()

	a.tabs.SelectIndex(1)
	a.recv.code.SetText("curry salsa blunt")
	if a.recv.recvBtn.Disabled() {
		t.Fatal("receive should be enabled with a code")
	}
	shot("4-receive")

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan bool, 1)
	go func() {
		got <- a.recv.askAccept(ctx, transfer.Offer{TotalSize: 3_212_000, Files: []transfer.FileEntry{
			{Path: "holiday-photos.zip", Size: 3_200_000},
			{Path: "Project files.zip", Size: 12_000, Zipped: true},
		}})
	}()
	waitFor(t, func() bool { return len(a.win.Canvas().Overlays().List()) > 0 })
	shot("5-offer")
	cancel()
	if <-got {
		t.Fatal("canceled offer must decline")
	}

	h := a.history
	e := h.add(true, []string{"holiday-photos.zip", "Project files"})
	h.finish(e, transfer.Result{Code: "curry-salsa-blunt", Bytes: 3_212_000}, nil)
	e = h.add(false, nil)
	h.finish(e, transfer.Result{Code: "zebra-apple-mango"}, &transfer.FriendlyError{Message: "Connection lost, try again."})
	a.tabs.SelectIndex(4)
	shot("6-history")

	a.tabs.SelectIndex(5)
	shot("7-settings")
}

// TestHostAndConnect starts a real relay from the Host tab, then checks the
// Connect tab can test and use it. Automatic port opening is off so the test
// doesn't depend on the build machine's network; the router logic is covered
// by relay's unit tests.
func TestHostAndConnect(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("APPDATA", cfgDir)
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(theme.DefaultTheme())
	a := newApp(fa)
	a.cfg.RelayAddress, a.cfg.RelayPassword = "", ""
	a.connect.refresh()
	a.win.Resize(fyne.NewSize(760, 760))
	a.win.Show()
	shot := screenshotter(t, a)

	a.tabs.SelectIndex(2)
	shot("8-host-idle")

	a.hostView.port.SetText("29109")
	a.hostView.autoOpen.SetChecked(false)
	test.Tap(a.hostView.startBtn)
	waitForLong(t, func() bool { return a.host != nil })
	defer a.stopHosting()
	shot("9-host-running")

	st := a.host.Status()
	if st.Checked || st.Problem != nil {
		t.Fatalf("auto-open was off, so nothing should be checked: %+v", st)
	}
	if s := a.settings(); s.RelayAddress != "127.0.0.1:29109" || s.RelayPassword != st.Password {
		t.Fatalf("own transfers should use the local relay, got %+v", s)
	}

	// Connect tab: test then connect to the relay we're hosting.
	a.tabs.SelectIndex(3)
	a.connect.code.SetText(relay.FormatCode("127.0.0.1", 29109, st.Password))
	test.Tap(a.connect.connect)
	waitForLong(t, func() bool { return a.cfg.RelayAddress != "" })
	shot("10-connect")
	if a.cfg.RelayAddress != "127.0.0.1:29109" || a.cfg.RelayPassword != st.Password {
		t.Fatalf("connect saved %q / %q", a.cfg.RelayAddress, a.cfg.RelayPassword)
	}

	// Wrong password is explained in plain English.
	a.connect.code.SetText(relay.FormatCode("127.0.0.1", 29109, "nope-nope-nope"))
	test.Tap(a.connect.test)
	waitForLong(t, func() bool { return strings.Contains(a.connect.result.Text, "password is wrong") })

	test.Tap(a.connect.public)
	if a.cfg.RelayAddress != "" {
		t.Fatal("Use public relays should clear the relay")
	}

	test.Tap(a.hostView.startBtn) // Stop hosting
	if a.host != nil {
		t.Fatal("still hosting after Stop")
	}
}

func screenshotter(t *testing.T, a *App) func(string) {
	dir := os.Getenv("TTS_SCREENSHOTS")
	return func(name string) {
		img := a.win.Canvas().Capture()
		if dir == "" {
			return
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}

func waitForLong(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		ok := false
		fyne.DoAndWait(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("condition not met within 30s")
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		ok := false
		fyne.DoAndWait(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met")
}

// TestReadmeScreens renders the Host and Connect tabs in fixed states for the
// README. Run with TTS_SCREENSHOTS=docs/screenshots to regenerate them.
func TestReadmeScreens(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("APPDATA", cfgDir)
	t.Setenv("XDG_CONFIG_HOME", cfgDir)

	fa := test.NewApp()
	defer fa.Quit()
	fa.Settings().SetTheme(theme.DefaultTheme())
	a := newApp(fa)
	a.win.Resize(fyne.NewSize(760, 620))
	a.win.Show()
	shot := screenshotter(t, a)

	base := relay.Status{Port: 9009, Password: "amber-falcon-ridge", LocalIP: "192.168.1.42",
		RouterIP: "192.168.1.1", PublicIP: "203.0.113.5", Checked: true}

	a.hostView.port.SetText("9009")
	a.hostView.pass.SetText(base.Password)
	ok := base
	ok.Opened, ok.Method = true, relay.MethodUPnP
	a.setHost(relay.NewDemo(ok))
	a.tabs.SelectIndex(2)
	a.hostView.render()
	shot("host")

	bad := base
	bad.Problem = relay.DemoManualForwardProblem(bad.LocalIP, bad.RouterIP, bad.Port)
	a.setHost(relay.NewDemo(bad))
	a.hostView.render()
	a.win.Resize(fyne.NewSize(760, 1060))
	shot("host-ports")
	a.win.Resize(fyne.NewSize(760, 620))
	a.stopHosting()

	a.cfg.RelayAddress, a.cfg.RelayPassword = "203.0.113.5:9009", "amber-falcon-ridge"
	a.connect.refresh()
	a.connect.setResult(true, "Connected. Your transfers now go through the relay at 203.0.113.5:9009. "+
		"Your friend needs to use the same relay, so make sure they're hosting it or connected to it too.")
	a.tabs.SelectIndex(3)
	shot("connect")
}
