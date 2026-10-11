// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Command manga-downloader-gui is the desktop front end of the CLI. It lives
// in its own module so the webview's cgo and its platform libraries never
// reach `go install` of the CLI; see AGENTS.md.
package main

import (
	"context"
	"embed"
	"log"

	"github.com/elboletaire/manga-downloader/notice"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// Version and Tag are set at link time by the release workflow (-X), the same
// way the CLI gets its version: Version is the short commit sha, Tag the
// release tag. Both stay at their defaults in a plain build.
var (
	Version = "dev"
	Tag     = ""
)

// assets is the built frontend. It's produced by `pnpm build` in frontend/,
// which make build/gui and the release workflow run before compiling.
//
//go:embed all:frontend/dist
var assets embed.FS

// appIcon is the window icon on Linux. It's the free-shape mark rather than
// the macOS tile, which would look like a box on a Linux taskbar. Windows and
// macOS take theirs from build/ at packaging time.
//
//go:embed build/linux/icon.png
var appIcon []byte

func main() {
	rt := &wailsRuntime{}
	var app *App
	// notify is called from the download goroutines too, so it reads app
	// through the pointer set before the window opens. Warnings raised while
	// the options load come before that, hence the nil check.
	ctrl, prefsPath := newSession(func() {
		if app != nil {
			app.notify()
		}
	})
	app = newApp(ctrl, rt, prefsPath, Tag, versionString())

	err := wails.Run(&options.App{
		Title:            "Manga Downloader",
		Width:            1100,
		Height:           760,
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: startupBackground(),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx, rt)
		},
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
		Linux:      &linux.Options{Icon: appIcon},
	})
	if err != nil {
		log.Fatal(err)
	}
}

// newSession returns the controller with the saved options applied, and the
// path they're saved at (empty when there is none). The controller is created
// first, so the warnings loading raises reach its log instead of stdout.
func newSession(notify func()) (*Controller, string) {
	settings := defaultSettings()
	ctrl := NewController(settings, notify)

	prefsPath, err := guiConfigPath()
	if err != nil {
		// the window still works, the options just won't outlive this run
		notice.Warn("%v", err)
		return ctrl, ""
	}
	if err := loadOptions(prefsPath, settings); err != nil {
		notice.Warn("ignoring the saved options: %v", err)
	}

	return ctrl, prefsPath
}

// versionString is what the window's About text shows: the tag when there is
// one, the build's version otherwise
func versionString() string {
	if Tag != "" {
		return Tag
	}

	return Version
}
