// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
)

// retryDelay is the base delay between retry attempts (multiplied by the
// attempt number). rateLimitDelay is the wait after a 429 whose response
// didn't say how long to back off: a rate limit needs longer than a blip.
// Package-level vars so tests can shrink them.
var (
	retryDelay     = time.Second
	rateLimitDelay = 10 * time.Second
)

// File represents a downloaded file
type File struct {
	Data []byte
	Page uint
}

// ProgressCallback is a function type for progress updates with optional error
type ProgressCallback func(page, progress int, err error)

// FetchChapter downloads all the pages of a chapter. Cancelling ctx cuts
// short the waits between retries; requests already under way still finish.
func FetchChapter(ctx context.Context, site grabber.Site, chapter *grabber.Chapter, onprogress ProgressCallback) ([]*File, error) {
	var pace func()
	if p, ok := site.(grabber.PagePacer); ok {
		pace = p.WaitPage
	}

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

			referer := site.BaseUrl()
			if page.NoReferer {
				referer = ""
			}
			file, err := fetchFile(ctx, http.RequestParams{
				URL:     page.URL,
				Referer: referer,
				Headers: page.Headers,
				Accept:  http.ImageAccept,
			}, uint(page.Number), site.GetRetries(), page.Transform, pace, page.RetryFlaky)

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
// growing delay between attempts (or the server's Retry-After after a 429,
// see retryWait). transform, if non-nil, post-processes the downloaded bytes.
// Cancelling ctx abandons the wait before a retry.
func FetchFile(ctx context.Context, params http.RequestParams, page uint, retries uint8, transform func([]byte) ([]byte, error)) (*File, error) {
	return fetchFile(ctx, params, page, retries, transform, nil, false)
}

// fetchFile is FetchFile with an optional pace, called before every attempt,
// and retryFlaky (Page.RetryFlaky)
func fetchFile(ctx context.Context, params http.RequestParams, page uint, retries uint8, transform func([]byte) ([]byte, error), pace func(), retryFlaky bool) (file *File, err error) {
	orig := params.URL
	for attempt := uint8(0); ; attempt++ {
		if pace != nil {
			pace()
		}
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

		wait := retryWait(err, attempt)
		limit := retries
		if retryFlaky && retries > 0 && isFlaky(err) {
			limit, wait = max(retries, flakyRetries), retryDelay
		}
		if attempt >= limit {
			return
		}
		if retryFlaky && isFlaky(err) {
			params.URL = bustCache(orig)
		}

		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, fmt.Errorf("%w (while waiting to retry after: %w)", ctx.Err(), err)
		case <-t.C:
		}
	}
}

// flakyRetries is how many times an opted-in page that fails is retried at least.
const flakyRetries = 6

// isFlaky reports whether err is a 404 or a 5xx, the failures an opted-in
// origin gives at random (a 429 keeps its own Retry-After handling).
func isFlaky(err error) bool {
	var se *http.StatusError
	return errors.As(err, &se) && (se.Code == stdhttp.StatusNotFound || se.Code >= 500)
}

// bustCache appends a unique query to the raw URL, leaving any existing one
// byte for byte (it may be signed), so a retry skips a CDN's cached 404.
func bustCache(rawURL string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "retry=" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// retryWait is the delay before retrying after attempt (0-based) failed with
// err. A 429 waits out its Retry-After (or rateLimitDelay): the linear delay
// would retry while the rate limit still applies.
func retryWait(err error, attempt uint8) time.Duration {
	var se *http.StatusError
	if errors.As(err, &se) && se.Code == stdhttp.StatusTooManyRequests {
		if se.RetryAfter > 0 {
			return se.RetryAfter
		}
		return rateLimitDelay
	}
	return retryDelay * time.Duration(attempt+1)
}

// fetchFileOnce performs a single GET + body read attempt. DisableRetry is
// set so the HTTP package's own retry loop doesn't fire — FetchFile has its
// own retry loop and the two would otherwise multiply for every 5xx page.
func fetchFileOnce(params http.RequestParams) (data []byte, err error) {
	params.DisableRetry = true
	body, err := http.Get(params)
	if err != nil {
		return
	}
	defer body.Close()

	data, err = io.ReadAll(body)
	return
}
