package downloader

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"

	"github.com/voxelost/manga-downloader/grabber"
	"github.com/voxelost/manga-downloader/http"
)

// File represents a downloaded file
type File struct {
	Data []byte
	Page uint
}

// FetchChapter downloads all the pages of a chapter
func FetchChapter(site grabber.Site, chapter *grabber.Chapter) (files []*File, err error) {
	wg := sync.WaitGroup{}

	slog.Debug(fmt.Sprintf("downloading pages for %q", chapter.GetTitle()))
	// guard := make(chan struct{}, site.GetMaxConcurrency().Pages)

	for _, page := range chapter.Pages {
		// guard <- struct{}{}
		wg.Add(1)
		go func(page grabber.Page) {
			defer wg.Done()

			file, err := FetchFile(http.RequestParams{
				URL:     page.URL,
				Referer: site.BaseURL(),
			}, uint(page.Number))

			if err != nil {
				slog.Error(fmt.Sprintf("error downloading page %d of %q", page.Number, chapter.GetTitle()))
				return
			}

			files = append(files, file)

			// // release guard
			// <-guard
		}(page)
	}
	wg.Wait()
	// close(guard)

	// sort files by page number
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Page < files[j].Page
	})

	return files, nil
}

// FetchFile gets an online file returning a new *File with its contents
func FetchFile(params http.RequestParams, page uint) (file *File, err error) {
	body, err := http.Get(params)
	if err != nil {
		return nil, err
	}

	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	file = &File{
		Data: data,
		Page: page,
	}

	return file, nil
}
