//go:build !windows

package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// downloadsDir honours XDG_DOWNLOAD_DIR on Linux, otherwise ~/Downloads.
func downloadsDir() string {
	if dir := xdgDownloads(); dir != "" {
		return dir
	}
	return homeDownloads()
}

func xdgDownloads() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	f, err := os.Open(filepath.Join(cfg, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	defer f.Close()
	home, _ := os.UserHomeDir()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		v, ok := strings.CutPrefix(line, "XDG_DOWNLOAD_DIR=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		return strings.Replace(v, "$HOME", home, 1)
	}
	return ""
}
