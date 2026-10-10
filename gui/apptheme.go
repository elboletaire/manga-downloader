// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// appTheme is Fyne's default theme with the icon's blue as the primary colour
// and status colours that stay readable in both variants: the stock success
// green (#43f436) is close to illegible as text on a white background.
// Everything else (fonts, icons, sizes) is the default theme's.
type appTheme struct {
	// variant, when set, pins the theme to light or dark regardless of the
	// system preference; the screenshot tests use it to capture both
	variant *fyne.ThemeVariant
}

func newAppTheme() fyne.Theme { return &appTheme{} }

// rgb is an opaque colour from its hex components
func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 0xff}
}

// withAlpha is c at the given opacity, for tints over the background
func withAlpha(c color.Color, a uint8) color.NRGBA {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = a
	return n
}

// palette holds the colours appTheme changes, one set per variant
type palette struct {
	primary, onPrimary, success, warning, err, onStatus, header color.NRGBA
}

var (
	lightPalette = palette{
		primary:   rgb(0x2563b0), // the icon's blue, lifted a little for buttons
		onPrimary: rgb(0xffffff),
		success:   rgb(0x1e8e3e),
		warning:   rgb(0xb76e00),
		err:       rgb(0xd32f2f),
		onStatus:  rgb(0xffffff),
		header:    rgb(0xeef3f9),
	}
	darkPalette = palette{
		primary:   rgb(0x5e9cf0),
		onPrimary: rgb(0x0e1a2b),
		success:   rgb(0x4cc76a),
		warning:   rgb(0xf5a623), // the icon's orange
		err:       rgb(0xff6b6b),
		onStatus:  rgb(0x171718),
		header:    rgb(0x1f232b),
	}
)

func (t *appTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if t.variant != nil {
		v = *t.variant
	}
	p := darkPalette
	if v == theme.VariantLight {
		p = lightPalette
	}
	switch n {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return p.primary
	case theme.ColorNameForegroundOnPrimary:
		return p.onPrimary
	case theme.ColorNameFocus:
		return withAlpha(p.primary, 0x66)
	case theme.ColorNameSelection:
		return withAlpha(p.primary, 0x40)
	case theme.ColorNameSuccess:
		return p.success
	case theme.ColorNameWarning:
		return p.warning
	case theme.ColorNameError:
		return p.err
	case theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning, theme.ColorNameForegroundOnError:
		return p.onStatus
	case theme.ColorNameHeaderBackground:
		return p.header
	}
	return theme.DefaultTheme().Color(n, v)
}

func (t *appTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (t *appTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (t *appTheme) Size(n fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(n) }
