// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import "github.com/wailsapp/wails/v2/pkg/options"

// The window's background shows until the page paints its own, so a dark
// system would flash the light colour first. These are the page's --bg in
// styles.css for each scheme; theme_test.go keeps them in step with that file.
var (
	lightBackground = options.RGBA{R: 0xf4, G: 0xf3, B: 0xf0, A: 0xff}
	darkBackground  = options.RGBA{R: 0x12, G: 0x12, B: 0x14, A: 0xff}
)

// startupBackground is the window's background for the system's theme as it
// is when the app starts. Wails sets the background once, so a theme changed
// while the app runs isn't followed by it (the page follows it).
func startupBackground() *options.RGBA {
	bg := lightBackground
	if systemPrefersDark() {
		bg = darkBackground
	}

	return &bg
}
