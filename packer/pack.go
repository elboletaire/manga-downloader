// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/fatih/color"
)

// Supported output formats (see grabber.Settings.Format)
const (
	FormatCBZ = "cbz"
	FormatRaw = "raw"
)

// DownloadedChapter represents a downloaded chapter (a Chapter + Files)
type DownloadedChapter struct {
	*grabber.Chapter
	Files []*downloader.File
}

// PackSingle packs a single downloaded chapter
func PackSingle(outputdir string, s grabber.Site, chapter *DownloadedChapter, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	return pack(outputdir, s.GetFormat(), s.GetFilenameTemplate(), title, NewChapterFileTemplateParts(title, chapter.Chapter), namePages(chapter.Files, s.GetConvertImages()), progress)
}

// PackBundle packs a bundle of downloaded chapters, grouping each chapter's
// pages into its own folder inside the archive (Chapter 0001/000.jpg, ...)
// so chapter boundaries survive bundling instead of a single flat renumbering.
func PackBundle(outputdir string, s grabber.Site, chapters []*DownloadedChapter, rng string, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	files := []File{}
	for _, chapter := range chapters {
		folder := SanitizeFilename(fmt.Sprintf("Chapter %s", paddedChapterNumber(chapter.GetNumber())))
		for _, page := range namePages(chapter.Files, s.GetConvertImages()) {
			files = append(files, File{
				Name: fmt.Sprintf("%s/%s", folder, page.Name),
				Data: page.Data,
			})
		}
	}

	return pack(outputdir, s.GetFormat(), s.GetFilenameTemplate(), title, FilenameTemplateParts{
		Series: title,
		Number: rng,
		Title:  "bundle",
	}, files, progress)
}

// namePages names a chapter's pages sequentially (000.jpg, 001.png, ...),
// restarting at 000 for each call, with extensions detected from the image
// bytes. Pages whose format is listed in formats are transcoded to JPEG first,
// so e-readers that can't render AVIF (or WebP) still get a readable archive.
func namePages(pages []*downloader.File, formats grabber.ConvertFormats) []File {
	named := make([]File, len(pages))
	for i, page := range pages {
		data, ext := page.Data, extFromContent(page.Data)

		if formats.Has(ext) {
			converted, err := convertToJPEG(data)
			if err != nil {
				// keep both the original bytes and the original extension:
				// aborting would lose the whole chapter (the whole bundle, when
				// bundling) over a single bad page, and naming undecodable
				// bytes .jpg would only make the failure more confusing
				color.Yellow("- warning: page %03d: converting %s to jpeg: %s (keeping the original)", i, ext, err)
			} else {
				data, ext = converted, "jpg"
			}
		}

		named[i] = File{
			Name: fmt.Sprintf("%03d.%s", i, ext),
			Data: data,
		}
	}
	return named
}

func pack(outputdir, format, template, title string, parts FilenameTemplateParts, files []File, progress func(page, progress int)) (string, error) {
	parts.Version = 1

	for {
		filename, err := NewFilenameFromTemplate(template, parts)
		if err != nil {
			return "", fmt.Errorf("- error creating filename for chapter %s: %s", title, err.Error())
		}

		var name string
		if format == FormatRaw {
			name = filename
			err = SaveRaw(filepath.Join(outputdir, name), files, progress)
		} else {
			name = filename + ".cbz"
			err = ArchiveCBZ(filepath.Join(outputdir, name), files, progress)
		}

		if os.IsExist(err) {
			parts.Version++
			continue
		}
		if err != nil {
			return "", fmt.Errorf("- error saving file %s: %s", name, err.Error())
		}

		return name, nil
	}
}
