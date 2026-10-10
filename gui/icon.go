// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed Icon.png
var iconPNG []byte

// appIcon is the window and desktop icon
var appIcon = fyne.NewStaticResource("Icon.png", iconPNG)
