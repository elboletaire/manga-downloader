// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
)

// retryDelay is the base delay between retry attempts (multiplied by the
// attempt number). It's a package-level var so tests can shrink it.
var retryDelay = time.Second

// File represents a downloaded file
type File struct {
	Data []byte
	Page uint
}

// ProgressCallback is a function type for progress updates with optional error
type ProgressCallback func(page, progress int, err error)

// FetchChapter downloads all the pages of a chapter
func FetchChapter(site grabber.Site, chapter *grabber.Chapter, onprogress ProgressCallback) ([]*File, error) {
	wg := sync.WaitGroup{}
	guard := make(chan struct{}, site.GetMaxConcurrency().Pages)
	errChan := make(chan error, 1)
	done := make(chan bool)
	// A local (not the named return) so page goroutines write into a slice that
	// can never be nilled out from under them. If a page fails and the function
	// returns early below, `return nil, err` sets the named return to nil - any
	// page goroutine still downloading would then write to a nil slice and panic
	// ("index out of range"). res is only handed to the caller on the success
	// path, so such late writes are just garbage the GC reclaims.
	res := make([]*File, len(chapter.Pages)) // Pre-allocate slice with correct size

	for i, page := range chapter.Pages {
		guard <- struct{}{}
		wg.Add(1)
		go func(page grabber.Page, idx int) {
			defer wg.Done()

			file, err := FetchFile(http.RequestParams{
				URL:     page.URL,
				Referer: site.BaseUrl(),
			}, uint(page.Number), site.GetRetries(), page.Transform)

			if err != nil {
				select {
				case errChan <- fmt.Errorf("page %d: %w", page.Number, err):
					onprogress(idx, idx, err)
				default:
				}
				<-guard
				return
			}

			res[idx] = file         // Store file directly in pre-allocated slice
			onprogress(1, idx, nil) // Progress by 1 page at a time
			<-guard
		}(page, i)
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errChan:
		close(guard)
		return nil, err
	case <-done:
		close(guard)
	}

	// sort files by page number
	sort.SliceStable(res, func(i, j int) bool {
		return res[i].Page < res[j].Page
	})

	return res, nil
}

// FetchFile gets an online file returning a new *File with its contents.
// On failure (either the GET itself, a mid-body read, or the optional
// transform) it retries up to `retries` additional times, with a short
// growing delay between attempts. transform, if non-nil, post-processes the
// downloaded bytes (e.g. a site's own page.Transform) before they're stored.
func FetchFile(params http.RequestParams, page uint, retries uint8, transform func([]byte) ([]byte, error)) (file *File, err error) {
	for attempt := uint8(0); ; attempt++ {
		var data []byte
		data, err = fetchFileOnce(params)
		if err == nil && transform != nil {
			data, err = transform(data)
		}
		if err == nil {
			file = &File{
				Data: data,
				Page: page,
			}
			return
		}

		if attempt >= retries {
			return
		}

		time.Sleep(retryDelay * time.Duration(attempt+1))
	}
}

// fetchFileOnce performs a single GET + body read attempt
func fetchFileOnce(params http.RequestParams) (data []byte, err error) {
	body, err := http.Get(params)
	if err != nil {
		return
	}
	defer body.Close()

	data, err = io.ReadAll(body)
	return
}
