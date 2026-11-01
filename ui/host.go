package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/troopers/transfer-service/relay"
)

const (
	routerGuidesURL = "https://portforward.com/router.htm"
	portCheckURL    = "https://canyouseeme.org/"
)

// hostView runs a relay on this PC for friends to connect to.
type hostView struct {
	a       *App
	content fyne.CanvasObject

	port     *widget.Entry
	pass     *widget.Entry
	newPass  *widget.Button
	autoOpen *widget.Check
	startBtn *widget.Button
	status   *fyne.Container

	starting bool
}

func newHostView(a *App) *hostView {
	v := &hostView{a: a}
	cfg := a.cfg

	intro := widget.NewLabel("Host your own relay so you and your friends can transfer through your PC instead of the public relays. " +
		"Start hosting, then send your friend the connection code. They paste it into their Connect tab.")
	intro.Wrapping = fyne.TextWrapWord

	v.port = widget.NewEntry()
	v.port.SetText(strconv.Itoa(cfg.HostPortOrDefault()))
	v.port.Validator = func(s string) error {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || !relay.ValidPort(n) {
			return fmt.Errorf("use a number from 1024 to %d", 65535-relay.PortCount+1)
		}
		return nil
	}

	v.pass = widget.NewEntry()
	if cfg.HostPassword == "" {
		cfg.HostPassword = relay.NewPassword()
	}
	v.pass.SetText(cfg.HostPassword)
	v.newPass = widget.NewButtonWithIcon("New", theme.ViewRefreshIcon(), func() {
		v.pass.SetText(relay.NewPassword())
	})

	v.autoOpen = widget.NewCheck("Open the ports on my router automatically (UPnP)", nil)
	v.autoOpen.SetChecked(!cfg.HostNoAutoOpen)

	v.startBtn = widget.NewButtonWithIcon("Start hosting", theme.MediaPlayIcon(), v.toggle)
	v.startBtn.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Starting port", v.port),
		widget.NewFormItem("Relay password", container.NewBorder(nil, nil, nil, v.newPass, v.pass)),
		widget.NewFormItem("", v.autoOpen),
	)
	portsHint := widget.NewLabel("")
	portsHint.Importance = widget.LowImportance
	portsHint.Wrapping = fyne.TextWrapWord
	showHint := func(string) {
		n, err := strconv.Atoi(strings.TrimSpace(v.port.Text))
		if err != nil || !relay.ValidPort(n) {
			portsHint.SetText("")
			return
		}
		portsHint.SetText(fmt.Sprintf("The relay uses %d ports in a row: TCP %d–%d.", relay.PortCount, n, n+relay.PortCount-1))
	}
	v.port.OnChanged = showHint
	showHint("")

	v.status = container.NewVBox()

	v.content = scrollPage("Host", intro.Text,
		section("Relay settings", "", form, portsHint, v.startBtn),
		section("Status", "", v.status),
	)
	v.render()
	return v
}

func (v *hostView) setInputsEnabled(on bool) {
	for _, w := range []fyne.Disableable{v.port, v.pass, v.newPass, v.autoOpen} {
		if on {
			w.Enable()
		} else {
			w.Disable()
		}
	}
}

func (v *hostView) toggle() {
	if v.starting {
		return
	}
	if v.a.hosting() {
		v.a.stopHosting()
		v.render()
		return
	}
	if err := v.port.Validate(); err != nil {
		dialog.ShowInformation("Check the starting port", "Use a number from 1024 to 65531. The default is 9009.", v.a.win)
		return
	}
	port, _ := strconv.Atoi(strings.TrimSpace(v.port.Text))
	pass := strings.TrimSpace(v.pass.Text)
	if pass == "" {
		pass = relay.NewPassword()
		v.pass.SetText(pass)
	}
	if strings.ContainsAny(pass, " /") {
		dialog.ShowInformation("Check the password", "The relay password can't contain spaces or slashes.", v.a.win)
		return
	}

	cfg := v.a.cfg
	cfg.HostPort, cfg.HostPassword, cfg.HostNoAutoOpen = port, pass, !v.autoOpen.Checked
	_ = cfg.Save()

	v.starting = true
	v.setInputsEnabled(false)
	v.startBtn.Disable()
	v.startBtn.SetText("Starting…")
	msg := "Starting the relay…"
	if v.autoOpen.Checked {
		msg = "Starting the relay and asking your router to open the ports. This usually takes a few seconds…"
	}
	v.status.Objects = []fyne.CanvasObject{wrapLabel(msg)}
	v.status.Refresh()

	opts := relay.Options{Port: port, Password: pass, AutoOpen: v.autoOpen.Checked}
	go func() {
		h, err := relay.Start(context.Background(), opts)
		fyne.Do(func() {
			v.starting = false
			v.startBtn.Enable()
			if err != nil {
				v.render()
				var se *relay.StartError
				if errors.As(err, &se) {
					v.showProblemDialog(se.Problem)
				} else {
					dialog.ShowInformation("Couldn't start hosting", err.Error(), v.a.win)
				}
				return
			}
			v.a.setHost(h)
			v.render()
		})
	}()
}

func (v *hostView) retry() {
	h := v.a.host
	if h == nil || v.starting {
		return
	}
	v.starting = true
	v.startBtn.Disable()
	v.status.Objects = []fyne.CanvasObject{wrapLabel("Checking your router again…")}
	v.status.Refresh()
	go func() {
		h.Retry(context.Background())
		fyne.Do(func() {
			v.starting = false
			v.startBtn.Enable()
			v.render()
		})
	}()
}

// render rebuilds the status area for the current state.
func (v *hostView) render() {
	h := v.a.host
	if h == nil {
		v.setInputsEnabled(true)
		v.startBtn.SetText("Start hosting")
		v.startBtn.SetIcon(theme.MediaPlayIcon())
		v.status.Objects = []fyne.CanvasObject{wrapLabel(
			"Not hosting. Windows may ask whether to allow Troopers Transfer through the firewall when you start. Click Allow, or friends won't be able to connect.")}
		v.status.Refresh()
		return
	}
	v.setInputsEnabled(false)
	v.startBtn.SetText("Stop hosting")
	v.startBtn.SetIcon(theme.MediaStopIcon())

	st := h.Status()
	objs := []fyne.CanvasObject{
		widget.NewLabelWithStyle(fmt.Sprintf("Hosting on TCP ports %d–%d", st.Port, st.Port+relay.PortCount-1),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		wrapLabel("Your own transfers now go through this relay too. Stop hosting to switch back."),
	}

	internet := st.InternetCode()
	if internet != "" && (st.Problem == nil || st.Problem.CanForward) {
		objs = append(objs, codeRow(v.a, "Connection code for friends over the internet", internet))
	}
	if local := st.LocalCode(); local != "" {
		objs = append(objs, codeRow(v.a, "Connection code for friends on your Wi-Fi / home network", local))
	}
	objs = append(objs, widget.NewSeparator())

	switch {
	case st.Problem != nil:
		objs = append(objs, v.problemCard(st.Problem, true))
	case st.Opened:
		ok := wrapLabel(fmt.Sprintf("Ports opened automatically (%s). Friends anywhere should be able to connect. Ask them to click Test connection in their Connect tab.", st.Method))
		ok.Importance = widget.SuccessImportance
		objs = append(objs, ok)
	case !st.Checked:
		objs = append(objs, wrapLabel(fmt.Sprintf(
			"Automatic port opening is off. Friends outside your home network can connect if you've forwarded TCP ports %d–%d on your router to this PC (%s).",
			st.Port, st.Port+relay.PortCount-1, orUnknown(st.LocalIP))),
			wrapLabel(fmt.Sprintf("To check, click Check my ports online. On the page that opens, enter port %d and click Check Port while you're hosting. "+
				"Success means friends can reach you. Error means the router rule isn't working yet, or your internet provider doesn't give you your own public address.", st.Port)),
			container.NewHBox(widget.NewButtonWithIcon("Check my ports online", theme.SearchIcon(), func() {
				if u, err := url.Parse(portCheckURL); err == nil {
					_ = v.a.fa.OpenURL(u)
				}
			})))
	}
	if internet == "" && (st.Problem == nil || st.Problem.CanForward) {
		objs = append(objs, wrapLabel("Couldn't look up your public IP address, so there's no internet connection code yet. Check your internet connection and click Try again."))
		if st.Problem == nil {
			objs = append(objs, widget.NewButtonWithIcon("Try again", theme.ViewRefreshIcon(), v.retry))
		}
	}
	v.status.Objects = objs
	v.status.Refresh()
}

// problemCard lays out a Problem with numbered steps and actions.
func (v *hostView) problemCard(p *relay.Problem, withRetry bool) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(p.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Importance = widget.WarningImportance
	box := container.NewVBox(title, wrapLabel(p.Summary))
	for i, s := range p.Steps {
		box.Add(wrapLabel(fmt.Sprintf("%d.  %s", i+1, s)))
	}
	if p.Tip != "" {
		tip := wrapLabel("Tip: " + p.Tip)
		tip.TextStyle = fyne.TextStyle{Italic: true}
		box.Add(tip)
	}
	buttons := container.NewHBox(widget.NewButtonWithIcon("Copy these steps", theme.ContentCopyIcon(), func() {
		v.a.fa.Clipboard().SetContent(p.Text())
	}))
	if withRetry && p.CanForward && v.a.host != nil {
		opened := widget.NewButtonWithIcon("I've opened the ports", theme.ConfirmIcon(), v.manualMode)
		opened.Importance = widget.HighImportance
		buttons.Add(opened)
	}
	if withRetry {
		buttons.Add(widget.NewButtonWithIcon("Try again", theme.ViewRefreshIcon(), v.retry))
	}
	if p.CanForward {
		buttons.Add(widget.NewButtonWithIcon("Router guides", theme.HelpIcon(), func() {
			if u, err := url.Parse(routerGuidesURL); err == nil {
				_ = v.a.fa.OpenURL(u)
			}
		}))
	}
	box.Add(buttons)
	if p.Details != "" {
		d := wrapLabel(p.Details)
		d.TextStyle = fyne.TextStyle{Monospace: true}
		box.Add(widget.NewAccordion(widget.NewAccordionItem("Technical details", d)))
	}
	return box
}

// manualMode is for when the user has forwarded the ports by hand: stop
// asking the router (it won't answer) and restart hosting without UPnP.
func (v *hostView) manualMode() {
	if v.starting {
		return
	}
	v.a.stopHosting()
	v.autoOpen.SetChecked(false)
	v.toggle()
}

func (v *hostView) showProblemDialog(p *relay.Problem) {
	d := dialog.NewCustom(p.Title, "OK", v.problemCard(p, false), v.a.win)
	d.Resize(fyne.NewSize(560, 0))
	d.Show()
}

func codeRow(a *App, label, code string) fyne.CanvasObject {
	l := widget.NewLabel(label)
	l.Importance = widget.LowImportance
	e := widget.NewLabel(code)
	e.Selectable = true
	e.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	e.Truncation = fyne.TextTruncateEllipsis
	copyBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		a.fa.Clipboard().SetContent(code)
	})
	return container.NewVBox(l, container.NewBorder(nil, nil, nil, copyBtn, e))
}

func wrapLabel(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	return l
}

func orUnknown(s string) string {
	if s == "" {
		return "IP address unknown"
	}
	return s
}
