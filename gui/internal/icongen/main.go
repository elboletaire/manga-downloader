// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Command icongen draws gui/Icon.png, the app icon: a rounded blue tile with
// a white page and a download arrow. It's a one-off generator run by hand
// (go run ./internal/icongen from gui/), not part of the shipped binary.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

const size = 512

var (
	tile   = color.RGBA{R: 0x1f, G: 0x4e, B: 0x8c, A: 0xff}
	paper  = color.RGBA{R: 0xfa, G: 0xfa, B: 0xf7, A: 0xff}
	line   = color.RGBA{R: 0xb9, G: 0xc6, B: 0xd8, A: 0xff}
	accent = color.RGBA{R: 0xf2, G: 0x9e, B: 0x2e, A: 0xff}
)

// inRoundedRect reports whether (x, y) is inside the rectangle [x0,x1)x[y0,y1)
// with corners of radius r
func inRoundedRect(x, y, x0, y0, x1, y1, r int) bool {
	if x < x0 || x >= x1 || y < y0 || y >= y1 {
		return false
	}
	cx, cy := -1, -1
	switch {
	case x < x0+r && y < y0+r:
		cx, cy = x0+r, y0+r
	case x >= x1-r && y < y0+r:
		cx, cy = x1-r, y0+r
	case x < x0+r && y >= y1-r:
		cx, cy = x0+r, y1-r
	case x >= x1-r && y >= y1-r:
		cx, cy = x1-r, y1-r
	}
	if cx < 0 {
		return true
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= r*r
}

func draw() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !inRoundedRect(x, y, 0, 0, size, size, 110) {
				continue
			}
			img.Set(x, y, tile)
			// the page: a sheet with a folded corner in the upper right
			if inRoundedRect(x, y, 150, 96, 362, 400, 18) && !(x > 300 && y < 158 && x-300 > 158-y) {
				img.Set(x, y, paper)
			}
			// text lines on the page
			if x >= 190 && x < 322 && (y >= 190 && y < 204 || y >= 236 && y < 250 || y >= 282 && y < 296) {
				img.Set(x, y, line)
			}
			// the arrow: a shaft under the page, then a head tapering to a point
			if x >= 236 && x < 276 && y >= 330 && y < 430 {
				img.Set(x, y, accent)
			}
			if y >= 380 && y < 440 && abs(x-256) <= (440-y)*90/60 {
				img.Set(x, y, accent)
			}
		}
	}
	return img
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func main() {
	out := "Icon.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, draw()); err != nil {
		log.Fatal(err)
	}
}
