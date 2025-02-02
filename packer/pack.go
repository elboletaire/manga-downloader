package packer

import (
	"fmt"
	"path/filepath"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
)

// DownloadedChapter represents a downloaded chapter (a Chapter + Files)
type DownloadedChapter struct {
	*grabber.Chapter
	Files []*downloader.File
}

// PackSingle packs a single downloaded chapter
func PackSingle(outputdir string, s grabber.Site, chapter *DownloadedChapter, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	return pack(outputdir, s.GetFilenameTemplate(), title, NewChapterFileTemplateParts(title, chapter.Chapter), chapter.Files, progress)
}

// PackBundle packs a bundle of downloaded chapters
func PackBundle(outputdir string, s grabber.Site, chapters []*DownloadedChapter, rng string, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	files := []*downloader.File{}
	for _, chapter := range chapters {
		files = append(files, chapter.Files...)
	}

	return pack(outputdir, s.GetFilenameTemplate(), title, FilenameTemplateParts{
		Series: title,
		Number: rng,
		Title:  "bundle",
	}, files, progress)
}

func pack(outputdir, template, title string, parts FilenameTemplateParts, files []*downloader.File, progress func(page, progress int)) (string, error) {
	filename, err := NewFilenameFromTemplate(template, parts)
	if err != nil {
		return "", fmt.Errorf("- error creating filename for chapter %s: %s", title, err.Error())
	}

	filename += ".cbz"

	if err = ArchiveCBZ(filepath.Join(outputdir, filename), files, progress); err != nil {
		return "", fmt.Errorf("- error saving file %s: %s", filename, err.Error())
	}

	return filename, nil
}
