package packer

import (
	"archive/zip"
	"errors"
	"os"

	"github.com/elboletaire/manga-downloader/downloader"
)

// ArchiveCBZ archives the given files into a CBZ file
func ArchiveCBZ(filename string, files []*downloader.File) error {
	if len(files) == 0 {
		return errors.New("no files to pack")
	}
	buff, err := os.Create(filename)
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
	}

	err = w.Close()
	return err
}
