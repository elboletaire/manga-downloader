// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	mangahttp "github.com/elboletaire/manga-downloader/http"
)

// withFastRetryDelay shrinks the package-level retry delay for the duration
// of a test, restoring the original value afterwards.
func withFastRetryDelay(t *testing.T) {
	t.Helper()
	original := retryDelay
	retryDelay = time.Millisecond
	t.Cleanup(func() {
		retryDelay = original
	})
}

// FetchChapter has to pass each page's own headers on, which is what lets a
// site bind a token to the URLs of one response (MangaPlus mints one per
// chapter, and a shared session would make chapters send each other's)
func TestFetchChapter_AppliesPageHeaders(t *testing.T) {
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		_, _ = w.Write([]byte("page"))
	}))
	defer server.Close()

	site := &fetchChapterTestSite{Grabber: &grabber.Grabber{
		URL: server.URL,
		Settings: &grabber.Settings{
			MaxConcurrency: grabber.MaxConcurrency{Chapters: 1, Pages: 1},
		},
	}}
	chapter := &grabber.Chapter{
		Number: 1,
		Pages: []grabber.Page{{
			Number:  1,
			URL:     server.URL + "/page",
			Headers: map[string]string{"Plus-Vw-Token": "0123456789abcdef0123456789abcdef"},
		}},
	}

	files, err := FetchChapter(site, chapter, func(page, progress int, err error) {})
	if err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	if len(files) != 1 || string(files[0].Data) != "page" {
		t.Fatalf("got %d files, want one holding the page", len(files))
	}
	if got := headers.Get("Plus-Vw-Token"); got != "0123456789abcdef0123456789abcdef" {
		t.Errorf("Plus-Vw-Token header = %q, want the page's own token", got)
	}
}

// fetchChapterTestSite is a grabber.Site stub: everything FetchChapter doesn't
// call is inherited from the base grabber
type fetchChapterTestSite struct {
	*grabber.Grabber
}

func (s fetchChapterTestSite) Test() (bool, error) { return true, nil }

func (s fetchChapterTestSite) FetchTitle() (string, error) { return "test", nil }

func (s fetchChapterTestSite) FetchChapters() (grabber.Filterables, []error) { return nil, nil }

func (s fetchChapterTestSite) FetchChapter(grabber.Filterable) (*grabber.Chapter, error) {
	return nil, errors.New("FetchChapter is not used by this test")
}

func TestFetchFile_RetriesOnGetFailure(t *testing.T) {
	withFastRetryDelay(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	file, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 1, nil)
	if err != nil {
		t.Fatalf("expected no error after retry, got: %v", err)
	}
	if string(file.Data) != "hello" {
		t.Errorf("expected file data %q, got %q", "hello", file.Data)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Errorf("expected 2 requests, got %d", got)
	}
}

func TestFetchFile_RetriesOnMidBodyReadFailure(t *testing.T) {
	withFastRetryDelay(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			// promise more bytes than we actually send, then close the
			// connection mid-body to simulate a "page cut at some point"
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer does not support hijacking")
			}
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack failed: %v", err)
			}
			defer conn.Close()
			bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nabc")
			bufrw.Flush()
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("full-body"))
	}))
	defer server.Close()

	file, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 1, nil)
	if err != nil {
		t.Fatalf("expected no error after retry, got: %v", err)
	}
	if string(file.Data) != "full-body" {
		t.Errorf("expected file data %q, got %q", "full-body", file.Data)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Errorf("expected 2 requests, got %d", got)
	}
}

func TestFetchFile_ExhaustsRetries(t *testing.T) {
	withFastRetryDelay(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 1, nil)
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Errorf("expected 2 requests (1 initial + 1 retry), got %d", got)
	}
}

func TestFetchFile_NoRetries(t *testing.T) {
	withFastRetryDelay(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 0, nil)
	if err == nil {
		t.Fatal("expected an error on first failure")
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Errorf("expected exactly 1 request (no retries), got %d", got)
	}
}

func TestFetchFile_AppliesTransform(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	transform := func(data []byte) ([]byte, error) {
		return append(data, []byte(" transformed")...), nil
	}

	file, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 1, transform)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if string(file.Data) != "hello transformed" {
		t.Errorf("expected transformed data %q, got %q", "hello transformed", file.Data)
	}
}

func TestFetchFile_RetriesOnTransformFailure(t *testing.T) {
	withFastRetryDelay(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	var calls int32
	transform := func(data []byte) ([]byte, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return nil, errors.New("boom")
		}
		return data, nil
	}

	file, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 1, transform)
	if err != nil {
		t.Fatalf("expected no error after retry, got: %v", err)
	}
	if string(file.Data) != "hello" {
		t.Errorf("expected file data %q, got %q", "hello", file.Data)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Errorf("expected 2 requests (transform failure re-fetches), got %d", got)
	}
}
