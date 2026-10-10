// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Bar thicknesses: a progress row's bar, and the run's overall one
const (
	barHeight     = 6
	overallHeight = 10
)

// thinBar is a slim, rounded progress bar: widget.ProgressBar is as tall as a
// button and prints a percentage, which made every progress row a block. Its
// colour carries the row's state, so done and failed differ at a glance.
//
// It can also stack several coloured segments, which is how the overall bar
// shows saved, skipped, failed and in-flight chapters side by side: one
// colour for the whole run would make a single failure look like all of them.
type thinBar struct {
	widget.BaseWidget
	segments []barSegment
	height   float32
}

// barSegment is a stretch of the bar, as a fraction of its width
type barSegment struct {
	value  float64
	colour fyne.ThemeColorName
}

func newThinBar(height float32) *thinBar {
	b := &thinBar{height: height}
	b.ExtendBaseWidget(b)
	return b
}

// set fills the bar to value, as a fraction, in one colour
func (b *thinBar) set(value float64, colour fyne.ThemeColorName) {
	b.setSegments(barSegment{value, colour})
}

// setSegments fills the bar with segs, left to right. Fractions are clamped
// so the segments never run past the end.
func (b *thinBar) setSegments(segs ...barSegment) {
	left := 1.0
	clamped := make([]barSegment, 0, len(segs))
	for _, s := range segs {
		s.value = min(max(s.value, 0), left)
		left -= s.value
		clamped = append(clamped, s)
	}
	if slices.Equal(clamped, b.segments) {
		return
	}
	b.segments = clamped
	b.Refresh()
}

func (b *thinBar) CreateRenderer() fyne.WidgetRenderer {
	r := &thinBarRenderer{bar: b, track: canvas.NewRectangle(color.Transparent)}
	r.Refresh()
	return r
}

type thinBarRenderer struct {
	bar   *thinBar
	track *canvas.Rectangle
	fills []*canvas.Rectangle
}

func (r *thinBarRenderer) Layout(size fyne.Size) {
	// the bar keeps its own thickness, centred, whatever height the layout
	// hands it
	h := min(r.bar.height, size.Height)
	y := (size.Height - h) / 2
	r.track.Move(fyne.NewPos(0, y))
	r.track.Resize(fyne.NewSize(size.Width, h))
	r.track.CornerRadius = h / 2

	last := -1
	for i, seg := range r.bar.segments {
		if seg.value > 0 {
			last = i
		}
	}
	x := float32(0)
	for i, f := range r.fills {
		w := size.Width * float32(r.bar.segments[i].value)
		f.Move(fyne.NewPos(x, y))
		f.Resize(fyne.NewSize(w, h))
		f.Hidden = w <= 0
		// only the fill's two ends are round, so adjacent segments meet
		// flush. A radius over half the width is what makes the software
		// renderer draw negative rectangles, hence the cap.
		radius := min(h, w) / 2
		f.TopLeftCornerRadius, f.BottomLeftCornerRadius = 0, 0
		f.TopRightCornerRadius, f.BottomRightCornerRadius = 0, 0
		if x <= 0 {
			f.TopLeftCornerRadius, f.BottomLeftCornerRadius = radius, radius
		}
		if i == last {
			f.TopRightCornerRadius, f.BottomRightCornerRadius = radius, radius
		}
		x += w
	}
}

func (r *thinBarRenderer) MinSize() fyne.Size { return fyne.NewSize(0, r.bar.height) }

func (r *thinBarRenderer) Refresh() {
	segs := r.bar.segments
	for len(r.fills) < len(segs) {
		r.fills = append(r.fills, canvas.NewRectangle(color.Transparent))
	}
	r.fills = r.fills[:len(segs)]
	// a one-colour bar's track is that colour, faded, so even an empty bar
	// says which state it's in; a stacked bar's is neutral
	track := withAlpha(theme.Color(theme.ColorNameForeground), tintSoft)
	if len(segs) == 1 {
		track = withAlpha(theme.Color(segs[0].colour), tintStrong)
	}
	r.track.FillColor = track
	for i, f := range r.fills {
		f.FillColor = theme.Color(segs[i].colour)
	}
	r.Layout(r.bar.Size())
	r.track.Refresh()
	for _, f := range r.fills {
		f.Refresh()
	}
}

func (r *thinBarRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.track}
	for _, f := range r.fills {
		objs = append(objs, f)
	}
	return objs
}
func (r *thinBarRenderer) Destroy() {}
