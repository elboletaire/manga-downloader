package packer

import (
	"fmt"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
)

// DownloadedChapter represents a downloaded chapter (a Chapter + Files)
type DownloadedChapter struct {
	*grabber.Chapter
	Files []*downloader.File
}

// PackSingle packs a single downloaded chapter
func PackSingle(s grabber.Site, chapter *DownloadedChapter) (string, error) {
	title, _ := s.FetchTitle()
	return pack(s.GetFilenameTemplate(), title, NewChapterFileTemplateParts(title, chapter.Chapter), chapter.Files)
}

// PackBundle packs a bundle of downloaded chapters
func PackBundle(s grabber.Site, chapters []*DownloadedChapter, rng string) (string, error) {
	title, _ := s.FetchTitle()
	files := []*downloader.File{}
	for _, chapter := range chapters {
		files = append(files, chapter.Files...)
	}

	return pack(s.GetFilenameTemplate(), title, FilenameTemplateParts{
		Series: title,
		Number: rng,
		Title:  "bundle",
	}, files)
}

func pack(template, title string, parts FilenameTemplateParts, files []*downloader.File) (string, error) {
	filename, err := NewFilenameFromTemplate(template, parts)
	if err != nil {
		return "", fmt.Errorf("- error creating filename for chapter %s: %s", title, err.Error())
	}

	filename += ".cbz"

	if err = ArchiveCBZ(filename, files); err != nil {
		return "", fmt.Errorf("- error saving file %s: %s", filename, err.Error())
	}

	return filename, nil
}
