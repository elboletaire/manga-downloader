// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/elboletaire/manga-downloader/packer/imgfmt"
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

// extFromContent detects the file extension to use from the image bytes, so
// page files can be browsed directly with real extensions. It shares the
// format sniff with tools/verify-cbz via packer/imgfmt, and falls back to
// "jpg" for anything unrecognised so a page always gets a browsable extension.
func extFromContent(data []byte) string {
	if ext := imgfmt.Sniff(data); ext != "" {
		return ext
	}
	return "jpg"
}
