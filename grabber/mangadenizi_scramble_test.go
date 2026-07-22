// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// scrambleForTest builds a "tiled-v1" scrambled image from an original one,
// using the exact same primitives as tiledV1Descramble but with source/
// destination swapped: it's the forward operation the site performs to
// produce the images we have to undo. It exists purely to give the
// descrambling test a known-correct fixture without needing a live-fetched
// sample image.
func scrambleForTest(src image.Image, grid int, seed uint32) image.Image {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	f := grid
	if f > width {
		f = width
	}
	if f > height {
		f = height
	}

	colSegs := splitAxis(width, f)
	rowSegs := splitAxis(height, f)

	colPerm := fisherYates(f, deriveSeed(seed, scrambleColumnConst))
	rowPerm := fisherYates(f, deriveSeed(seed, scrambleRowConst))

	scrambledColSegs := permutedSegments(colSegs, colPerm)
	scrambledRowSegs := permutedSegments(rowSegs, rowPerm)

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for row := 0; row < f; row++ {
		destRow := rowPerm[row]
		origRowSeg := rowSegs[destRow]
		scrambledRowSeg := scrambledRowSegs[row]

		for col := 0; col < f; col++ {
			destCol := colPerm[col]
			origColSeg := colSegs[destCol]
			scrambledColSeg := scrambledColSegs[col]

			origRect := image.Rect(
				origColSeg.offset, origRowSeg.offset,
				origColSeg.offset+origColSeg.length, origRowSeg.offset+origRowSeg.length,
			)
			destPt := image.Pt(scrambledColSeg.offset, scrambledRowSeg.offset)

			draw.Draw(dst, image.Rectangle{Min: destPt, Max: destPt.Add(origRect.Size())}, src, origRect.Min, draw.Src)
		}
	}

	return dst
}

// checkerboardImage builds a deterministic, non-uniform test image so tile
// shuffling is actually detectable (each grid cell must differ from its
// neighbours, and pixels within a cell must differ from each other so a
// wrong offset within a correctly-placed tile also fails the comparison).
func checkerboardImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 7) % 256),
				G: uint8((y * 13) % 256),
				B: uint8((x + y) % 256),
				A: 255,
			})
		}
	}
	return img
}

func imagesEqual(a, b image.Image) bool {
	ba, bb := a.Bounds(), b.Bounds()
	if ba != bb {
		return false
	}
	for y := ba.Min.Y; y < ba.Max.Y; y++ {
		for x := ba.Min.X; x < ba.Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}

func TestTiledV1Descramble_RoundTrip(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
		grid          int
		seed          uint32
	}{
		{"square, evenly divisible", 100, 100, 10, 3618263072},
		{"tall page, uneven division", 97, 313, 7, 1464422084},
		{"grid larger than dimension", 5, 5, 10, 42},
		{"seed that derives to zero", 720, 480, 8, scrambleColumnConst}, // seed ^ scrambleColumnConst == 0
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := checkerboardImage(tc.width, tc.height)
			scrambled := scrambleForTest(original, tc.grid, tc.seed)

			// sanity check: scrambling must actually change the image (grid
			// > 1 and dimensions big enough), otherwise the test proves
			// nothing
			if tc.width >= 2 && tc.height >= 2 && tc.grid > 1 && imagesEqual(original, scrambled) {
				t.Fatalf("scrambleForTest did not change the image, fixture is not exercising anything")
			}

			descrambled := tiledV1Descramble(scrambled, tc.width, tc.height, tc.grid, tc.seed)

			if !imagesEqual(original, descrambled) {
				t.Fatalf("descrambled image does not match the original")
			}
		})
	}
}

func TestDescrambleMangadeniziImage_PassthroughForNone(t *testing.T) {
	data := []byte("not even a real image")

	for _, method := range []string{"", "none"} {
		out, err := descrambleMangadeniziImage(data, mangadeniziScramble{Method: method})
		if err != nil {
			t.Fatalf("method %q: unexpected error: %v", method, err)
		}
		if !bytes.Equal(out, data) {
			t.Fatalf("method %q: expected passthrough, got different bytes", method)
		}
	}
}

func TestDescrambleMangadeniziImage_UnsupportedMethod(t *testing.T) {
	_, err := descrambleMangadeniziImage([]byte("x"), mangadeniziScramble{Method: "xor"})
	if err == nil {
		t.Fatal("expected an error for an unsupported scramble method")
	}
}
