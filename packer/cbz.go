// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"archive/zip"
	"errors"
	"os"
)

// File represents a named entry to be written into a CBZ archive
type File struct {
	// Name is the zip entry name (may include a directory prefix, e.g. "Chapter 0001/000.jpg")
	Name string
	// Data is the raw file contents
	Data []byte
}

// ArchiveCBZ archives the given named files into a CBZ file
func ArchiveCBZ(filename string, files []File, progress func(page, progress int)) error {
	if len(files) == 0 {
		return errors.New("no files to pack")
	}
	buff, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer buff.Close()
	w := zip.NewWriter(buff)

	for _, file := range files {
		f, err := w.Create(file.Name)
		if err != nil {
			return err
		}
		if _, err = f.Write(file.Data); err != nil {
			return err
		}
		progress(1, 0) // Report progress by single page increments
	}

	err = w.Close()
	return err
}
