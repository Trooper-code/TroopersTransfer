package ui

import (
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/troopers/transfer-service/transfer"
)

type sendItem struct {
	path  string
	isDir bool
	size  int64
}

type sendView struct {
	a       *App
	content fyne.CanvasObject

	items      []sendItem
	list       *widget.List
	empty      fyne.CanvasObject
	summary    *widget.Label
	customCode *widget.Entry
	sendBtn    *widget.Button
	addFiles   *widget.Button
	addFolder  *widget.Button
	clearBtn   *widget.Button

	codePanel *fyne.Container
	codeText  *canvas.Text
	qr        *canvas.Image
	code      string
}

func newSendView(a *App) *sendView {
	v := &sendView{a: a}

	v.addFiles = widget.NewButtonWithIcon("Add files…", theme.FileIcon(), v.browseFile)
	v.addFolder = widget.NewButtonWithIcon("Add folder…", theme.FolderIcon(), v.browseFolder)
	v.clearBtn = widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		v.items = nil
		v.refresh()
	})

	v.list = widget.NewList(
		func() int { return len(v.items) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("")
			name.Truncation = fyne.TextTruncateEllipsis
			// Placeholder text reserves enough width for any size string.
			size := widget.NewLabelWithStyle("000.0 MB", fyne.TextAlignTrailing, fyne.TextStyle{})
			remove := widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)
			remove.Importance = widget.LowImportance
			return container.NewBorder(nil, nil, widget.NewIcon(theme.FileIcon()), container.NewHBox(size, remove), name)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			it := v.items[i]
			row := o.(*fyne.Container)
			name := row.Objects[0].(*widget.Label)
			icon := row.Objects[1].(*widget.Icon)
			right := row.Objects[2].(*fyne.Container)
			name.SetText(filepath.Base(it.path) + "   —   " + filepath.Dir(it.path))
			if it.isDir {
				icon.SetResource(theme.FolderIcon())
			} else {
				icon.SetResource(theme.FileIcon())
			}
			right.Objects[0].(*widget.Label).SetText(transfer.FormatBytes(it.size))
			right.Objects[1].(*widget.Button).OnTapped = func() {
				if !v.a.busy() {
					v.items = slices.Delete(v.items, i, i+1)
					v.refresh()
				}
			}
		},
	)

	// Dashed-looking drop zone shown while nothing is selected.
	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = theme.Color(theme.ColorNamePrimary)
	border.StrokeWidth = 2
	border.CornerRadius = 12
	hint := widget.NewLabelWithStyle("Drag and drop files or folders here", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	sub := widget.NewLabelWithStyle("or use the buttons below", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	v.empty = container.NewStack(border, container.NewCenter(container.NewVBox(
		container.NewCenter(widget.NewIcon(theme.UploadIcon())), hint, sub)))

	v.summary = widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{})
	v.summary.Truncation = fyne.TextTruncateEllipsis
	v.customCode = widget.NewEntry()
	v.customCode.SetPlaceHolder("Optional: your own code phrase (leave blank for a random one)")

	v.sendBtn = widget.NewButtonWithIcon("Send", theme.MailSendIcon(), v.start)
	v.sendBtn.Importance = widget.HighImportance

	v.codeText = canvas.NewText("", theme.Color(theme.ColorNamePrimary))
	v.codeText.TextSize = 34
	v.codeText.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	v.codeText.Alignment = fyne.TextAlignCenter
	var copyBtn *widget.Button
	copyBtn = widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		a.fa.Clipboard().SetContent(v.code)
		copyBtn.SetText("Copied!")
		copyBtn.SetIcon(theme.ConfirmIcon())
		time.AfterFunc(1500*time.Millisecond, func() {
			fyne.Do(func() {
				copyBtn.SetText("Copy")
				copyBtn.SetIcon(theme.ContentCopyIcon())
			})
		})
	})
	copyBtn.Importance = widget.HighImportance
	v.qr = canvas.NewImageFromResource(nil)
	v.qr.FillMode = canvas.ImageFillContain
	v.qr.SetMinSize(fyne.NewSize(170, 170))
	// While sending, the code replaces the file list.
	v.codePanel = container.NewCenter(container.NewHBox(
		container.NewVBox(
			widget.NewLabelWithStyle("Your code phrase", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			v.codeText,
			container.NewCenter(copyBtn),
			widget.NewLabelWithStyle("Enter it on the other computer's Receive tab,\nor scan the QR code.",
				fyne.TextAlignCenter, fyne.TextStyle{Italic: true}),
		),
		v.qr,
	))
	v.codePanel.Hide()

	body := container.NewStack(v.list, v.empty, v.codePanel)
	buttons := container.NewBorder(nil, nil, container.NewHBox(v.addFiles, v.addFolder, v.clearBtn), nil, v.summary)
	bottom := container.NewVBox(buttons, v.customCode, v.sendBtn)
	v.content = container.NewPadded(container.NewBorder(nil, bottom, nil, nil, body))
	v.refresh()
	return v
}

func (v *sendView) add(path string) {
	path = filepath.Clean(path)
	for _, it := range v.items {
		if it.path == path {
			return
		}
	}
	st, err := os.Stat(path)
	if err != nil {
		v.a.showError("Can't add item", &transfer.FriendlyError{Message: "Can't open " + filepath.Base(path) + ".", Err: err})
		return
	}
	it := sendItem{path: path, isDir: st.IsDir(), size: st.Size()}
	if it.isDir {
		it.size = dirSize(path)
	}
	v.items = append(v.items, it)
	v.refresh()
}

func (v *sendView) refresh() {
	if len(v.items) == 0 {
		v.summary.SetText("")
	} else {
		var total int64
		for _, it := range v.items {
			total += it.size
		}
		v.summary.SetText(fmt.Sprintf("%d item(s), %s", len(v.items), transfer.FormatBytes(total)))
	}
	v.list.Refresh()
	v.setBusy(v.a.busy())
}

// layoutBody shows exactly one of: the code panel, the file list, or the
// empty drop zone.
func (v *sendView) layoutBody() {
	code := v.codePanel.Visible()
	setVisible(v.list, !code && len(v.items) > 0)
	setVisible(v.empty, !code && len(v.items) == 0)
}

func setVisible(o fyne.CanvasObject, visible bool) {
	if visible {
		o.Show()
	} else {
		o.Hide()
	}
}

func (v *sendView) setBusy(busy bool) {
	for _, w := range []fyne.Disableable{v.addFiles, v.addFolder, v.clearBtn, v.customCode} {
		if busy {
			w.Disable()
		} else {
			w.Enable()
		}
	}
	if busy || len(v.items) == 0 {
		v.sendBtn.Disable()
	} else {
		v.sendBtn.Enable()
	}
	if !busy {
		v.codePanel.Hide()
	}
	v.layoutBody()
}

func (v *sendView) browseFile() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		p := uriPath(r.URI())
		r.Close()
		v.add(p)
	}, v.a.win)
	d.Resize(dialogSize(v.a.win))
	d.Show()
}

func (v *sendView) browseFolder() {
	d := dialog.NewFolderOpen(func(u fyne.ListableURI, err error) {
		if err != nil || u == nil {
			return
		}
		v.add(uriPath(u))
	}, v.a.win)
	d.Resize(dialogSize(v.a.win))
	d.Show()
}

func (v *sendView) showCode(code string) {
	v.code = code
	v.codeText.Text = code
	v.codeText.Refresh()
	if png, err := qrcode.Encode(code, qrcode.Medium, 256); err == nil {
		v.qr.Resource = fyne.NewStaticResource("code.png", png)
		v.qr.Refresh()
	}
	v.codePanel.Show()
	v.codePanel.Refresh() // re-measure the code text
	v.layoutBody()
}

func (v *sendView) start() {
	if len(v.items) == 0 || v.a.busy() {
		return
	}
	paths := make([]string, len(v.items))
	for i, it := range v.items {
		paths[i] = it.path
	}
	req := transfer.SendRequest{Paths: paths, Code: transfer.NormalizeCode(v.customCode.Text), Settings: v.a.settings()}
	ctx := v.a.begin("Sending")
	entry := v.a.history.add(true, paths)

	go func() {
		res, err := transfer.Send(ctx, req, transfer.Callbacks{
			OnCode:     func(code string) { fyne.Do(func() { v.showCode(code) }) },
			OnProgress: func(p transfer.Progress) { fyne.Do(func() { v.a.progress.update(p) }) },
		})
		fyne.Do(func() {
			v.a.end()
			v.a.history.finish(entry, res, err)
			if err != nil {
				v.a.showError("Send failed", err)
				return
			}
			v.items = nil
			v.refresh()
			dialog.ShowInformation("Sent", fmt.Sprintf("Sent %s in %s.",
				transfer.FormatBytes(res.Bytes), formatETA(res.Duration)), v.a.win)
		})
	}()
}

func dirSize(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

func uriPath(u fyne.URI) string {
	return filepath.FromSlash(u.Path())
}

func dialogSize(w fyne.Window) fyne.Size {
	s := w.Canvas().Size()
	return fyne.NewSize(s.Width*0.95, s.Height*0.95)
}
