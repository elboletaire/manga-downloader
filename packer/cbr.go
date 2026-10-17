// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"errors"
	"os"

	rar "github.com/elboletaire/go-rar"
)

// ArchiveCBR archives the given named files into a CBR file. The pages are
// stored, not compressed: they're already images.
func ArchiveCBR(filename string, files []File, progress func(page, progress int)) error {
	if len(files) == 0 {
		return errors.New("no files to pack")
	}
	buff, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer buff.Close()
	w := rar.NewWriter(buff)

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

	return w.Close()
}
