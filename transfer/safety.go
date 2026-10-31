package transfer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var errOutsideDestination = errors.New("path is outside the destination folder")

// croc already normalises every incoming name with receivefs.Normalize and
// performs all receive I/O through an os.Root opened on the working
// directory, so a hostile sender cannot write outside it. The checks below are
// our own, independent layer on top of that.

// safeRelative validates a sender-supplied relative path (forward or back
// slashes) and returns it in OS form. It rejects absolute paths, drive
// letters, "..", empty names and Windows reserved device names.
func safeRelative(p string) (string, error) {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, ":") {
		return "", fmt.Errorf("%w: %q", errOutsideDestination, p)
	}
	clean := path.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", errOutsideDestination, p)
	}
	osPath := filepath.FromSlash(clean)
	if !filepath.IsLocal(osPath) {
		return "", fmt.Errorf("%w: %q", errOutsideDestination, p)
	}
	return osPath, nil
}

// within reports whether target is root or lies inside it.
func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || filepath.IsLocal(rel)
}

// verifyTree walks a finished staging folder and makes sure nothing in it
// points outside. Symlinks whose target escapes the folder are removed.
func verifyTree(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !within(root, p) {
			return fmt.Errorf("%w: %s", errOutsideDestination, p)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(p), target)
			}
			if !within(root, filepath.Clean(target)) {
				if err := os.Remove(p); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// uniquePath returns dir/name, or "name (1).ext", "name (2).ext", ... if that
// already exists. Folders get the suffix at the end of the name.
func uniquePath(dir, name string, isDir bool) string {
	candidate := filepath.Join(dir, name)
	if _, err := os.Lstat(candidate); errors.Is(err, fs.ErrNotExist) {
		return candidate
	}
	base, ext := name, ""
	if !isDir {
		ext = filepath.Ext(name)
		base = strings.TrimSuffix(name, ext)
	}
	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Lstat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
	}
}

// moveIntoPlace moves every top-level entry of staging into dest, renaming on
// collision so that nothing existing is ever overwritten. It returns the final
// paths.
func moveIntoPlace(staging, dest string) ([]string, error) {
	entries, err := os.ReadDir(staging)
	if err != nil {
		return nil, err
	}
	var saved []string
	for _, e := range entries {
		src := filepath.Join(staging, e.Name())
		dst := uniquePath(dest, e.Name(), e.IsDir())
		if !within(dest, dst) {
			return saved, fmt.Errorf("%w: %s", errOutsideDestination, dst)
		}
		if err := os.Rename(src, dst); err != nil {
			return saved, err
		}
		saved = append(saved, dst)
	}
	return saved, nil
}
