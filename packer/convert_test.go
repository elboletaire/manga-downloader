// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/gen2brain/avif"
)

// gradientImage builds an opaque test image, synthesized rather than loaded
// from a fixture file (the repo has no testdata convention)
func gradientImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	return img
}

// avifBytes encodes img as AVIF, so tests get a real AVIF fixture without a
// testdata file. Keep the images tiny: the encoder runs under wazero, so a
// large one makes the test suite crawl.
func avifBytes(t *testing.T, img image.Image) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	if err := avif.Encode(buf, img, avif.Options{Quality: 60, Speed: 10}); err != nil {
		t.Fatalf("encoding the avif fixture: %s", err)
	}

	return buf.Bytes()
}

// pngBytes encodes img as PNG, for the cases that need a losslessly encoded
// fixture (transparency assertions can't survive a lossy encoder)
func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		t.Fatalf("encoding the png fixture: %s", err)
	}

	return buf.Bytes()
}

func jpegBytes(t *testing.T, img image.Image) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, img, nil); err != nil {
		t.Fatalf("encoding the jpeg fixture: %s", err)
	}

	return buf.Bytes()
}

func TestConvertToJPEG(t *testing.T) {
	data, err := convertToJPEG(avifBytes(t, gradientImage(16, 16)))
	if err != nil {
		t.Fatalf("convertToJPEG returned an unexpected error: %s", err)
	}

	if got := extFromContent(data); got != "jpg" {
		t.Errorf("converted image is %q, want %q", got, "jpg")
	}

	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the converted image doesn't decode as jpeg: %s", err)
	}
	if got := img.Bounds().Size(); got.X != 16 || got.Y != 16 {
		t.Errorf("converted image is %v, want 16x16", got)
	}
}

// TestConvertToJPEGFlattensTransparencyOntoWhite covers the trap that makes
// this conversion look fine in an extension check and awful on screen: JPEG
// has no alpha channel and jpeg.Encode doesn't composite, it just drops it.
// Transparent pixels are almost always stored as {0,0,0,0}, so without
// flattening they'd come out solid black - a page with transparent margins
// would end up with black bars.
func TestConvertToJPEGFlattensTransparencyOntoWhite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if x < 2 {
				img.SetNRGBA(x, y, color.NRGBA{}) // fully transparent
			} else {
				img.SetNRGBA(x, y, color.NRGBA{A: 255}) // opaque black
			}
		}
	}

	// encoded as png rather than avif: the lossy avif encoder doesn't preserve
	// the sharp transparent/opaque split faithfully enough to assert on
	data, err := convertToJPEG(pngBytes(t, img))
	if err != nil {
		t.Fatalf("convertToJPEG returned an unexpected error: %s", err)
	}

	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the converted image doesn't decode as jpeg: %s", err)
	}

	transparent, _, _, _ := decoded.At(0, 0).RGBA()
	if transparent>>8 < 200 {
		t.Errorf("transparent pixel converted to %d, want it flattened onto white (>200)", transparent>>8)
	}

	opaque, _, _, _ := decoded.At(3, 0).RGBA()
	if opaque>>8 > 60 {
		t.Errorf("opaque black pixel converted to %d, want it left black (<60)", opaque>>8)
	}
}

// TestFlattenAlphaPreservesOpaqueImage pins that an already-opaque image is
// returned untouched, so a lossy webp's *image.YCbCr keeps jpeg.Encode's
// no-conversion fast path instead of being round-tripped through RGBA
func TestFlattenAlphaPreservesOpaqueImage(t *testing.T) {
	img := gradientImage(4, 4)
	if got := flattenAlpha(img); got != image.Image(img) {
		t.Error("flattenAlpha copied an already-opaque image, want it returned as-is")
	}
}

func TestNamePagesConvertsListedFormats(t *testing.T) {
	realAvif := avifBytes(t, gradientImage(16, 16))
	realPng := pngBytes(t, gradientImage(4, 4))
	realJpeg := jpegBytes(t, gradientImage(4, 4))
	// enough of a header for the sniffing to report "webp"; it's never decoded
	// in the cases below, which is exactly what's being asserted
	webpHeader := []byte("RIFF\x00\x00\x00\x00WEBPVP8 stub")

	cases := []struct {
		name     string
		page     []byte
		formats  grabber.ConvertFormats
		wantName string
		// wantUntouched asserts the page bytes went in and came out identical,
		// which an extension-only assertion can't catch
		wantUntouched bool
	}{
		{"avif is converted when listed", realAvif, grabber.ConvertFormats{grabber.ConvertAVIF: true}, "000.jpg", false},
		{"avif is kept when not listed", realAvif, grabber.ConvertFormats{}, "000.avif", true},
		{"webp is kept when not listed", webpHeader, grabber.ConvertFormats{grabber.ConvertAVIF: true}, "000.webp", true},
		// a jpeg page must never be re-encoded, that's generation loss for nothing
		{"jpeg is never re-encoded", realJpeg, grabber.ConvertFormats{grabber.ConvertAVIF: true, grabber.ConvertWebP: true}, "000.jpg", true},
		{"png is never converted", realPng, grabber.ConvertFormats{grabber.ConvertAVIF: true, grabber.ConvertWebP: true}, "000.png", true},
	}

	for _, c := range cases {
		named := namePages([]*downloader.File{{Data: c.page}}, c.formats)

		if len(named) != 1 {
			t.Fatalf("%s: namePages returned %d files, want 1", c.name, len(named))
		}
		if named[0].Name != c.wantName {
			t.Errorf("%s: page named %q, want %q", c.name, named[0].Name, c.wantName)
		}
		if untouched := bytes.Equal(named[0].Data, c.page); untouched != c.wantUntouched {
			t.Errorf("%s: page bytes untouched = %v, want %v", c.name, untouched, c.wantUntouched)
		}
	}
}

// TestNamePagesConvertsWebP covers the webp routing with the decoder stubbed
// out: x/image/webp is decode-only so a real fixture can't be generated at
// runtime, and the decoding itself is the dependency's business anyway - what
// this needs to pin is that a webp page reaches the converter when asked for.
func TestNamePagesConvertsWebP(t *testing.T) {
	original := decodeImage
	defer func() { decodeImage = original }()
	decodeImage = func([]byte) (image.Image, error) {
		return gradientImage(4, 4), nil
	}

	page := []byte("RIFF\x00\x00\x00\x00WEBPVP8 stub")
	named := namePages([]*downloader.File{{Data: page}}, grabber.ConvertFormats{grabber.ConvertWebP: true})

	if named[0].Name != "000.jpg" {
		t.Errorf("page named %q, want %q", named[0].Name, "000.jpg")
	}
	if got := extFromContent(named[0].Data); got != "jpg" {
		t.Errorf("converted page is %q, want %q", got, "jpg")
	}
}

// TestNamePagesKeepsOriginalOnConversionFailure pins the failure contract: a
// page that can't be converted keeps its original bytes *and* its original
// extension. Aborting instead would lose the whole chapter (the whole bundle,
// in bundle mode) over one bad page, and naming undecodable bytes .jpg would
// just move the failure somewhere more confusing.
func TestNamePagesKeepsOriginalOnConversionFailure(t *testing.T) {
	// sniffs as avif, but there's nothing decodable behind the header
	page := append([]byte{0x00, 0x00, 0x00, 0x1c, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, []byte("truncated")...)

	named := namePages([]*downloader.File{{Data: page}}, grabber.ConvertFormats{grabber.ConvertAVIF: true})

	if named[0].Name != "000.avif" {
		t.Errorf("page named %q, want %q", named[0].Name, "000.avif")
	}
	if !bytes.Equal(named[0].Data, page) {
		t.Error("page bytes were modified, want the original kept as-is")
	}
}

func TestConvertToJPEGDecodeError(t *testing.T) {
	sentinel := errors.New("boom")

	original := decodeImage
	defer func() { decodeImage = original }()
	decodeImage = func([]byte) (image.Image, error) {
		return nil, sentinel
	}

	if _, err := convertToJPEG([]byte("whatever")); !errors.Is(err, sentinel) {
		t.Errorf("convertToJPEG error = %v, want it to wrap %v", err, sentinel)
	}
}
