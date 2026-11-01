package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// palette is one of the app's colour schemes.
type palette struct {
	Name string
	Dark bool

	Background color.Color // window
	Surface    color.Color // cards, sidebar, inputs
	Raised     color.Color // buttons, hover
	Foreground color.Color
	Muted      color.Color // placeholders, disabled text
	Primary    color.Color
	OnPrimary  color.Color
	Separator  color.Color
	Success    color.Color
	Warning    color.Color
	Error      color.Color
}

func hex(s string) color.Color {
	var r, g, b uint8
	for i, p := range []*uint8{&r, &g, &b} {
		*p = unhex(s[1+i*2])<<4 | unhex(s[2+i*2])
	}
	return color.NRGBA{R: r, G: g, B: b, A: 0xff}
}

func unhex(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// SystemTheme follows Windows' light/dark setting with Fyne's own colours.
const SystemTheme = "System"

// DefaultTheme is used when nothing has been chosen yet.
const DefaultTheme = "Midnight"

// palettes are the built-in themes, in the order the picker shows them.
var palettes = []palette{
	{Name: "Midnight", Dark: true,
		Background: hex("#0f1420"), Surface: hex("#171e2e"), Raised: hex("#222b3f"),
		Foreground: hex("#e6ebf5"), Muted: hex("#8a94a8"), Primary: hex("#5b8def"), OnPrimary: hex("#ffffff"),
		Separator: hex("#283247"), Success: hex("#4cc38a"), Warning: hex("#f0b44c"), Error: hex("#ef6461")},
	{Name: "Daylight",
		Background: hex("#eef1f6"), Surface: hex("#ffffff"), Raised: hex("#e3e8f0"),
		Foreground: hex("#1b2333"), Muted: hex("#6b7487"), Primary: hex("#2f6fe4"), OnPrimary: hex("#ffffff"),
		Separator: hex("#d5dbe5"), Success: hex("#1f9d62"), Warning: hex("#b7791f"), Error: hex("#d64545")},
	{Name: "Trooper", Dark: true,
		Background: hex("#13150f"), Surface: hex("#1d2017"), Raised: hex("#2a2e21"),
		Foreground: hex("#e9ead9"), Muted: hex("#9a9c85"), Primary: hex("#b8a64a"), OnPrimary: hex("#13150f"),
		Separator: hex("#33382a"), Success: hex("#7fb35a"), Warning: hex("#e0a640"), Error: hex("#e0644f")},
	{Name: "Forest", Dark: true,
		Background: hex("#0e1813"), Surface: hex("#15241c"), Raised: hex("#1f3228"),
		Foreground: hex("#e2eee7"), Muted: hex("#86a092"), Primary: hex("#3fbf7f"), OnPrimary: hex("#08130d"),
		Separator: hex("#25392e"), Success: hex("#5fd39a"), Warning: hex("#e8b450"), Error: hex("#ec6a5e")},
	{Name: "Ember", Dark: true,
		Background: hex("#19110e"), Surface: hex("#241915"), Raised: hex("#33241e"),
		Foreground: hex("#f4e9e3"), Muted: hex("#a8918a"), Primary: hex("#ff7a3d"), OnPrimary: hex("#1a0d06"),
		Separator: hex("#3a2a23"), Success: hex("#6cc28a"), Warning: hex("#ffc04d"), Error: hex("#ff5f5f")},
	{Name: "Nightshade", Dark: true,
		Background: hex("#15111d"), Surface: hex("#1f192b"), Raised: hex("#2b233b"),
		Foreground: hex("#ece6f7"), Muted: hex("#9b91ad"), Primary: hex("#b38cff"), OnPrimary: hex("#140c22"),
		Separator: hex("#332a45"), Success: hex("#63d1a0"), Warning: hex("#f3c060"), Error: hex("#ff6b8b")},
	{Name: "Ocean", Dark: true,
		Background: hex("#0b1719"), Surface: hex("#112226"), Raised: hex("#1a3136"),
		Foreground: hex("#e0f1f2"), Muted: hex("#84a3a6"), Primary: hex("#2ec4c4"), OnPrimary: hex("#041314"),
		Separator: hex("#1f383d"), Success: hex("#4fd18b"), Warning: hex("#f2b84b"), Error: hex("#f2666a")},
	{Name: "High contrast", Dark: true,
		Background: hex("#000000"), Surface: hex("#0a0a0a"), Raised: hex("#1f1f1f"),
		Foreground: hex("#ffffff"), Muted: hex("#c8c8c8"), Primary: hex("#ffd500"), OnPrimary: hex("#000000"),
		Separator: hex("#ffffff"), Success: hex("#3dff8f"), Warning: hex("#ffb000"), Error: hex("#ff4d4d")},
}

// ThemeNames lists every choice for the picker: System first, then the
// built-in palettes.
func ThemeNames() []string {
	names := []string{SystemTheme}
	for _, p := range palettes {
		names = append(names, p.Name)
	}
	return names
}

func findPalette(name string) (palette, bool) {
	for _, p := range palettes {
		if p.Name == name {
			return p, true
		}
	}
	return palette{}, false
}

// themeFor returns the Fyne theme for a picker name. Unknown names fall back
// to the default theme, so an old or hand-edited settings file never breaks.
func themeFor(name string) fyne.Theme {
	if name == SystemTheme {
		return &appTheme{base: theme.DefaultTheme()}
	}
	p, ok := findPalette(name)
	if !ok {
		p, _ = findPalette(DefaultTheme)
	}
	return &appTheme{base: theme.DefaultTheme(), p: &p}
}

// appTheme applies a palette over Fyne's default theme and adds the app's
// own spacing and rounder corners. With no palette it only changes sizes.
type appTheme struct {
	base fyne.Theme
	p    *palette
}

func (t *appTheme) variant(v fyne.ThemeVariant) fyne.ThemeVariant {
	if t.p == nil {
		return v
	}
	if t.p.Dark {
		return theme.VariantDark
	}
	return theme.VariantLight
}

func withAlpha(c color.Color, a uint8) color.Color {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: a}
}

func (t *appTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	p := t.p
	if p == nil {
		return t.base.Color(n, v)
	}
	switch n {
	case theme.ColorNameBackground:
		return p.Background
	case theme.ColorNameHeaderBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return p.Surface
	case theme.ColorNameInputBackground:
		return p.Raised
	case theme.ColorNameButton:
		return p.Raised
	case theme.ColorNameDisabledButton:
		return withAlpha(p.Raised, 0x80)
	case theme.ColorNameForeground:
		return p.Foreground
	case theme.ColorNameDisabled, theme.ColorNamePlaceHolder:
		return p.Muted
	case theme.ColorNamePrimary, theme.ColorNameHyperlink, theme.ColorNameFocus:
		if n == theme.ColorNameFocus {
			return withAlpha(p.Primary, 0x99)
		}
		return p.Primary
	case theme.ColorNameForegroundOnPrimary:
		return p.OnPrimary
	case theme.ColorNameSelection:
		return withAlpha(p.Primary, 0x44)
	case theme.ColorNameHover:
		return withAlpha(p.Foreground, 0x14)
	case theme.ColorNamePressed:
		return withAlpha(p.Foreground, 0x26)
	case theme.ColorNameSeparator, theme.ColorNameInputBorder:
		return p.Separator
	case theme.ColorNameScrollBar:
		return withAlpha(p.Foreground, 0x55)
	case theme.ColorNameShadow:
		if p.Dark {
			return color.NRGBA{A: 0x88}
		}
		return color.NRGBA{A: 0x33}
	case theme.ColorNameSuccess:
		return p.Success
	case theme.ColorNameWarning:
		return p.Warning
	case theme.ColorNameError:
		return p.Error
	case theme.ColorNameForegroundOnError, theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning:
		if p.Dark {
			return hex("#0b0b0b")
		}
		return hex("#ffffff")
	}
	return t.base.Color(n, t.variant(v))
}

func (t *appTheme) Font(s fyne.TextStyle) fyne.Resource     { return t.base.Font(s) }
func (t *appTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return t.base.Icon(n) }

func (t *appTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius, theme.SizeNameButtonRadius:
		return 8
	case theme.SizeNameCardRadius, theme.SizeNameDialogRadius, theme.SizeNamePopupRadius:
		return 12
	case theme.SizeNameHeadingText:
		return 22
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameInnerPadding:
		return 10
	}
	return t.base.Size(n)
}

// swatch returns the colours shown next to a theme name in the picker:
// background, surface and accent.
func swatch(name string) []color.Color {
	if p, ok := findPalette(name); ok {
		return []color.Color{p.Background, p.Surface, p.Primary}
	}
	d := theme.DefaultTheme()
	return []color.Color{
		d.Color(theme.ColorNameBackground, theme.VariantLight),
		d.Color(theme.ColorNameBackground, theme.VariantDark),
		d.Color(theme.ColorNamePrimary, theme.VariantDark),
	}
}
