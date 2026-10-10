// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Tint opacities. A tint is a theme colour laid over the background, so the
// same value reads in both variants without picking colours per theme.
const (
	tintSoft   = 0x1c // a status badge or banner
	tintStrong = 0x30 // a coloured badge that has to stand out
)

// surface paints a theme-coloured panel behind its content. The colour is
// looked up on every refresh, so it follows a light/dark switch; that's why
// it's a widget rather than a canvas.Rectangle built once.
//
// It stands in for widget.Card, which crashes the software renderer inside a
// List (negative rectangle sizes from its shadow).
type surface struct {
	widget.BaseWidget
	content fyne.CanvasObject
	// colour is the panel colour; alpha, when non-zero, turns it into a tint
	colour fyne.ThemeColorName
	alpha  uint8
	// radius rounds the corners; inset is the padding around the content, in
	// multiples of the theme padding
	radius float32
	inset  float32
}

func newSurface(content fyne.CanvasObject, colour fyne.ThemeColorName, alpha uint8) *surface {
	s := &surface{content: content, colour: colour, alpha: alpha, inset: 1}
	s.ExtendBaseWidget(s)
	return s
}

// setTint changes the panel colour, redrawing only when it differs
func (s *surface) setTint(colour fyne.ThemeColorName, alpha uint8) {
	if s.colour == colour && s.alpha == alpha {
		return
	}
	s.colour, s.alpha = colour, alpha
	s.Refresh()
}

func (s *surface) CreateRenderer() fyne.WidgetRenderer {
	r := &surfaceRenderer{s: s, bg: canvas.NewRectangle(color.Transparent)}
	r.Refresh()
	return r
}

type surfaceRenderer struct {
	s  *surface
	bg *canvas.Rectangle
}

func (r *surfaceRenderer) pad() float32 { return r.s.inset * theme.Padding() }

func (r *surfaceRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	p := r.pad()
	r.s.content.Move(fyne.NewPos(p, p))
	r.s.content.Resize(fyne.NewSize(max(size.Width-2*p, 0), max(size.Height-2*p, 0)))
}

func (r *surfaceRenderer) MinSize() fyne.Size {
	p := r.pad()
	return r.s.content.MinSize().Add(fyne.NewSize(2*p, 2*p))
}

func (r *surfaceRenderer) Refresh() {
	c := theme.Color(r.s.colour)
	if r.s.alpha != 0 {
		c = withAlpha(c, r.s.alpha)
	}
	r.bg.FillColor = c
	r.bg.CornerRadius = r.s.radius
	r.bg.Refresh()
	r.Layout(r.s.Size())
}

func (r *surfaceRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.s.content}
}
func (r *surfaceRenderer) Destroy() {}

// pill is a badge: a label on a rounded tint of its own status colour. It
// wraps a plain widget.Label, so callers keep setting Text and Importance on
// that label and then refresh the pill.
//
// The badge hugs its text, but a truncating label can still be squeezed: the
// pill is as wide as its text when there's room and ellipsises otherwise, so
// a long error never pushes the buttons beside it out of the window.
type pill struct {
	widget.BaseWidget
	label *widget.Label
	// measure is an untruncated twin of label, never shown: its MinSize is
	// the width the whole text needs, as the label itself would lay it out
	measure *widget.Label
}

func newPill(l *widget.Label) *pill {
	p := &pill{label: l, measure: widget.NewLabel("")}
	p.ExtendBaseWidget(p)
	return p
}

// pillColour is the tint behind a label of each importance: neutral for
// plain text, its own colour otherwise
func pillColour(imp widget.Importance) (fyne.ThemeColorName, uint8) {
	switch imp {
	case widget.HighImportance:
		return theme.ColorNamePrimary, tintStrong
	case widget.SuccessImportance:
		return theme.ColorNameSuccess, tintStrong
	case widget.WarningImportance:
		return theme.ColorNameWarning, tintStrong
	case widget.DangerImportance:
		return theme.ColorNameError, tintStrong
	}
	return theme.ColorNameForeground, tintSoft
}

func (p *pill) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = canvas.RadiusMaximum
	r := &pillRenderer{p: p, bg: bg}
	r.Refresh()
	return r
}

type pillRenderer struct {
	p  *pill
	bg *canvas.Rectangle
}

// natural is the width the label needs to show its whole text
func (r *pillRenderer) natural() float32 {
	l, m := r.p.label, r.p.measure
	if m.Text != l.Text || m.SizeName != l.SizeName || m.TextStyle != l.TextStyle {
		m.Text, m.SizeName, m.TextStyle = l.Text, l.SizeName, l.TextStyle
		m.Refresh()
	}
	return m.MinSize().Width
}

func (r *pillRenderer) Layout(size fyne.Size) {
	h := r.p.label.MinSize().Height
	// the label ellipsises at exactly its own min width (rounding in its
	// measuring), so it gets a hair more than the text needs
	// and never narrower than tall, so a two-letter badge stays round
	w := min(max(r.natural()+2, h), size.Width)
	pos := fyne.NewPos(0, (size.Height-h)/2)
	r.bg.Move(pos)
	r.bg.Resize(fyne.NewSize(w, h))
	r.p.label.Move(pos)
	r.p.label.Resize(fyne.NewSize(w, h))
}

func (r *pillRenderer) MinSize() fyne.Size {
	m := r.p.label.MinSize()
	return fyne.NewSize(max(m.Width, m.Height), m.Height)
}

func (r *pillRenderer) Refresh() {
	colour, alpha := pillColour(r.p.label.Importance)
	r.bg.FillColor = withAlpha(theme.Color(colour), alpha)
	// an empty pill is no pill at all, not a blank capsule
	r.bg.Hidden = r.p.label.Text == ""
	r.bg.Refresh()
	r.p.label.Refresh()
	r.Layout(r.p.Size())
}

func (r *pillRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.p.label} }
func (r *pillRenderer) Destroy()                     {}
