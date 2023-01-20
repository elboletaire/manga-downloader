package downloader

import (
	"io"
	"sort"
	"sync"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// File represents a downloaded file
type File struct {
	Data []byte
	Page uint
}

// FetchChapter downloads all the pages of a chapter
func FetchChapter(site grabber.Site, chapter *grabber.Chapter) (files []*File, err error) {
	wg := sync.WaitGroup{}

	color.Blue("- downloading %s pages...", color.HiBlackString(chapter.GetTitle()))
	guard := make(chan struct{}, site.GetMaxConcurrency().Pages)

	for _, page := range chapter.Pages {
		guard <- struct{}{}
		wg.Add(1)
		go func(page grabber.Page) {
			defer wg.Done()

			file, err := FetchFile(http.RequestParams{
				URL:     page.URL,
				Referer: site.BaseUrl(),
			}, uint(page.Number))

			if err != nil {
				color.Red("- error downloading page %d of %s", page.Number, chapter.GetTitle())
				return
			}

			files = append(files, file)

			// release guard
			<-guard
		}(page)
	}
	wg.Wait()
	close(guard)

	// sort files by page number
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Page < files[j].Page
	})

	return
}

// FetchFile gets an online file returning a new *File with its contents
func FetchFile(params http.RequestParams, page uint) (file *File, err error) {
	body, err := http.Get(params)
	if err != nil {
		return
	}

	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return
	}

	file = &File{
		Data: data,
		Page: page,
	}

	return
}
