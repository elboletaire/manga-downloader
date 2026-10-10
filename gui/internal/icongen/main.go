// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Command icongen draws the app icon: a rounded blue tile with a white page and
// a download arrow. The design is laid out on a 512 grid and scaled to the
// requested size, so it can make build/appicon.png at 1024. It's a one-off
// generator run by hand, not part of the shipped binary:
//
//	go run ./internal/icongen build/appicon.png 1024   (from gui/)
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"strconv"
)

// base is the grid the design is drawn on; size is the output's width
const base = 512

var size = base

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
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			// x and y are on the base grid, so the design doesn't change with size
			x, y := px*base/size, py*base/size
			if !inRoundedRect(x, y, 0, 0, base, base, 110) {
				continue
			}
			img.Set(px, py, tile)
			// the page: a sheet with a folded corner in the upper right
			if inRoundedRect(x, y, 150, 96, 362, 400, 18) && !(x > 300 && y < 158 && x-300 > 158-y) {
				img.Set(px, py, paper)
			}
			// text lines on the page
			if x >= 190 && x < 322 && (y >= 190 && y < 204 || y >= 236 && y < 250 || y >= 282 && y < 296) {
				img.Set(px, py, line)
			}
			// the arrow: a shaft under the page, then a head tapering to a point
			if x >= 236 && x < 276 && y >= 330 && y < 430 {
				img.Set(px, py, accent)
			}
			if y >= 380 && y < 440 && abs(x-256) <= (440-y)*90/60 {
				img.Set(px, py, accent)
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
	out := "build/appicon.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if len(os.Args) > 2 {
		n, err := strconv.Atoi(os.Args[2])
		if err != nil || n < 16 {
			log.Fatal("size must be at least 16")
		}
		size = n
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
