package ui

import (
	"net"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/troopers/transfer-service/config"
)

type settingsView struct {
	content fyne.CanvasObject
	refresh func() // reload the relay fields after Connect changes them
}

func newSettingsView(a *App) *settingsView {
	cfg := a.cfg

	relay := widget.NewEntry()
	relay.SetPlaceHolder("Blank = public croc relays (e.g. myrelay.example.com:9009)")
	relay.SetText(cfg.RelayAddress)

	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("Blank = croc default")
	pass.SetText(cfg.RelayPassword)

	zip := widget.NewCheck("Zip folders before sending (faster for many small files)", nil)
	zip.SetChecked(cfg.ZipFolders)

	folder := cfg.ReceiveFolder
	folderLabel := widget.NewLabel(cfg.DestinationOrDefault())
	folderLabel.Truncation = fyne.TextTruncateEllipsis
	chooseFolder := widget.NewButtonWithIcon("Change…", theme.FolderOpenIcon(), func() {
		d := dialog.NewFolderOpen(func(u fyne.ListableURI, err error) {
			if err == nil && u != nil {
				folder = uriPath(u)
				folderLabel.SetText(folder)
			}
		}, a.win)
		if l, err := storage.ListerForURI(storage.NewFileURI(cfg.DestinationOrDefault())); err == nil {
			d.SetLocation(l)
		}
		d.Resize(dialogSize(a.win))
		d.Show()
	})
	resetFolder := widget.NewButtonWithIcon("Default", theme.ViewRefreshIcon(), func() {
		folder = ""
		folderLabel.SetText(config.DefaultReceiveFolder())
	})

	where := widget.NewLabel("")
	where.Wrapping = fyne.TextWrapWord
	where.Importance = widget.LowImportance
	showWhere := func() { where.SetText("Settings file: " + cfg.Path()) }
	showWhere()

	save := widget.NewButtonWithIcon("Save settings", theme.DocumentSaveIcon(), func() {
		addr := strings.TrimSpace(relay.Text)
		if addr != "" {
			if _, port, err := net.SplitHostPort(addr); err != nil || port == "" {
				dialog.ShowInformation("Check the relay address",
					"Enter the relay as host:port, for example myrelay.example.com:9009.", a.win)
				return
			}
		}
		cfg.RelayAddress = addr
		cfg.RelayPassword = pass.Text
		cfg.ZipFolders = zip.Checked
		cfg.ReceiveFolder = folder
		if err := cfg.Save(); err != nil {
			dialog.ShowError(err, a.win)
			return
		}
		a.recv.setDest(cfg.DestinationOrDefault())
		if a.connect != nil {
			a.connect.refresh()
		}
		showWhere()
		dialog.ShowInformation("Saved", "Your settings were saved.", a.win)
	})
	save.Importance = widget.HighImportance

	relayHelp := "Only change these if you run your own croc relay. Both computers must use the same relay and password. " +
		"The Connect tab fills these in for you from a friend's connection code."

	form := widget.NewForm(
		widget.NewFormItem("Relay address", relay),
		widget.NewFormItem("Relay password", pass),
	)
	transfers := widget.NewForm(
		widget.NewFormItem("", zip),
		widget.NewFormItem("Default save folder", container.NewBorder(nil, nil, nil,
			container.NewHBox(chooseFolder, resetFolder), folderLabel)),
	)
	about := widget.NewLabel(config.AppName + " " + Version + " • built on croc (github.com/schollz/croc)")
	about.Importance = widget.LowImportance

	content := scrollPage("Settings", "Changes to the relay and transfer settings are saved when you click Save settings.",
		newAppearanceSection(a),
		section("Relay", relayHelp, form),
		section("Transfers", "", transfers),
		container.NewHBox(save),
		newUpdatesView(a).content,
		section("About", "", where, about),
	)
	return &settingsView{content: content, refresh: func() {
		relay.SetText(cfg.RelayAddress)
		pass.SetText(cfg.RelayPassword)
	}}
}
