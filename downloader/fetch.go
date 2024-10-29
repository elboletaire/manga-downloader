package downloader

import (
	"fmt"
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

// ProgressCallback is a function type for progress updates with optional error
type ProgressCallback func(page, progress int, err error)

// FetchChapter downloads all the pages of a chapter
func FetchChapter(site grabber.Site, chapter *grabber.Chapter, onprogress ProgressCallback) (files []*File, err error) {
	wg := sync.WaitGroup{}
	guard := make(chan struct{}, site.GetMaxConcurrency().Pages)
	errChan := make(chan error, len(chapter.Pages)) // Buffer for all possible page errors
	done := make(chan bool)
	var mu sync.Mutex

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

			pn := int(page.Number)
			cp := pn * 100 / len(chapter.Pages)

			if err != nil {
				errChan <- fmt.Errorf("page %d: %w", page.Number, err)
				onprogress(pn, cp, err)
				return
			}

			mu.Lock()
			files = append(files, file)
			mu.Unlock()

			onprogress(pn, cp, nil)
		}(page)
	}

	go func() {
		wg.Wait()
		close(done)
		close(errChan)
	}()

	// Collect all errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	<-done
	close(guard)

	if len(errors) > 0 {
		return files, fmt.Errorf("failed to download %d pages", len(errors))
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
