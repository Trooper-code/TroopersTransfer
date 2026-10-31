// Package config loads and saves the app's settings as a JSON file.
//
// To stay portable the file lives next to the executable. If that folder is
// not writable (e.g. the exe sits in Program Files or on read-only media) it
// falls back to the per-user config folder (%APPDATA% on Windows). Nothing is
// ever written to the registry.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	AppName  = "Troopers Transfer"
	fileName = "TroopersTransfer.json"
	dirName  = "TroopersTransfer"
)

type Config struct {
	RelayAddress  string `json:"relay_address"`  // empty = public croc relays
	RelayPassword string `json:"relay_password"` // empty = croc default
	ZipFolders    bool   `json:"zip_folders"`
	ReceiveFolder string `json:"receive_folder"` // empty = Downloads/<AppName>

	// Hosting a relay for friends (Host tab).
	HostPort       int    `json:"host_port,omitempty"`         // 0 = 9009
	HostPassword   string `json:"host_password,omitempty"`     // kept so the connection code stays the same
	HostNoAutoOpen bool   `json:"host_no_auto_open,omitempty"` // true = don't try UPnP / NAT-PMP

	path string
}

// Path reports where the settings were loaded from or will be saved to.
func (c *Config) Path() string { return c.path }

// HostPortOrDefault returns the relay port to host on (default 9009).
func (c *Config) HostPortOrDefault() int {
	if c.HostPort > 0 {
		return c.HostPort
	}
	return 9009
}

// DestinationOrDefault returns the configured receive folder or the default.
func (c *Config) DestinationOrDefault() string {
	if c.ReceiveFolder != "" {
		return c.ReceiveFolder
	}
	return DefaultReceiveFolder()
}

// Load reads settings from the portable location, then the fallback. A
// missing file is not an error; defaults are returned.
func Load() *Config {
	for _, p := range candidates() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		c := &Config{}
		if json.Unmarshal(b, c) == nil {
			c.path = p
			return c
		}
	}
	c := &Config{}
	if cs := candidates(); len(cs) > 0 {
		c.path = cs[0]
	}
	return c
}

// Save writes the settings, preferring the folder next to the exe.
func (c *Config) Save() error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	var lastErr error = errors.New("no writable settings location")
	for _, p := range candidates() {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			lastErr = err
			continue
		}
		if err := writeAtomic(p, b); err != nil {
			lastErr = err
			continue
		}
		c.path = p
		return nil
	}
	return lastErr
}

// candidates lists settings paths in priority order: next to the exe (only if
// that folder is writable or the file already exists there), then the
// per-user config dir.
func candidates() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			dir := filepath.Dir(exe)
			p := filepath.Join(dir, fileName)
			if _, err := os.Stat(p); err == nil || writable(dir) {
				out = append(out, p)
			}
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		out = append(out, filepath.Join(dir, dirName, fileName))
	}
	return out
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// DefaultReceiveFolder is Downloads/<AppName>.
func DefaultReceiveFolder() string {
	return filepath.Join(downloadsDir(), AppName)
}

func homeDownloads() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}
