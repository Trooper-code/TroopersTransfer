package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.2.0", "1.1.3", true},
		{"v1.1.3", "1.1.3", false},
		{"V1.1.10", "1.1.9", true},
		{"1.1", "1.1.0", false},
		{"1.1.1", "1.1", true},
		{"1.0.0", "dev", true},
		{"dev", "1.0.0", false},
		{"2.0.0-beta", "1.9.9", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// fakeApp is a fake executable big enough to pass the size check.
var fakeApp = bytes.Repeat([]byte("NEWVERSION"), 200_000)

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "TroopersTransfer.exe"
	}
	return "TroopersTransfer"
}

func buildArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create(binaryName())
		w.Write(fakeApp)
		zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: binaryName(), Mode: 0o755, Size: int64(len(fakeApp)), Typeflag: tar.TypeReg})
	tw.Write(fakeApp)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// fakeGitHub serves the releases API and the release files.
func fakeGitHub(t *testing.T, archive []byte, checksum string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + Repo + "/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"V1.9.0","body":"notes","html_url":"%[1]s/rel","assets":[
				{"name":"TroopersTransfer-v1.9.0-source.zip","browser_download_url":"%[1]s/src","size":10},
				{"name":"%[2]s","browser_download_url":"%[1]s/dl","size":%[3]d},
				{"name":"%[2]s.sha256","browser_download_url":"%[1]s/sum","size":64}]}`,
				srv.URL, AssetName(), len(archive))
		case "/repos/" + CrocRepo + "/releases/latest":
			fmt.Fprint(w, `{"tag_name":"v11.9.0"}`)
		case "/dl":
			w.Write(archive)
		case "/sum":
			fmt.Fprintf(w, "%s  %s\n", checksum, AssetName())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = old })
	return srv
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestLatestAndInstall(t *testing.T) {
	archive := buildArchive(t)
	fakeGitHub(t, archive, sum(archive))
	ctx := context.Background()

	rel, err := Latest(ctx, Repo)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "1.9.0" || rel.Asset == nil || rel.Asset.Name != AssetName() || rel.Checksum == nil {
		t.Fatalf("release parsed wrong: %+v asset=%+v", rel, rel.Asset)
	}
	if croc, err := LatestCroc(ctx); err != nil || croc != "v11.9.0" {
		t.Fatalf("LatestCroc = %q, %v", croc, err)
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, binaryName())
	os.WriteFile(exe, []byte("OLDVERSION"), 0o755)

	var lastDone, lastTotal int64
	got, err := installTo(ctx, exe, rel, func(d, tot int64) { lastDone, lastTotal = d, tot })
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if got != exe || lastDone != int64(len(archive)) || lastTotal != int64(len(archive)) {
		t.Fatalf("got %q progress %d/%d", got, lastDone, lastTotal)
	}
	if b, _ := os.ReadFile(exe); !bytes.Equal(b, fakeApp) {
		t.Fatal("exe wasn't replaced with the new version")
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "OLDVERSION" {
		t.Fatal("old version should be kept as .old until the next start")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".TroopersTransfer-update-*"))
	if len(leftovers) != 0 {
		t.Fatalf("download left behind: %v", leftovers)
	}
}

func TestChecksumMismatchKeepsCurrentVersion(t *testing.T) {
	archive := buildArchive(t)
	fakeGitHub(t, archive, strings.Repeat("0", 64))
	rel, err := Latest(context.Background(), Repo)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, binaryName())
	os.WriteFile(exe, []byte("OLDVERSION"), 0o755)

	_, err = installTo(context.Background(), exe, rel, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want checksum error, got %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "OLDVERSION" {
		t.Fatal("current version must be untouched after a failed update")
	}
	if _, err := os.Stat(exe + ".new"); !os.IsNotExist(err) {
		t.Fatal(".new left behind")
	}
}

func TestDamagedArchiveKeepsCurrentVersion(t *testing.T) {
	archive := []byte("this is not an archive")
	fakeGitHub(t, archive, sum(archive))
	rel, _ := Latest(context.Background(), Repo)
	dir := t.TempDir()
	exe := filepath.Join(dir, binaryName())
	os.WriteFile(exe, []byte("OLDVERSION"), 0o755)
	if _, err := installTo(context.Background(), exe, rel, nil); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("want damaged error, got %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "OLDVERSION" {
		t.Fatal("current version must be untouched")
	}
}

func TestNoAssetForPlatform(t *testing.T) {
	if _, err := installTo(context.Background(), "x", &Release{Version: "9"}, nil); err == nil ||
		!strings.Contains(err.Error(), "releases page") {
		t.Fatalf("got %v", err)
	}
}
