package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeRelative(t *testing.T) {
	ok := []string{"a.txt", "folder/a.txt", `folder\sub\b.txt`, "./x/y", "a/../b"}
	bad := []string{"", ".", "..", "../x", "a/../../x", "/etc/passwd", `\\server\share\x`, `C:\Windows\x`, "c:x", `..\x`}
	for _, p := range ok {
		if _, err := safeRelative(p); err != nil {
			t.Errorf("safeRelative(%q) rejected: %v", p, err)
		}
	}
	for _, p := range bad {
		if _, err := safeRelative(p); err == nil {
			t.Errorf("safeRelative(%q) accepted", p)
		}
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "a (1).txt"), nil, 0o644))
	must(t, os.Mkdir(filepath.Join(dir, "pics.v2"), 0o755))

	if got, want := uniquePath(dir, "a.txt", false), filepath.Join(dir, "a (2).txt"); got != want {
		t.Errorf("got %s want %s", got, want)
	}
	if got, want := uniquePath(dir, "pics.v2", true), filepath.Join(dir, "pics.v2 (1)"); got != want {
		t.Errorf("got %s want %s", got, want)
	}
	if got, want := uniquePath(dir, "new.txt", false), filepath.Join(dir, "new.txt"); got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestMoveIntoPlaceNeverOverwrites(t *testing.T) {
	dest := t.TempDir()
	staging := filepath.Join(dest, ".troopers-incoming-x")
	must(t, os.MkdirAll(filepath.Join(staging, "folder"), 0o755))
	must(t, os.WriteFile(filepath.Join(staging, "a.txt"), []byte("new"), 0o644))
	must(t, os.WriteFile(filepath.Join(staging, "folder", "b.txt"), []byte("new"), 0o644))
	must(t, os.WriteFile(filepath.Join(dest, "a.txt"), []byte("old"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dest, "folder"), 0o755))
	must(t, os.WriteFile(filepath.Join(dest, "folder", "b.txt"), []byte("old"), 0o644))

	saved, err := moveIntoPlace(staging, dest)
	must(t, err)
	if len(saved) != 2 {
		t.Fatalf("saved %v", saved)
	}
	for _, p := range []string{filepath.Join(dest, "a.txt"), filepath.Join(dest, "folder", "b.txt")} {
		if b, _ := os.ReadFile(p); string(b) != "old" {
			t.Errorf("%s was overwritten", p)
		}
	}
	for _, p := range []string{filepath.Join(dest, "a (1).txt"), filepath.Join(dest, "folder (1)", "b.txt")} {
		if b, _ := os.ReadFile(p); string(b) != "new" {
			t.Errorf("%s missing", p)
		}
	}
}

func TestVerifyTreeRemovesEscapingSymlinks(t *testing.T) {
	root := t.TempDir()
	must(t, os.WriteFile(filepath.Join(root, "inside.txt"), nil, 0o644))
	if err := os.Symlink(filepath.Join("..", ".."), filepath.Join(root, "escape")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	must(t, os.Symlink("inside.txt", filepath.Join(root, "ok")))
	must(t, verifyTree(root))
	if _, err := os.Lstat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Error("escaping symlink was kept")
	}
	if _, err := os.Lstat(filepath.Join(root, "ok")); err != nil {
		t.Error("safe symlink was removed")
	}
}

func TestNormalizeCode(t *testing.T) {
	if got := NormalizeCode("  curry salsa\tblunt \n"); got != "curry-salsa-blunt" {
		t.Errorf("got %q", got)
	}
}

func TestRelayPorts(t *testing.T) {
	got := relayPorts("relay.example.com:7000")
	if len(got) != 5 || got[0] != "7000" || got[4] != "7004" {
		t.Errorf("got %v", got)
	}
	if got := relayPorts("relay.example.com"); got[0] != "9009" {
		t.Errorf("got %v", got)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
