// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
)

// SaveRaw saves the given named files as plain image files inside dirname,
// one file per page, honoring directory prefixes in entry names (bundles).
// dirname is created fresh with os.Mkdir (not MkdirAll), so an
// already-existing directory returns an os.IsExist-compatible error,
// matching ArchiveCBZ's behaviour and letting the same version-bump loop in
// pack() handle the dedup.
func SaveRaw(dirname string, files []File, progress func(page, progress int)) error {
	if len(files) == 0 {
		return errors.New("no files to pack")
	}

	if err := os.Mkdir(dirname, 0755); err != nil {
		return err
	}

	for _, file := range files {
		path := filepath.Join(dirname, filepath.FromSlash(file.Name))
		if dir := filepath.Dir(path); dir != dirname {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
		}
		if err := os.WriteFile(path, file.Data, 0644); err != nil {
			return err
		}
		progress(1, 0) // Report progress by single page increments
	}

	return nil
}

// extFromContent detects the file extension to use from the image bytes,
// so page files can be browsed directly with real extensions. Falls back
// to "jpg" for anything not recognised.
func extFromContent(data []byte) string {
	if isAvif(data) {
		// net/http.DetectContentType doesn't recognise AVIF (as of Go
		// 1.19), so it's sniffed manually from the ISOBMFF "ftyp" box
		// (atsu.moe serves its page images as AVIF)
		return "avif"
	}

	switch http.DetectContentType(data) {
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	default:
		return "jpg"
	}
}

// isAvif reports whether data looks like an ISOBMFF file with an "avif" or
// "avis" (image sequence) brand, i.e. bytes 4-7 are "ftyp" and bytes 8-11
// are the brand.
func isAvif(data []byte) bool {
	if len(data) < 12 || string(data[4:8]) != "ftyp" {
		return false
	}
	brand := string(data[8:12])
	return brand == "avif" || brand == "avis"
}
