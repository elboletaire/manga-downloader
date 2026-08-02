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

// tinyWebP is a real WebP file inlined as bytes: x/image/webp is decode-only,
// so a fixture can't be generated at runtime like the avif one. Lifted from
// golang.org/x/image@v0.44.0/webp's own testdata (gopher-doc.1bpp.lossless.webp,
// the smallest file there).
var tinyWebP = []byte{
	0x52, 0x49, 0x46, 0x46, 0xb2, 0x01, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x4c,
	0xa5, 0x01, 0x00, 0x00, 0x2f, 0x4a, 0xc0, 0x18, 0x00, 0x0f, 0x30, 0xff, 0xf3, 0x3f, 0xff, 0xf3,
	0x1f, 0x78, 0x90, 0x24, 0x6d, 0x7b, 0xda, 0x48, 0x6e, 0xe6, 0xf1, 0x0d, 0xc6, 0x7d, 0x84, 0x81,
	0x25, 0xe9, 0x30, 0x43, 0x3b, 0x66, 0xfc, 0x87, 0x19, 0x96, 0x0c, 0x27, 0x99, 0x62, 0x26, 0x9f,
	0x60, 0x4a, 0xed, 0xa1, 0x66, 0x06, 0xd9, 0xd5, 0x8a, 0xbe, 0xaa, 0xff, 0xff, 0x15, 0x3a, 0x41,
	0x44, 0xff, 0x19, 0xb8, 0x6d, 0xa4, 0xc8, 0xbb, 0xc7, 0x38, 0xf0, 0x0a, 0xc4, 0xa3, 0xaf, 0x81,
	0xdf, 0x31, 0x4a, 0x62, 0x59, 0xf7, 0xa6, 0xa0, 0xa5, 0x48, 0x22, 0x97, 0xd1, 0xb7, 0xa0, 0x15,
	0x30, 0x17, 0x14, 0xe2, 0xd7, 0x1d, 0x2c, 0x85, 0xf1, 0xc0, 0x8d, 0x71, 0x91, 0x06, 0xe0, 0xec,
	0xb0, 0xb8, 0x0e, 0x0a, 0x55, 0x57, 0xc9, 0x0a, 0x20, 0x2b, 0x53, 0xb1, 0x80, 0x80, 0x92, 0x3c,
	0xfa, 0x52, 0x4f, 0xfc, 0xe2, 0x8c, 0x4f, 0xf7, 0xc1, 0x02, 0x37, 0xaf, 0x83, 0x57, 0x18, 0x07,
	0xb6, 0x15, 0x90, 0x5b, 0x96, 0x81, 0xad, 0xa5, 0xc8, 0xf8, 0xb9, 0x23, 0x41, 0xc5, 0xcb, 0x96,
	0x13, 0xa5, 0x62, 0x07, 0x83, 0x44, 0x59, 0xa6, 0x49, 0xe2, 0x45, 0x55, 0xbd, 0xa1, 0xd1, 0xc0,
	0x28, 0xec, 0x28, 0xb1, 0x6b, 0x8e, 0x19, 0xdc, 0x48, 0xca, 0x7d, 0x8e, 0xbd, 0xa0, 0x83, 0xbe,
	0x18, 0x3f, 0xc1, 0xee, 0x93, 0xc1, 0xa7, 0x4f, 0x04, 0xf6, 0xea, 0x05, 0x5e, 0x7c, 0x32, 0xc2,
	0xe6, 0x30, 0x9f, 0x32, 0x66, 0x73, 0x96, 0x93, 0xc4, 0x91, 0xcf, 0x83, 0x7e, 0x42, 0x8c, 0x8f,
	0x2f, 0xe3, 0x27, 0x6a, 0x6c, 0xcc, 0xbd, 0xc1, 0x35, 0xac, 0x73, 0x44, 0xaf, 0xdd, 0x45, 0xf4,
	0x62, 0x99, 0x3d, 0x55, 0x1c, 0x4b, 0xdc, 0x3b, 0x3e, 0x18, 0x47, 0xdf, 0xab, 0x2e, 0x07, 0xda,
	0x8f, 0x79, 0x86, 0xff, 0xa0, 0xb9, 0x3a, 0x72, 0xe4, 0xe2, 0x27, 0x4c, 0x0e, 0x2b, 0x79, 0xb9,
	0x87, 0x57, 0x0a, 0x8d, 0x6e, 0x84, 0x55, 0x90, 0x98, 0x30, 0xae, 0xdd, 0xc5, 0xc2, 0x82, 0x05,
	0xd8, 0x0f, 0xf4, 0x79, 0x0a, 0xaf, 0xd8, 0x24, 0x00, 0xed, 0x8f, 0xf0, 0x62, 0x99, 0x19, 0x65,
	0x5d, 0x20, 0x06, 0xad, 0x41, 0xaf, 0xb5, 0x20, 0x3a, 0x6d, 0xea, 0xac, 0xa8, 0xad, 0x5c, 0x1d,
	0xcb, 0x4d, 0x71, 0x75, 0x6f, 0x09, 0x91, 0xf9, 0x3a, 0xc6, 0x31, 0x17, 0x99, 0x54, 0x10, 0xf8,
	0x74, 0x1d, 0x16, 0xbe, 0x8e, 0x2a, 0x12, 0x0d, 0xdf, 0x87, 0x57, 0x5a, 0xad, 0x3e, 0xd2, 0xaa,
	0xfa, 0x10, 0x94, 0x82, 0x79, 0xe5, 0x4b, 0x1f, 0xdf, 0xa0, 0xbc, 0x64, 0xcb, 0xca, 0xa3, 0x3a,
	0xe4, 0xf4, 0x38, 0xe2, 0x28, 0x73, 0x95, 0x35, 0xf1, 0x40, 0xa8, 0xca, 0x6c, 0x0b, 0xec, 0x85,
	0x78, 0x22, 0xaf, 0xb2, 0xe2, 0x97, 0xdc, 0x38, 0x2f, 0x66, 0xef, 0x33, 0x27, 0x26, 0x8d, 0x07,
	0x2a, 0x5d, 0xa3, 0x02, 0x3b, 0xa0, 0x65, 0x63, 0x6f, 0x22, 0xf8, 0x53, 0x8b, 0xcd, 0xb7, 0xc8,
	0xd6, 0xf1, 0x2a, 0xc4, 0x08, 0x68, 0xb6, 0x87, 0x00, 0x00,
}

// TestNamePagesConvertsWebP runs a real WebP page through the unstubbed
// pipeline. Besides the routing, this is what pins the load-bearing blank
// import of x/image/webp in convert.go: with the decoder stubbed, removing
// that import would fail no test while silently downgrading webp conversion
// to "kept original + warning" at runtime.
func TestNamePagesConvertsWebP(t *testing.T) {
	if got := extFromContent(tinyWebP); got != "webp" {
		t.Fatalf("fixture sniffs as %q, want %q", got, "webp")
	}

	named := namePages([]*downloader.File{{Data: tinyWebP}}, grabber.ConvertFormats{grabber.ConvertWebP: true})

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
