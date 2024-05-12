package downloader

import (
	"io"
	"sort"
	"sync"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
)

// File represents a downloaded file
type File struct {
	Data []byte
	Page uint
}

// FetchChapter downloads all the pages of a chapter
func FetchChapter(site grabber.Site, chapter *grabber.Chapter, onprogress func(page, progress int)) (files []*File, err error) {
	wg := sync.WaitGroup{}
	guard := make(chan struct{}, site.GetMaxConcurrency().Pages)
	errChan := make(chan error, 1)
	done := make(chan bool)

	for _, page := range chapter.Pages {
		guard <- struct{}{}
		wg.Add(1)
		go func(page grabber.Page) {
			// release waitgroup and guard when done
			defer wg.Done()
			defer func() { <-guard }()

			file, err := FetchFile(http.RequestParams{
				URL:     page.URL,
				Referer: site.BaseUrl(),
			}, uint(page.Number))

			if err != nil {
				select {
				case errChan <- err:
				default:
				}
				return
			}

			files = append(files, file)
			pn := int(page.Number)
			cp := pn * 100 / len(chapter.Pages)

			onprogress(pn, cp)
		}(page)
	}

	go func() {
		wg.Wait()
		// signal that all goroutines have completed
		close(done)
	}()

	select {
	// in case of error, return the very first one
	case err := <-errChan:
		close(guard)
		return nil, err
	case <-done:
		// all goroutines finished successfully, continue
	}

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
		// TODO: should retry at least once (configurable)
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
