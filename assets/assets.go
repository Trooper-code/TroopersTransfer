// Package assets embeds static resources such as the app icon.
package assets

import _ "embed"

// Icon is the 512x512 application icon (also used by go-winres for the .exe).
//
//go:embed icon.png
var Icon []byte
