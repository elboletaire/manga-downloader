package packer

import (
	"fmt"
	"os"
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
	return pack(outputdir, s.GetFilenameTemplate(), title, NewChapterFileTemplateParts(title, chapter.Chapter), namePages(chapter.Files), progress)
}

// PackBundle packs a bundle of downloaded chapters, grouping each chapter's
// pages into its own folder inside the archive (Chapter 0001/000.jpg, ...)
// so chapter boundaries survive bundling instead of a single flat renumbering.
func PackBundle(outputdir string, s grabber.Site, chapters []*DownloadedChapter, rng string, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	files := []File{}
	for _, chapter := range chapters {
		folder := SanitizeFilename(fmt.Sprintf("Chapter %s", paddedChapterNumber(chapter.GetNumber())))
		for _, page := range namePages(chapter.Files) {
			files = append(files, File{
				Name: fmt.Sprintf("%s/%s", folder, page.Name),
				Data: page.Data,
			})
		}
	}

	return pack(outputdir, s.GetFilenameTemplate(), title, FilenameTemplateParts{
		Series: title,
		Number: rng,
		Title:  "bundle",
	}, files, progress)
}

// namePages names a chapter's pages sequentially (000.jpg, 001.jpg, ...),
// restarting at 000 for each call.
func namePages(pages []*downloader.File) []File {
	named := make([]File, len(pages))
	for i, page := range pages {
		named[i] = File{
			Name: fmt.Sprintf("%03d.jpg", i),
			Data: page.Data,
		}
	}
	return named
}

func pack(outputdir, template, title string, parts FilenameTemplateParts, files []File, progress func(page, progress int)) (string, error) {
	parts.Version = 1

	for {
		filename, err := NewFilenameFromTemplate(template, parts)
		if err != nil {
			return "", fmt.Errorf("- error creating filename for chapter %s: %s", title, err.Error())
		}

		filename += ".cbz"

		err = ArchiveCBZ(filepath.Join(outputdir, filename), files, progress)
		if os.IsExist(err) {
			parts.Version++
			continue
		}
		if err != nil {
			return "", fmt.Errorf("- error saving file %s: %s", filename, err.Error())
		}

		return filename, nil
	}
}
