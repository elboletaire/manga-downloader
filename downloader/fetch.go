package downloader

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

type File struct {
	Data []byte
	Name string
}

// FetchChapter downloads all the pages of a chapter
func FetchChapter(site grabber.Site, chapter grabber.Chapter) (files []*File, err error) {
	wg := sync.WaitGroup{}

	color.Blue("- downloading %s pages...", color.HiBlackString(chapter.GetTitle()))
	guard := make(chan struct{}, site.GetMaxConcurrency().Pages)

	for _, page := range chapter.Pages {
		guard <- struct{}{}
		wg.Add(1)
		go func(page grabber.Page) {
			defer wg.Done()

			filename := fmt.Sprintf("%03d.jpg", page.Number)
			file, err := FetchFile(http.RequestParams{
				URL:     page.URL,
				Referer: site.GetBaseUrl(),
			}, filename)

			if err != nil {
				color.Red("- error downloading page %s", filename)
				return
			}

			files = append(files, file)

			// release guard
			<-guard
		}(page)
	}

	wg.Wait()

	return
}

// FetchFile gets an online file returning a new *File with its contents
func FetchFile(params http.RequestParams, filename string) (file *File, err error) {
	body, err := http.Get(params)
	if err != nil {
		return
	}

	data := new(bytes.Buffer)
	io.Copy(data, body)
	if err != nil {
		return
	}

	file = &File{
		Data: data.Bytes(),
		Name: filename,
	}

	return
}
