// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// barHeight is the thickness of a progress row's bar
const barHeight = 4

// thinBar is a slim progress bar for the progress rows: widget.ProgressBar is
// as tall as a button and prints a percentage, which made every row a block.
// Its colour carries the row's state, so done and failed differ at a glance.
type thinBar struct {
	widget.BaseWidget
	value  float64
	colour fyne.ThemeColorName
}

func newThinBar() *thinBar {
	b := &thinBar{colour: theme.ColorNamePrimary}
	b.ExtendBaseWidget(b)
	return b
}

// set updates the fill, as a fraction, and its colour
func (b *thinBar) set(value float64, colour fyne.ThemeColorName) {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	if b.value == value && b.colour == colour {
		return
	}
	b.value, b.colour = value, colour
	b.Refresh()
}

func (b *thinBar) CreateRenderer() fyne.WidgetRenderer {
	r := &thinBarRenderer{bar: b, track: canvas.NewRectangle(color.Transparent), fill: canvas.NewRectangle(color.Transparent)}
	r.Refresh()
	return r
}

type thinBarRenderer struct {
	bar         *thinBar
	track, fill *canvas.Rectangle
}

func (r *thinBarRenderer) Layout(size fyne.Size) {
	r.track.Resize(size)
	r.fill.Resize(fyne.NewSize(size.Width*float32(r.bar.value), size.Height))
}

func (r *thinBarRenderer) MinSize() fyne.Size { return fyne.NewSize(0, barHeight) }

func (r *thinBarRenderer) Refresh() {
	r.track.FillColor = theme.Color(theme.ColorNameInputBackground)
	r.fill.FillColor = theme.Color(r.bar.colour)
	r.Layout(r.bar.Size())
	r.track.Refresh()
	r.fill.Refresh()
}

func (r *thinBarRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.track, r.fill} }
func (r *thinBarRenderer) Destroy()                     {}
