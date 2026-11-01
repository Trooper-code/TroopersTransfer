package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// box draws its content on a rounded, themed background. The colours are
// read from the current theme every time it refreshes, so switching themes
// restyles it immediately.
type box struct {
	widget.BaseWidget
	content fyne.CanvasObject
	fill    func() color.Color
	stroke  func() color.Color
	radius  fyne.ThemeSizeName
	pad     float32 // padding multiplier around the content
}

func newBox(content fyne.CanvasObject) *box {
	b := &box{
		content: content,
		fill:    func() color.Color { return theme.Color(theme.ColorNameHeaderBackground) },
		stroke:  func() color.Color { return theme.Color(theme.ColorNameSeparator) },
		radius:  theme.SizeNameCardRadius,
		pad:     2,
	}
	b.ExtendBaseWidget(b)
	return b
}

func (b *box) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	r := &boxRenderer{b: b, bg: bg}
	r.Refresh()
	return r
}

type boxRenderer struct {
	b  *box
	bg *canvas.Rectangle
}

func (r *boxRenderer) inset() float32 { return theme.Padding() * r.b.pad }

func (r *boxRenderer) Layout(s fyne.Size) {
	r.bg.Resize(s)
	in := r.inset()
	r.b.content.Move(fyne.NewPos(in, in))
	r.b.content.Resize(fyne.NewSize(s.Width-2*in, s.Height-2*in))
}

func (r *boxRenderer) MinSize() fyne.Size {
	in := r.inset()
	return r.b.content.MinSize().Add(fyne.NewSize(2*in, 2*in))
}

func (r *boxRenderer) Refresh() {
	r.bg.FillColor = r.b.fill()
	r.bg.StrokeColor = r.b.stroke()
	r.bg.StrokeWidth = 1
	r.bg.CornerRadius = theme.Size(r.b.radius)
	r.bg.Refresh()
	r.b.content.Refresh()
}

func (r *boxRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.b.content} }
func (r *boxRenderer) Destroy()                     {}

// section is a titled card: a bold heading, an optional one-line
// explanation, then the content.
func section(title, subtitle string, objs ...fyne.CanvasObject) *box {
	head := widget.NewLabel(title)
	head.TextStyle = fyne.TextStyle{Bold: true}
	head.SizeName = theme.SizeNameSubHeadingText
	items := []fyne.CanvasObject{head}
	if subtitle != "" {
		sub := widget.NewLabel(subtitle)
		sub.Wrapping = fyne.TextWrapWord
		sub.Importance = widget.LowImportance
		items = append(items, sub)
	}
	items = append(items, objs...)
	return newBox(container.NewVBox(items...))
}

// page gives every tab the same header: a large title and a muted
// description, with the body filling the rest.
func page(title, subtitle string, body fyne.CanvasObject) fyne.CanvasObject {
	head := widget.NewLabel(title)
	head.TextStyle = fyne.TextStyle{Bold: true}
	head.SizeName = theme.SizeNameHeadingText
	sub := widget.NewLabel(subtitle)
	sub.Wrapping = fyne.TextWrapWord
	sub.Importance = widget.LowImportance
	header := container.NewVBox(head, sub)
	return container.NewPadded(container.NewBorder(header, nil, nil, nil, body))
}

// scrollPage is page with a scrolling stack of sections.
func scrollPage(title, subtitle string, sections ...fyne.CanvasObject) fyne.CanvasObject {
	return page(title, subtitle, container.NewVScroll(container.NewVBox(sections...)))
}

// pill is a small rounded label tinted with the accent colour, used for the
// relay status in the header.
type pill struct {
	*box
	label *widget.Label
	tone  widget.Importance
}

func newPill() *pill {
	l := widget.NewLabel("")
	l.TextStyle = fyne.TextStyle{Bold: true}
	p := &pill{label: l}
	p.box = newBox(l)
	p.box.pad = 0.5
	p.box.radius = theme.SizeNameInputRadius
	p.box.fill = func() color.Color {
		c := theme.Color(theme.ColorNamePrimary)
		switch p.tone {
		case widget.SuccessImportance:
			c = theme.Color(theme.ColorNameSuccess)
		case widget.WarningImportance:
			c = theme.Color(theme.ColorNameWarning)
		}
		return withAlpha(c, 0x2e)
	}
	p.box.stroke = func() color.Color { return color.Transparent }
	return p
}

func (p *pill) set(text string, tone widget.Importance) {
	p.tone = tone
	p.label.Importance = tone
	p.label.SetText(text)
	p.box.Refresh()
}

// colorDot is a small filled circle for theme swatches.
func colorDot(c color.Color) fyne.CanvasObject {
	d := canvas.NewCircle(c)
	d.StrokeColor = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0x80}
	d.StrokeWidth = 1
	return container.NewGridWrap(fyne.NewSize(14, 14), d)
}
