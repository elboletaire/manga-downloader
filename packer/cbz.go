package packer

import (
	"archive/zip"
	"os"

	"github.com/elboletaire/manga-downloader/downloader"
)

func ArchiveCBZ(filename string, files downloader.Files) error {
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
		_, err = f.Write(file.Data)
		if err != nil {
			return err
		}
	}

	err = w.Close()
	return err
}
