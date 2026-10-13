// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
)

// Supported output formats (see grabber.Settings.Format)
const (
	FormatCBZ  = "cbz"
	FormatRaw  = "raw"
	FormatEPUB = "epub"
)

// DownloadedChapter represents a downloaded chapter (a Chapter + Files)
type DownloadedChapter struct {
	*grabber.Chapter
	Files []*downloader.File
}

// PackSingle packs a single downloaded chapter
func PackSingle(outputdir string, s grabber.Site, chapter *DownloadedChapter, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	format := s.GetFormat()
	info := EPUBInfo{
		Series:           title,
		Position:         formatChapterNumber(chapter.GetNumber()),
		ReadingDirection: s.GetReadingDirection(),
		Labels:           map[string]string{epubSingleFolder: chapterLabel(chapter.Chapter)},
	}
	return pack(outputdir, format, s.GetFilenameTemplate(), title, NewChapterFileTemplateParts(title, chapter.Chapter), namePages(chapter.Files, pageFormats(format, s.GetConvertImages())), info, progress)
}

// PackBundle packs a bundle of downloaded chapters, grouping each chapter's
// pages into its own folder inside the archive (Chapter 0001/000.jpg, ...)
// so chapter boundaries survive bundling instead of a single flat renumbering.
func PackBundle(outputdir string, s grabber.Site, chapters []*DownloadedChapter, rng string, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	format := s.GetFormat()
	files, labels := chapterFolders(chapters, pageFormats(format, s.GetConvertImages()))
	info := EPUBInfo{Series: title, ReadingDirection: s.GetReadingDirection(), Labels: labels}
	return pack(outputdir, format, s.GetFilenameTemplate(), title, FilenameTemplateParts{
		Series: title,
		Number: rng,
		Title:  "bundle",
	}, files, info, progress)
}

// PackVolume packs the downloaded chapters of a volume into one file, laid
// out as a bundle is (a folder per chapter), and named with VolumeTemplate.
// chapters must be sorted by number.
func PackVolume(outputdir string, s grabber.Site, volume int, chapters []*DownloadedChapter, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	listed := make([]*grabber.Chapter, len(chapters))
	for i, c := range chapters {
		listed[i] = c.Chapter
	}
	format := s.GetFormat()
	files, labels := chapterFolders(chapters, pageFormats(format, s.GetConvertImages()))
	info := EPUBInfo{Series: title, Position: strconv.Itoa(volume), ReadingDirection: s.GetReadingDirection(), Labels: labels}
	return pack(outputdir, format, VolumeTemplate(s.GetFilenameTemplate()), title,
		NewVolumeFileTemplateParts(title, volume, listed), files, info, progress)
}

// pageFormats returns the source formats to convert pages from for the output
// format. EPUB readers can't show AVIF, so it's always converted for EPUB,
// whatever --convert-images says; the other formats use it as given.
func pageFormats(format string, formats grabber.ConvertFormats) grabber.ConvertFormats {
	if format != FormatEPUB || formats.Has(grabber.ConvertAVIF) {
		return formats
	}
	forced := grabber.ConvertFormats{grabber.ConvertAVIF: true}
	for f := range formats {
		forced[f] = true
	}
	return forced
}

// chapterFolders names the pages of several chapters for one archive, each
// chapter in its own folder (Chapter 0001/000.jpg, ...). It also returns the
// chapters' labels by folder, for the EPUB table of contents.
func chapterFolders(chapters []*DownloadedChapter, formats grabber.ConvertFormats) ([]File, map[string]string) {
	files := []File{}
	labels := map[string]string{}
	// Several chapters can share a number (a site re-releasing the same chapter,
	// or two genuinely distinct chapters both numbered, e.g. a pair of
	// season-finale entries). Each chapter gets its own "Chapter 0001/" folder
	// so chapter boundaries survive bundling, so repeated numbers are
	// disambiguated with a " (2)", " (3)" suffix instead of silently merging
	// both chapters' pages into one folder (which would lose the first one's).
	usedFolders := map[string]int{}
	for _, chapter := range chapters {
		base := SanitizeFilename(fmt.Sprintf("Chapter %s", paddedChapterNumber(chapter.GetNumber())))
		folder := base
		if n := usedFolders[base]; n > 0 {
			folder = fmt.Sprintf("%s (%d)", base, n+1)
		}
		usedFolders[base]++
		labels[folder] = chapterLabel(chapter.Chapter)

		for _, page := range namePages(chapter.Files, formats) {
			files = append(files, File{
				Name: fmt.Sprintf("%s/%s", folder, page.Name),
				Data: page.Data,
			})
		}
	}

	return files, labels
}

// chapterLabel names a chapter the way the filename template does (no padding,
// decimals kept), with its title when the title says more than the number
func chapterLabel(chapter *grabber.Chapter) string {
	number := formatChapterNumber(chapter.GetNumber())
	label := "Chapter " + number
	if title := chapter.GetTitle(); title != "" && title != number && title != label {
		label += " - " + title
	}
	return label
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
				notice.Warn("- warning: page %03d: converting %s to jpeg: %s (keeping the original)", i, ext, err)
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

func pack(outputdir, format, template, title string, parts FilenameTemplateParts, files []File, info EPUBInfo, progress func(page, progress int)) (string, error) {
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
		} else if format == FormatEPUB {
			name = filename + ".epub"
			info.Title = filename
			err = ArchiveEPUB(filepath.Join(outputdir, name), info, files, progress)
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
