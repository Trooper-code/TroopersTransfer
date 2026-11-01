package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// newAppearanceSection is the theme picker. A theme applies immediately and
// is remembered in the settings file.
func newAppearanceSection(a *App) fyne.CanvasObject {
	preview := container.NewHBox()
	showPreview := func(name string) {
		preview.Objects = nil
		for _, c := range swatch(name) {
			preview.Add(colorDot(c))
		}
		preview.Refresh()
	}

	pick := widget.NewSelect(ThemeNames(), nil)
	pick.SetSelected(a.themeName())
	showPreview(a.themeName())
	pick.OnChanged = func(name string) {
		showPreview(name)
		a.setTheme(name)
	}

	// Quick-pick buttons, one per theme, with its colours.
	grid := container.NewGridWithColumns(3)
	for _, name := range ThemeNames() {
		dots := container.NewHBox()
		for _, c := range swatch(name) {
			dots.Add(colorDot(c))
		}
		b := widget.NewButton(name, func() { pick.SetSelected(name) })
		b.Alignment = widget.ButtonAlignLeading
		grid.Add(container.NewBorder(nil, nil, nil, container.NewCenter(dots), b))
	}

	form := widget.NewForm(widget.NewFormItem("Theme",
		container.NewBorder(nil, nil, nil, container.NewCenter(preview), pick)))
	return section("Appearance", "Pick a theme. It applies straight away. System follows your Windows light or dark setting.",
		form, grid)
}
