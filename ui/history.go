package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/troopers/transfer-service/transfer"
)

type historyEntry struct {
	sent   bool
	when   time.Time
	names  []string
	bytes  int64
	code   string
	status string // "", "Done", "Canceled", or an error message
	ok     bool
	folder string
}

// historyView lists this session's transfers, newest first. Nothing is
// persisted to disk.
type historyView struct {
	a       *App
	content fyne.CanvasObject
	entries []*historyEntry
	list    *widget.List
	empty   *widget.Label
}

func newHistoryView(a *App) *historyView {
	h := &historyView{a: a}
	h.list = widget.NewList(
		func() int { return len(h.entries) },
		func() fyne.CanvasObject {
			title := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			title.Truncation = fyne.TextTruncateEllipsis
			detail := widget.NewLabel("")
			detail.Truncation = fyne.TextTruncateEllipsis
			open := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), nil)
			open.Importance = widget.LowImportance
			return container.NewBorder(nil, nil, widget.NewIcon(theme.UploadIcon()), open,
				container.NewVBox(title, detail))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			e := h.entries[i]
			row := o.(*fyne.Container)
			texts := row.Objects[0].(*fyne.Container)
			icon := row.Objects[1].(*widget.Icon)
			open := row.Objects[2].(*widget.Button)

			verb := "Received"
			icon.SetResource(theme.DownloadIcon())
			if e.sent {
				verb = "Sent"
				icon.SetResource(theme.UploadIcon())
			}
			title := verb + " " + describe(e.names)
			if e.bytes > 0 {
				title += " (" + transfer.FormatBytes(e.bytes) + ")"
			}
			texts.Objects[0].(*widget.Label).SetText(title)

			status := e.status
			switch {
			case status == "":
				status = "In progress…"
			case !e.ok && status != "Canceled":
				status = "Failed: " + status
			}
			detail := e.when.Format("15:04:05") + "  •  " + status
			if e.code != "" {
				detail += "  •  code " + e.code
			}
			texts.Objects[1].(*widget.Label).SetText(detail)

			if e.folder != "" {
				open.Show()
				open.OnTapped = func() { h.a.openFolder(e.folder) }
			} else {
				open.Hide()
			}
		},
	)
	h.list.OnSelected = func(widget.ListItemID) { h.list.UnselectAll() }
	h.empty = widget.NewLabelWithStyle("Transfers you make this session will show up here.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	h.content = container.NewPadded(container.NewStack(h.list, container.NewCenter(h.empty)))
	return h
}

func (h *historyView) add(sent bool, paths []string) *historyEntry {
	e := &historyEntry{sent: sent, when: time.Now()}
	for _, p := range paths {
		e.names = append(e.names, filepath.Base(p))
	}
	h.entries = append([]*historyEntry{e}, h.entries...)
	h.empty.Hide()
	h.list.Refresh()
	return e
}

func (h *historyView) finish(e *historyEntry, res transfer.Result, err error) {
	e.code = res.Code
	e.bytes = res.Bytes
	if !e.sent {
		e.names = nil
		for _, f := range res.Files {
			e.names = append(e.names, filepath.Base(f))
		}
		if len(res.Files) > 0 {
			e.folder = filepath.Dir(res.Files[0])
		}
	}
	switch {
	case err == nil:
		e.ok, e.status = true, "Done"
	case errors.Is(err, transfer.ErrCanceled):
		e.status = "Canceled"
	default:
		e.status = err.Error()
	}
	h.list.Refresh()
}

func describe(names []string) string {
	switch len(names) {
	case 0:
		return "files"
	case 1:
		return names[0]
	}
	return fmt.Sprintf("%s and %d more", names[0], len(names)-1)
}
