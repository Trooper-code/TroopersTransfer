package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/troopers/transfer-service/transfer"
)

type receiveView struct {
	a       *App
	content fyne.CanvasObject

	code       *widget.Entry
	paste      *widget.Button
	destLabel  *widget.Label
	changeDest *widget.Button
	recvBtn    *widget.Button
	dest       string
}

func newReceiveView(a *App) *receiveView {
	v := &receiveView{a: a, dest: a.cfg.DestinationOrDefault()}

	v.code = widget.NewEntry()
	v.code.SetPlaceHolder("e.g. curry-salsa-blunt")
	v.code.OnSubmitted = func(string) { v.start() }
	v.code.OnChanged = func(string) { v.setBusy(a.busy()) }
	v.paste = widget.NewButtonWithIcon("Paste", theme.ContentPasteIcon(), func() {
		v.code.SetText(strings.TrimSpace(a.fa.Clipboard().Content()))
	})

	v.destLabel = widget.NewLabel(v.dest)
	v.destLabel.Truncation = fyne.TextTruncateEllipsis
	v.changeDest = widget.NewButtonWithIcon("Change…", theme.FolderOpenIcon(), v.chooseDest)
	openDest := widget.NewButtonWithIcon("Open", theme.FolderIcon(), func() { a.openFolder(v.dest) })

	v.recvBtn = widget.NewButtonWithIcon("Receive", theme.DownloadIcon(), v.start)
	v.recvBtn.Importance = widget.HighImportance

	help := widget.NewLabel("Ask the sender for their code phrase. You'll see the list of files and can accept or decline before anything is saved.")
	help.Wrapping = fyne.TextWrapWord

	form := widget.NewForm(
		widget.NewFormItem("Code phrase", container.NewBorder(nil, nil, nil, v.paste, v.code)),
		widget.NewFormItem("Save to", container.NewBorder(nil, nil, nil, container.NewHBox(v.changeDest, openDest), v.destLabel)),
	)
	v.content = scrollPage("Receive", "Get files someone is sending you.",
		section("Enter the code", help.Text, form, v.recvBtn))
	v.setBusy(false)
	return v
}

func (v *receiveView) setBusy(busy bool) {
	for _, w := range []fyne.Disableable{v.code, v.paste, v.changeDest} {
		if busy {
			w.Disable()
		} else {
			w.Enable()
		}
	}
	if busy || len(transfer.NormalizeCode(v.code.Text)) < 6 {
		v.recvBtn.Disable()
	} else {
		v.recvBtn.Enable()
	}
}

func (v *receiveView) setDest(dir string) {
	v.dest = dir
	v.destLabel.SetText(dir)
}

func (v *receiveView) chooseDest() {
	d := dialog.NewFolderOpen(func(u fyne.ListableURI, err error) {
		if err == nil && u != nil {
			v.setDest(uriPath(u))
		}
	}, v.a.win)
	if l, err := storage.ListerForURI(storage.NewFileURI(v.dest)); err == nil {
		d.SetLocation(l)
	}
	d.Resize(dialogSize(v.a.win))
	d.Show()
}

func (v *receiveView) start() {
	code := transfer.NormalizeCode(v.code.Text)
	if len(code) < 6 || v.a.busy() {
		return
	}
	req := transfer.ReceiveRequest{Code: code, Destination: v.dest, Settings: v.a.settings()}
	ctx := v.a.begin("Receiving")
	entry := v.a.history.add(false, nil)

	go func() {
		res, err := transfer.Receive(ctx, req, transfer.Callbacks{
			OnProgress: func(p transfer.Progress) { fyne.Do(func() { v.a.progress.update(p) }) },
			OnOffer:    func(o transfer.Offer) bool { return v.askAccept(ctx, o) },
		})
		fyne.Do(func() {
			v.a.end()
			v.a.history.finish(entry, res, err)
			if err != nil {
				v.a.showError("Receive failed", err)
				return
			}
			v.code.SetText("")
			v.showDone(res)
		})
	}()
}

// askAccept shows the incoming file list and blocks (off the UI goroutine)
// until the user decides or the transfer is canceled.
func (v *receiveView) askAccept(ctx context.Context, o transfer.Offer) bool {
	answer := make(chan bool, 1)
	var d *dialog.ConfirmDialog
	fyne.Do(func() {
		list := widget.NewList(
			func() int { return len(o.Files) },
			func() fyne.CanvasObject {
				name := widget.NewLabel("")
				name.Truncation = fyne.TextTruncateEllipsis
				// Placeholder text reserves width for any size string.
				return container.NewBorder(nil, nil, nil, widget.NewLabel("000.0 MB"), name)
			},
			func(i widget.ListItemID, obj fyne.CanvasObject) {
				f := o.Files[i]
				row := obj.(*fyne.Container)
				name := f.Path
				if f.Zipped {
					name = strings.TrimSuffix(name, ".zip") + "/  (folder)"
				}
				row.Objects[0].(*widget.Label).SetText(name)
				row.Objects[1].(*widget.Label).SetText(transfer.FormatBytes(f.Size))
			},
		)
		summary := fmt.Sprintf("The sender wants to give you %d file(s), %s in total.", len(o.Files), transfer.FormatBytes(o.TotalSize))
		if n := len(o.EmptyFolders); n > 0 {
			summary += fmt.Sprintf(" Plus %d empty folder(s).", n)
		}
		head := widget.NewLabel(summary)
		head.Wrapping = fyne.TextWrapWord
		where := widget.NewLabel("They will be saved to " + v.dest + ". Existing files are never overwritten.")
		where.Wrapping = fyne.TextWrapWord
		body := container.NewBorder(head, where, nil, nil, list)

		d = dialog.NewCustomConfirm("Accept these files?", "Accept", "Decline", body, func(ok bool) {
			answer <- ok
		}, v.a.win)
		d.SetConfirmImportance(widget.HighImportance)
		d.Resize(fyne.NewSize(560, 420))
		d.Show()
		v.a.win.RequestFocus()
	})
	select {
	case ok := <-answer:
		return ok
	case <-ctx.Done():
		fyne.Do(func() {
			if d != nil {
				d.Hide()
			}
		})
		return false
	}
}

func (v *receiveView) showDone(res transfer.Result) {
	names := make([]string, 0, len(res.Files))
	for i, f := range res.Files {
		if i == 8 {
			names = append(names, fmt.Sprintf("…and %d more", len(res.Files)-8))
			break
		}
		names = append(names, "• "+filepath.Base(f))
	}
	msg := widget.NewLabel(fmt.Sprintf("Saved %s to %s:\n%s", transfer.FormatBytes(res.Bytes), v.dest, strings.Join(names, "\n")))
	msg.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm("Received", "Open folder", "Close", msg, func(open bool) {
		if open {
			v.a.openFolder(v.dest)
		}
	}, v.a.win)
	d.Resize(fyne.NewSize(460, 0))
	d.Show()
}
