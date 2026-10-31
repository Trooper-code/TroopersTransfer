package config

import "golang.org/x/sys/windows"

// downloadsDir asks Windows for the real Downloads folder, which users can
// relocate (e.g. to another drive).
func downloadsDir() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0); err == nil && p != "" {
		return p
	}
	return homeDownloads()
}
