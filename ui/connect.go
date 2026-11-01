package ui

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/troopers/transfer-service/relay"
)

// connectView lets the user point the app at a relay a friend is hosting,
// using the connection code from the friend's Host tab.
type connectView struct {
	a       *App
	content fyne.CanvasObject

	current *widget.Label
	note    *widget.Label
	code    *widget.Entry
	paste   *widget.Button
	test    *widget.Button
	connect *widget.Button
	public  *widget.Button
	result  *widget.Label
	testing bool
}

func newConnectView(a *App) *connectView {
	v := &connectView{a: a}

	intro := widget.NewLabel("Connect to a relay a friend is hosting. Ask them for the connection code from their Host tab, paste it here, and click Connect. " +
		"Both of you will then send and receive through their relay instead of the public ones.")
	intro.Wrapping = fyne.TextWrapWord

	v.current = widget.NewLabel("")
	v.current.Wrapping = fyne.TextWrapWord
	v.current.TextStyle = fyne.TextStyle{Bold: true}
	v.note = widget.NewLabel("")
	v.note.Wrapping = fyne.TextWrapWord
	v.note.Importance = widget.LowImportance

	v.code = widget.NewEntry()
	v.code.SetPlaceHolder("e.g. 203.0.113.5:9009/amber-falcon-ridge")
	v.code.OnChanged = func(string) { v.refreshButtons() }
	v.code.OnSubmitted = func(string) { v.run(true) }
	v.paste = widget.NewButtonWithIcon("Paste", theme.ContentPasteIcon(), func() {
		v.code.SetText(strings.TrimSpace(a.fa.Clipboard().Content()))
	})

	v.test = widget.NewButtonWithIcon("Test connection", theme.SearchIcon(), func() { v.run(false) })
	v.connect = widget.NewButtonWithIcon("Connect", theme.LoginIcon(), func() { v.run(true) })
	v.connect.Importance = widget.HighImportance
	v.public = widget.NewButtonWithIcon("Use public relays", theme.ViewRefreshIcon(), v.usePublic)

	v.result = widget.NewLabel("")
	v.result.Wrapping = fyne.TextWrapWord

	form := widget.NewForm(widget.NewFormItem("Connection code",
		container.NewBorder(nil, nil, nil, v.paste, v.code)))

	v.content = scrollPage("Connect", intro.Text,
		section("Current relay", "", v.current, v.note, container.NewHBox(v.public)),
		section("Connect to a friend's relay", "Paste the connection code from their Host tab.",
			form, container.NewHBox(v.connect, v.test), v.result),
	)
	v.refresh()
	return v
}

// refresh updates the "currently using" line from the saved settings and
// hosting state.
func (v *connectView) refresh() {
	switch {
	case v.a.cfg.RelayAddress == "":
		v.current.SetText("Currently using: the public croc relays")
	default:
		v.current.SetText("Currently using: the relay at " + v.a.cfg.RelayAddress)
	}
	if v.a.hosting() {
		v.note.SetText("You're hosting a relay right now, so your own transfers go through it. This setting applies again when you stop hosting.")
		v.note.Show()
	} else {
		v.note.Hide()
	}
	v.refreshButtons()
}

func (v *connectView) refreshButtons() {
	has := strings.TrimSpace(v.code.Text) != ""
	for _, b := range []*widget.Button{v.test, v.connect} {
		if has && !v.testing {
			b.Enable()
		} else {
			b.Disable()
		}
	}
	if v.a.cfg.RelayAddress == "" || v.testing {
		v.public.Disable()
	} else {
		v.public.Enable()
	}
}

// run tests the code and, when save is true and the test passes, makes it
// the relay used for transfers.
func (v *connectView) run(save bool) {
	if v.testing {
		return
	}
	addr, pass, err := relay.ParseCode(v.code.Text)
	if err != nil {
		v.setResult(false, "That code doesn't look right: "+err.Error()+". It should look like 203.0.113.5:9009/amber-falcon-ridge.")
		return
	}
	v.testing = true
	v.refreshButtons()
	v.result.Importance = widget.MediumImportance
	v.result.SetText("Testing the connection to " + addr + "…")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := relay.Test(ctx, addr, pass)
		fyne.Do(func() {
			v.testing = false
			if err != nil {
				v.setResult(false, err.Error())
				v.refreshButtons()
				return
			}
			if !save {
				v.setResult(true, "The relay at "+addr+" is reachable and the password is right. Click Connect to use it.")
				v.refreshButtons()
				return
			}
			v.a.cfg.RelayAddress = addr
			v.a.cfg.RelayPassword = pass
			if err := v.a.cfg.Save(); err != nil {
				dialog.ShowError(err, v.a.win)
			}
			v.a.relayChanged()
			v.code.SetText("")
			v.setResult(true, "Connected. Your transfers now go through the relay at "+addr+". "+
				"Your friend needs to use the same relay, so make sure they're hosting it or connected to it too.")
		})
	}()
}

func (v *connectView) usePublic() {
	v.a.cfg.RelayAddress = ""
	v.a.cfg.RelayPassword = ""
	if err := v.a.cfg.Save(); err != nil {
		dialog.ShowError(err, v.a.win)
	}
	v.a.relayChanged()
	v.setResult(true, "Switched back to the public croc relays.")
}

func (v *connectView) setResult(ok bool, msg string) {
	if ok {
		v.result.Importance = widget.SuccessImportance
	} else {
		v.result.Importance = widget.DangerImportance
	}
	v.result.SetText(msg)
}
