// Package ui is the Fyne front end: Send, Receive, Host, Connect, History and
// Settings tabs around the transfer and relay packages.
package ui

import (
	"context"
	"errors"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/troopers/transfer-service/assets"
	"github.com/troopers/transfer-service/config"
	"github.com/troopers/transfer-service/relay"
	"github.com/troopers/transfer-service/transfer"
)

// Version is set at build time with -ldflags "-X .../ui.Version=1.2.3".
var Version = "dev"

type App struct {
	fa  fyne.App
	win fyne.Window
	cfg *config.Config

	tabs         *container.AppTabs
	sendTab      *container.TabItem
	send         *sendView
	recv         *receiveView
	progress     *progressPanel
	history      *historyView
	hostView     *hostView
	connect      *connectView
	settingsView *settingsView

	host *relay.Host // non-nil while hosting a relay

	root   *fyne.Container
	cancel context.CancelFunc // non-nil while a transfer runs
}

// Run builds the window and blocks until it is closed.
func Run() {
	newApp(app.NewWithID("com.troopers.transfer")).win.ShowAndRun()
}

func newApp(fa fyne.App) *App {
	icon := fyne.NewStaticResource("icon.png", assets.Icon)
	fa.SetIcon(icon)

	a := &App{fa: fa, cfg: config.Load()}
	a.win = fa.NewWindow(config.AppName)
	a.win.SetIcon(icon)

	a.progress = newProgressPanel(a.cancelTransfer)
	a.history = newHistoryView(a)
	a.send = newSendView(a)
	a.recv = newReceiveView(a)
	a.settingsView = newSettingsView(a)
	a.connect = newConnectView(a)
	a.hostView = newHostView(a)

	a.sendTab = container.NewTabItemWithIcon("Send", theme.UploadIcon(), a.send.content)
	a.tabs = container.NewAppTabs(
		a.sendTab,
		container.NewTabItemWithIcon("Receive", theme.DownloadIcon(), a.recv.content),
		container.NewTabItemWithIcon("Host", theme.ComputerIcon(), a.hostView.content),
		container.NewTabItemWithIcon("Connect", theme.LoginIcon(), a.connect.content),
		container.NewTabItemWithIcon("History", theme.HistoryIcon(), a.history.content),
		container.NewTabItemWithIcon("Settings", theme.SettingsIcon(), a.settingsView.content),
	)

	a.root = container.NewBorder(nil, a.progress.content, nil, nil, a.tabs)
	a.win.SetContent(a.root)
	a.win.Resize(fyne.NewSize(760, 620))
	a.win.CenterOnScreen()

	a.win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		if a.busy() {
			return
		}
		a.tabs.Select(a.sendTab)
		for _, u := range uris {
			a.send.add(uriPath(u))
		}
	})
	a.win.SetCloseIntercept(func() {
		if !a.busy() {
			a.stopHosting()
			a.win.Close()
			return
		}
		dialog.ShowConfirm("Transfer in progress",
			"A transfer is still running. Cancel it and quit?",
			func(ok bool) {
				if ok {
					a.cancelTransfer()
					a.stopHosting()
					a.win.Close()
				}
			}, a.win)
	})
	// Close the router ports however the app exits.
	fa.Lifecycle().SetOnStopped(a.stopHosting)

	return a
}

// settings picks the relay for a transfer: the one this PC is hosting, else
// the one chosen in Connect / Settings, else the public pool.
func (a *App) settings() transfer.Settings {
	s := transfer.Settings{
		RelayAddress:  a.cfg.RelayAddress,
		RelayPassword: a.cfg.RelayPassword,
		ZipFolders:    a.cfg.ZipFolders,
	}
	if a.host != nil {
		st := a.host.Status()
		s.RelayAddress = st.OwnAddress()
		s.RelayPassword = st.Password
	}
	return s
}

func (a *App) hosting() bool { return a.host != nil }

// setHost records a newly started relay. Must be called on the UI goroutine.
func (a *App) setHost(h *relay.Host) {
	a.host = h
	a.connect.refresh()
}

// stopHosting shuts down the relay and closes the router ports, if hosting.
func (a *App) stopHosting() {
	if a.host == nil {
		return
	}
	a.host.Stop()
	a.host = nil
	if a.connect != nil {
		a.connect.refresh()
	}
}

// relayChanged refreshes every view that shows the chosen relay.
func (a *App) relayChanged() {
	a.connect.refresh()
	a.settingsView.refresh()
}

func (a *App) busy() bool { return a.cancel != nil }

// begin marks a transfer as running and returns its context. Must be called
// on the UI goroutine.
func (a *App) begin(title string) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.progress.start(title)
	a.root.Refresh() // re-layout now that the progress panel is visible
	a.send.setBusy(true)
	a.recv.setBusy(true)
	return ctx
}

// end clears the running transfer. Must be called on the UI goroutine.
func (a *App) end() {
	if a.cancel != nil {
		a.cancel()
	}
	a.cancel = nil
	a.progress.stop()
	a.root.Refresh()
	a.send.setBusy(false)
	a.recv.setBusy(false)
}

func (a *App) cancelTransfer() {
	if a.cancel != nil {
		a.cancel()
	}
}

// showError displays a friendly message with the technical details tucked
// away underneath.
func (a *App) showError(title string, err error) {
	if errors.Is(err, transfer.ErrCanceled) {
		return
	}
	msg := widget.NewLabel(err.Error())
	msg.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(msg)
	var fe *transfer.FriendlyError
	if errors.As(err, &fe) && fe.Details() != "" && fe.Details() != fe.Message {
		details := widget.NewLabel(fe.Details())
		details.Wrapping = fyne.TextWrapWord
		details.TextStyle = fyne.TextStyle{Monospace: true}
		content.Add(widget.NewAccordion(widget.NewAccordionItem("Technical details", details)))
	}
	d := dialog.NewCustom(title, "OK", content, a.win)
	d.Resize(fyne.NewSize(460, 0))
	d.Show()
}

func (a *App) openFolder(path string) {
	u, err := url.Parse(storage.NewFileURI(path).String())
	if err == nil {
		_ = a.fa.OpenURL(u)
	}
}
