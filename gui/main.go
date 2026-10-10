// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Command manga-downloader-gui is the desktop front end of the CLI. It lives
// in its own module so Fyne's cgo dependencies never reach `go install` of
// the CLI; see AGENTS.md.
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
)

// Version and Tag are set at link time by the release workflow (-X), the same
// way the CLI gets its version: Tag is the release tag, Version a describe
// string. Both stay at their defaults in a plain build.
var (
	Version = "dev"
	Tag     = ""
)

// appID is the reverse-DNS identifier Fyne derives preferences and the
// desktop entry from; changing it orphans saved options
const appID = "com.github.elboletaire.mangadownloader"

func main() {
	a := app.NewWithID(appID)
	a.SetIcon(appIcon)
	a.Settings().SetTheme(newAppTheme())

	w := a.NewWindow("Manga Downloader")
	settings := defaultSettings()
	loadOptions(a.Preferences(), settings)

	// the controller signals through a one-slot channel: a pending signal
	// already covers any number of further changes
	changed := make(chan struct{}, 1)
	ctrl := NewController(settings, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	v := newView(a, w, ctrl, changed)
	w.SetContent(v.content)
	w.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("Help",
		fyne.NewMenuItem("About", func() {
			dialog.ShowInformation("About Manga Downloader", aboutText(), w)
		}),
	)))
	w.Resize(fyne.NewSize(960, 720))

	// a download still running is cancelled and the browser closed on quit,
	// so no Chrome process outlives the app
	a.Lifecycle().SetOnStopped(ctrl.Shutdown)

	w.ShowAndRun()
}

func aboutText() string {
	v := Version
	if Tag != "" {
		v = Tag
	}
	return "Version " + v + "\n\nDownloads manga chapters from supported sites into CBZ files.\n" +
		"Source and issues: https://github.com/elboletaire/manga-downloader"
}
