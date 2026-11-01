package ui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/troopers/transfer-service/updater"
)

// updatesView shows the app and croc versions and installs new releases.
// croc is compiled into the app, so a croc upgrade arrives as an app update.
type updatesView struct {
	a       *App
	content fyne.CanvasObject

	crocLine *widget.Label
	status   *widget.Label
	check    *widget.Button
	install  *widget.Button
	bar      *widget.ProgressBar
	notes    *widget.Accordion
	notesTxt *widget.Label

	release *updater.Release
}

func newUpdatesView(a *App) *updatesView {
	v := &updatesView{a: a}

	bundled := updater.BundledCroc()
	if bundled == "" {
		bundled = "unknown"
	}
	versions := widget.NewLabel(fmt.Sprintf("This version: %s  •  croc built in: %s", Version, bundled))
	versions.TextStyle = fyne.TextStyle{Bold: true}
	v.crocLine = wrapLabel("")
	v.crocLine.Hide()
	v.status = wrapLabel("")
	v.status.Hide()

	v.check = widget.NewButtonWithIcon("Check for updates", theme.ViewRefreshIcon(), v.runCheck)
	v.check.Importance = widget.HighImportance
	v.install = widget.NewButtonWithIcon("Update and restart", theme.DownloadIcon(), v.runInstall)
	v.install.Importance = widget.SuccessImportance
	v.install.Hide()
	releases := widget.NewButtonWithIcon("Releases page", theme.ComputerIcon(), func() {
		if u, err := url.Parse(updater.ReleasesPage); err == nil {
			_ = a.fa.OpenURL(u)
		}
	})
	v.bar = widget.NewProgressBar()
	v.bar.Hide()
	v.notesTxt = wrapLabel("")
	v.notes = widget.NewAccordion(widget.NewAccordionItem("What's new", v.notesTxt))
	v.notes.Hide()

	v.content = section("Updates",
		"croc is built into Troopers Transfer, so new croc versions arrive as app updates. "+
			"Check for updates downloads the newest release from GitHub and restarts the app. Nothing is checked unless you click it.",
		versions, v.crocLine, container.NewHBox(v.check, v.install, releases), v.bar, v.status, v.notes)
	return v
}

func (v *updatesView) setStatus(msg string, tone widget.Importance) {
	v.status.Importance = tone
	v.status.SetText(msg)
	v.status.Show()
}

func (v *updatesView) runCheck() {
	v.check.Disable()
	v.install.Hide()
	v.notes.Hide()
	v.setStatus("Checking GitHub…", widget.MediumImportance)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rel, err := updater.Latest(ctx, updater.Repo)
		croc, cerr := updater.LatestCroc(ctx)
		fyne.Do(func() {
			v.check.Enable()
			v.showCroc(croc, cerr)
			if err != nil {
				v.setStatus(err.Error(), widget.DangerImportance)
				return
			}
			v.showRelease(rel)
		})
	}()
}

func (v *updatesView) showCroc(latest string, err error) {
	if err != nil || latest == "" {
		v.crocLine.Hide()
		return
	}
	bundled := updater.BundledCroc()
	switch {
	case bundled == "":
		v.crocLine.SetText("Latest croc release: " + latest + ".")
	case updater.Newer(latest, bundled):
		v.crocLine.SetText(fmt.Sprintf("A newer croc (%s) has been released. This app has %s; the newer croc will come with a future Troopers Transfer update.", latest, bundled))
	default:
		v.crocLine.SetText(fmt.Sprintf("croc %s is the latest croc release.", bundled))
	}
	v.crocLine.Show()
}

func (v *updatesView) showRelease(rel *updater.Release) {
	v.release = rel
	if notes := strings.TrimSpace(rel.Notes); notes != "" {
		v.notesTxt.SetText(notes)
		v.notes.Show()
	}
	newer := updater.Newer(rel.Version, Version)
	switch {
	case Version == "dev":
		v.setStatus(fmt.Sprintf("This is a development build. The latest release is %s.", rel.Tag), widget.MediumImportance)
	case !newer:
		v.setStatus(fmt.Sprintf("You're up to date. %s is the latest release.", rel.Tag), widget.SuccessImportance)
		v.notes.Hide()
	case rel.Asset == nil:
		v.setStatus(fmt.Sprintf("Version %s is out, but it has no download for your system yet. Get it from the releases page.", rel.Tag), widget.WarningImportance)
	default:
		v.setStatus(fmt.Sprintf("Version %s is available.", rel.Tag), widget.SuccessImportance)
		v.install.SetText("Update to " + rel.Tag + " and restart")
		v.install.Show()
	}
}

func (v *updatesView) runInstall() {
	rel := v.release
	if rel == nil {
		return
	}
	var warn []string
	if v.a.busy() {
		warn = append(warn, "the running transfer will be canceled")
	}
	if v.a.hosting() {
		warn = append(warn, "hosting will stop (friends will need to reconnect after the restart)")
	}
	if len(warn) > 0 {
		dialog.ShowConfirm("Update now?", "Updating restarts Troopers Transfer, so "+strings.Join(warn, ", and ")+".",
			func(ok bool) {
				if ok {
					v.doInstall(rel)
				}
			}, v.a.win)
		return
	}
	v.doInstall(rel)
}

func (v *updatesView) doInstall(rel *updater.Release) {
	v.check.Disable()
	v.install.Disable()
	v.bar.SetValue(0)
	v.bar.Show()
	v.setStatus("Downloading "+rel.Tag+"…", widget.MediumImportance)
	go func() {
		exe, err := updater.Install(context.Background(), rel, func(done, total int64) {
			if total > 0 {
				fyne.Do(func() { v.bar.SetValue(float64(done) / float64(total)) })
			}
		})
		fyne.Do(func() {
			v.bar.Hide()
			v.check.Enable()
			v.install.Enable()
			if err != nil {
				v.setStatus("The update wasn't installed. Your current version still works.", widget.DangerImportance)
				v.a.showError("Update failed", err)
				return
			}
			v.setStatus("Installed. Restarting…", widget.SuccessImportance)
			v.a.cancelTransfer()
			v.a.stopHosting()
			if err := updater.Restart(exe); err != nil {
				v.a.showError("Restart failed", fmt.Errorf("the update is installed; close and reopen Troopers Transfer to use it (%w)", err))
				return
			}
			v.a.fa.Quit()
		})
	}()
}
