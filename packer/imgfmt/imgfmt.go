// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package imgfmt identifies an image's format from its leading bytes,
// returning the canonical short name shared by packer's page naming
// (packer.extFromContent) and the verify-cbz archive checker. Keeping the
// sniff in one place guarantees the two agree on what a page's bytes mean,
// instead of two copies that can drift apart unnoticed (a drift here is
// exactly what would let a half-broken conversion slip past verify-cbz).
//
// Recognised formats and their names: avif, gif, jpg, png, webp. Anything
// else returns "", so each caller applies its own policy for the unknown
// case (packer falls back to "jpg"; verify-cbz treats it as "not an image").
package imgfmt

import "bytes"

// Sniff reports the image format of data from its leading bytes, or "" when
// the bytes aren't a recognised image.
//
// net/http.DetectContentType doesn't recognise AVIF (as of Go 1.19), so the
// ISOBMFF "ftyp" box is matched explicitly; the remaining formats are read
// straight off their magic bytes (which DetectContentType would also catch,
// but spelling them out keeps the whole sniff dependency-free and lets
// verify-cbz work from just a 12-byte head).
func Sniff(data []byte) string {
	switch {
	case isAvif(data):
		return "avif"
	case bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}):
		return "jpg"
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return "png"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "gif"
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "webp"
	}
	return ""
}

// isAvif reports whether data looks like an ISOBMFF file with an "avif" or
// "avis" (image sequence) brand: bytes 4-7 are "ftyp" and bytes 8-11 are the
// brand.
func isAvif(data []byte) bool {
	if len(data) < 12 || !bytes.Equal(data[4:8], []byte("ftyp")) {
		return false
	}
	brand := data[8:12]
	return bytes.Equal(brand, []byte("avif")) || bytes.Equal(brand, []byte("avis"))
}
