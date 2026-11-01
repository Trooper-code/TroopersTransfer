package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/troopers/transfer-service/transfer"
)

// progressPanel sits under the tabs and is visible only during a transfer.
type progressPanel struct {
	content *fyne.Container
	title   *widget.Label
	status  *widget.Label
	file    *widget.Label
	bar     *widget.ProgressBar
	wait    *widget.ProgressBarInfinite
	stats   *widget.Label
	cancel  *widget.Button
}

func newProgressPanel(onCancel func()) *progressPanel {
	p := &progressPanel{
		title:  widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		status: widget.NewLabel(""),
		file:   widget.NewLabel(""),
		bar:    widget.NewProgressBar(),
		wait:   widget.NewProgressBarInfinite(),
		stats:  widget.NewLabel(""),
	}
	p.file.Truncation = fyne.TextTruncateEllipsis
	p.cancel = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), onCancel)
	p.cancel.Importance = widget.DangerImportance

	header := container.NewBorder(nil, nil, p.title, p.cancel, p.status)
	bars := container.NewStack(p.bar, p.wait)
	p.content = container.NewPadded(newBox(container.NewVBox(header, bars, p.file, p.stats)))
	p.content.Hide()
	return p
}

func (p *progressPanel) start(title string) {
	p.title.SetText(title)
	p.status.SetText(transfer.PhasePreparing.String())
	p.file.SetText("")
	p.stats.SetText("")
	p.bar.SetValue(0)
	p.bar.Hide()
	p.wait.Show()
	p.wait.Start()
	p.cancel.Enable()
	p.content.Show()
}

func (p *progressPanel) stop() {
	p.wait.Stop()
	p.content.Hide()
}

func (p *progressPanel) update(pr transfer.Progress) {
	p.status.SetText(pr.Phase.String())
	if pr.Phase != transfer.PhaseTransferring || pr.BytesTotal == 0 {
		p.bar.Hide()
		p.wait.Show()
		p.wait.Start()
		return
	}
	p.wait.Stop()
	p.wait.Hide()
	p.bar.Show()
	p.bar.SetValue(float64(pr.BytesDone) / float64(pr.BytesTotal))
	if pr.FileCount > 1 {
		p.file.SetText(fmt.Sprintf("File %d of %d: %s", pr.FileIndex, pr.FileCount, pr.CurrentFile))
	} else {
		p.file.SetText(pr.CurrentFile)
	}
	stats := fmt.Sprintf("%s of %s", transfer.FormatBytes(pr.BytesDone), transfer.FormatBytes(pr.BytesTotal))
	if pr.Speed > 0 {
		stats += fmt.Sprintf("  •  %s/s", transfer.FormatBytes(int64(pr.Speed)))
	}
	if pr.ETA > 0 {
		stats += "  •  " + formatETA(pr.ETA) + " left"
	}
	p.stats.SetText(stats)
}

func formatETA(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}
