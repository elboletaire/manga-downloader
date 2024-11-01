package packer

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"

	"github.com/voxelost/manga-downloader/downloader"
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

	for i, file := range files {
		f, err := w.Create(fmt.Sprintf("%03d.jpg", i))
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
