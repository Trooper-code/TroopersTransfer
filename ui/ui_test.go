package ui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

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
	a.tabs.SelectIndex(2)
	shot("6-history")

	a.tabs.SelectIndex(3)
	shot("7-settings")
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
