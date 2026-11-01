// Package updater checks GitHub for new Troopers Transfer releases and
// installs them in place: download the release archive, swap the exe, and
// restart. croc is compiled into the app, so this is also how croc upgrades
// reach users: a new croc ships inside a new Troopers Transfer release.
package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// Repo is where releases are published.
const Repo = "Trooper-code/TroopersTransfer"

// CrocRepo is croc's own repository, for the "latest croc" line.
const CrocRepo = "schollz/croc"

// ReleasesPage is the human-readable releases page.
const ReleasesPage = "https://github.com/" + Repo + "/releases"

// apiBase is GitHub's API root. A variable so tests can use a fake server.
var apiBase = "https://api.github.com"

var client = &http.Client{Timeout: 30 * time.Second}

// Asset is a downloadable file attached to a release.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// Release describes a published release.
type Release struct {
	Tag      string // as published, e.g. "v1.2.0" or "V1.1.3"
	Version  string // normalised, e.g. "1.2.0"
	Notes    string
	PageURL  string
	Asset    *Asset // the archive for this platform; nil if there isn't one
	Checksum *Asset // optional "<archive>.sha256"
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func getJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "TroopersTransfer-updater")
	resp, err := client.Do(req)
	if err != nil {
		return &Error{Message: "Couldn't reach GitHub. Check your internet connection and try again.", Err: err}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return &Error{Message: "No releases have been published yet.", Err: fmt.Errorf("GET %s: %s", url, resp.Status)}
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return &Error{Message: "GitHub is limiting update checks right now. Try again in an hour.", Err: fmt.Errorf("GET %s: %s", url, resp.Status)}
	case resp.StatusCode != http.StatusOK:
		return &Error{Message: "GitHub returned an unexpected answer. Try again later.", Err: fmt.Errorf("GET %s: %s", url, resp.Status)}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

// Latest returns the newest published release of repo, with the archive for
// this platform picked out.
func Latest(ctx context.Context, repo string) (*Release, error) {
	var gr ghRelease
	if err := getJSON(ctx, apiBase+"/repos/"+repo+"/releases/latest", &gr); err != nil {
		return nil, err
	}
	rel := &Release{Tag: gr.TagName, Version: Normalize(gr.TagName), Notes: gr.Body, PageURL: gr.HTMLURL}
	want := AssetName()
	for _, a := range gr.Assets {
		a := Asset{Name: a.Name, URL: a.URL, Size: a.Size}
		lower := strings.ToLower(a.Name)
		switch {
		case lower == strings.ToLower(want+".sha256"):
			rel.Checksum = &a
		case lower == strings.ToLower(want):
			rel.Asset = &a
		case rel.Asset == nil && strings.Contains(lower, platformKey()) && strings.HasSuffix(lower, archiveExt()):
			// Accept versioned names like TroopersTransfer-v1.2.0-windows-amd64.zip.
			rel.Asset = &a
		}
	}
	return rel, nil
}

// LatestCroc returns the newest croc release tag, e.g. "v10.2.4".
func LatestCroc(ctx context.Context) (string, error) {
	var gr ghRelease
	if err := getJSON(ctx, apiBase+"/repos/"+CrocRepo+"/releases/latest", &gr); err != nil {
		return "", err
	}
	return gr.TagName, nil
}

// BundledCroc reports the croc version compiled into this build, e.g.
// "v11.5.5", or "" if it can't be determined.
func BundledCroc() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, d := range info.Deps {
		if strings.HasPrefix(d.Path, "github.com/schollz/croc") {
			if d.Replace != nil {
				return d.Replace.Version
			}
			return d.Version
		}
	}
	return ""
}

// Normalize strips a leading "v"/"V" and surrounding space from a tag.
func Normalize(tag string) string {
	tag = strings.TrimSpace(tag)
	return strings.TrimLeft(tag, "vV")
}

func parts(v string) []int {
	v = Normalize(v)
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// Newer reports whether version a is newer than b. Unparseable versions
// (such as "dev") are treated as older than any real one.
func Newer(a, b string) bool {
	pa, pb := parts(a), parts(b)
	if pa == nil {
		return false
	}
	if pb == nil {
		return true
	}
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func platformKey() string {
	switch runtime.GOOS {
	case "windows":
		return "windows-" + runtime.GOARCH
	case "darwin":
		return "macos-" + runtime.GOARCH
	}
	return runtime.GOOS + "-" + runtime.GOARCH
}

func archiveExt() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

// AssetName is the release archive for this platform, matching what the
// build workflow publishes.
func AssetName() string { return "TroopersTransfer-" + platformKey() + archiveExt() }

// Error is a plain-English failure with the technical cause attached.
type Error struct {
	Message string
	Err     error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Err }

// Details returns the technical cause.
func (e *Error) Details() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Install downloads rel and replaces the running executable with the new
// one. The old exe is kept as "<exe>.old" until the next start (Windows
// can't delete a running exe, but it can rename it). It returns the path to
// start.
func Install(ctx context.Context, rel *Release, progress func(done, total int64)) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	return installTo(ctx, exe, rel, progress)
}

func installTo(ctx context.Context, exe string, rel *Release, progress func(done, total int64)) (string, error) {
	if rel == nil || rel.Asset == nil {
		return "", &Error{Message: "This release doesn't include a download for your system. Get it from the releases page instead."}
	}
	dir := filepath.Dir(exe)
	if !writable(dir) {
		return "", &Error{Message: "Troopers Transfer can't update itself in this folder, because Windows won't let it write there. " +
			"Download the new version from the releases page, or move the app to a folder you own (like Desktop or Documents)."}
	}

	archive, err := download(ctx, dir, rel.Asset, progress)
	if err != nil {
		return "", err
	}
	defer os.Remove(archive)

	if rel.Checksum != nil {
		if err := verifyChecksum(ctx, archive, rel.Checksum); err != nil {
			return "", err
		}
	}

	newExe := exe + ".new"
	if err := extractExe(archive, newExe); err != nil {
		os.Remove(newExe)
		return "", &Error{Message: "The downloaded update looks damaged. Try again, or download it from the releases page.", Err: err}
	}

	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(newExe)
		return "", &Error{Message: "Couldn't replace the app file. Close any other copies of Troopers Transfer and try again.", Err: err}
	}
	if err := os.Rename(newExe, exe); err != nil {
		_ = os.Rename(old, exe) // put the working version back
		os.Remove(newExe)
		return "", &Error{Message: "Couldn't install the update. Your current version is unchanged.", Err: err}
	}
	return exe, nil
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".update-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func download(ctx context.Context, dir string, a *Asset, progress func(done, total int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "TroopersTransfer-updater")
	dl := &http.Client{} // no overall timeout: large downloads on slow links
	resp, err := dl.Do(req)
	if err != nil {
		return "", &Error{Message: "The download failed. Check your internet connection and try again.", Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &Error{Message: "The download failed. Try again later.", Err: fmt.Errorf("GET %s: %s", a.URL, resp.Status)}
	}
	total := a.Size
	if total <= 0 {
		total = resp.ContentLength
	}

	f, err := os.CreateTemp(dir, ".TroopersTransfer-update-*"+archiveExt())
	if err != nil {
		return "", err
	}
	name := f.Name()
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				os.Remove(name)
				return "", err
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(name)
			return "", &Error{Message: "The download was interrupted. Try again.", Err: rerr}
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if a.Size > 0 && done != a.Size {
		os.Remove(name)
		return "", &Error{Message: "The download was incomplete. Try again.", Err: fmt.Errorf("got %d of %d bytes", done, a.Size)}
	}
	return name, nil
}

func verifyChecksum(ctx context.Context, archive string, sum *Asset) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sum.URL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return &Error{Message: "Couldn't download the update's checksum. Try again.", Err: err}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	want := strings.ToLower(strings.Fields(string(b) + " ")[0])

	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return &Error{Message: "The download didn't match its checksum, so it wasn't installed. Try again.",
			Err: fmt.Errorf("sha256 %s, expected %s", got, want)}
	}
	return nil
}

// isAppBinary picks the executable out of a release archive.
func isAppBinary(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	if runtime.GOOS == "windows" {
		return strings.HasSuffix(base, ".exe") && strings.Contains(base, "trooperstransfer")
	}
	return base == "trooperstransfer"
}

func extractExe(archive, dest string) error {
	var src io.Reader
	var closeFn func() error
	if strings.HasSuffix(archive, ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if isAppBinary(f.Name) && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				src, closeFn = rc, rc.Close
				break
			}
		}
	} else {
		f, err := os.Open(archive)
		if err != nil {
			return err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if err != nil {
				break
			}
			if h.Typeflag == tar.TypeReg && isAppBinary(h.Name) {
				src = tr
				break
			}
		}
	}
	if src == nil {
		return errors.New("no Troopers Transfer executable in the archive")
	}
	if closeFn != nil {
		defer closeFn()
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(src, 1<<30))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n < 1<<20 {
		err = fmt.Errorf("executable is only %d bytes", n)
	}
	return err
}

// CleanupOld removes the previous version left behind by an update. Call
// it at startup.
func CleanupOld() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if exe, err = filepath.EvalSymlinks(exe); err == nil {
		_ = os.Remove(exe + ".old")
	}
}

// Restart starts exe as a new process. The caller should then quit.
func Restart(exe string) error {
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	return cmd.Start()
}
