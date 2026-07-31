// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"

	// registers the AVIF decoder with image.Decode (both the "avif" still and
	// "avis" image sequence brands). Its registered decoder returns the first
	// frame only, which is what we want: no e-reader renders animated AVIF at
	// all, so a single frame beats an unreadable page.
	_ "github.com/gen2brain/avif"
	// registers the WebP decoder with image.Decode
	_ "golang.org/x/image/webp"
)

// jpegQuality is the quality pages are re-encoded at. The stdlib default (75)
// visibly muddies the screentones and small lettering that make up most of a
// manga page.
const jpegQuality = 90

// decodeImage decodes an encoded image, picking the decoder from its magic
// bytes. It's the single seam every decoder is reached through, so the AVIF
// backend can be swapped in one place (and stubbed out in tests), same as
// downloader's retryDelay.
var decodeImage = func(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// convertToJPEG decodes an image and re-encodes it as JPEG, so pages served in
// formats dedicated e-readers can't display (AVIF, and optionally WebP) end up
// in an archive they can actually open.
func convertToJPEG(data []byte) ([]byte, error) {
	img, err := decodeImage(data)
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, flattenAlpha(img), &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("encoding jpeg: %w", err)
	}

	return buf.Bytes(), nil
}

// flattenAlpha composites img onto an opaque white background, since JPEG has
// no alpha channel and jpeg.Encode drops it rather than compositing: without
// this, the {0,0,0,0} transparent pixels AVIF/WebP pages use would be encoded
// as solid black. Images that are already opaque are returned untouched, so a
// lossy webp's *image.YCbCr keeps jpeg.Encode's no-conversion fast path
// instead of being round-tripped through RGBA.
func flattenAlpha(img image.Image) image.Image {
	if opaque, ok := img.(interface{ Opaque() bool }); ok && opaque.Opaque() {
		return img
	}

	// the source bounds don't necessarily start at (0,0), so the destination is
	// allocated from its size and drawn from its Min corner
	bounds := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Over)

	return dst
}
