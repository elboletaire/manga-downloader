// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

	files, err := FetchChapter(context.Background(), site, chapter, func(page, progress int, err error) {})
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

// Pages are sent with the site's Referer unless they opt out: some image
// hosts answer 403 to any request that carries one
func TestFetchChapter_Referer(t *testing.T) {
	cases := []struct {
		name      string
		noReferer bool
		wantSent  bool
	}{
		{"default sends the site", false, true},
		{"NoReferer sends none", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
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
				Pages:  []grabber.Page{{Number: 1, URL: server.URL + "/page", NoReferer: c.noReferer}},
			}

			if _, err := FetchChapter(context.Background(), site, chapter, func(page, progress int, err error) {}); err != nil {
				t.Fatalf("FetchChapter: %v", err)
			}
			_, sent := headers["Referer"]
			if sent != c.wantSent {
				t.Errorf("Referer sent = %v (%q), want %v", sent, headers.Get("Referer"), c.wantSent)
			}
		})
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

	file, err := FetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL}, 1, 1, nil)
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

	file, err := FetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL}, 1, 1, nil)
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

	_, err := FetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL}, 1, 1, nil)
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

	_, err := FetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL}, 1, 0, nil)
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

	file, err := FetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL}, 1, 1, transform)
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

	file, err := FetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL}, 1, 1, transform)
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

// rateLimitedServer answers 429 to the first request (with the given
// Retry-After, if any), then serves the page. It records each request's arrival.
func rateLimitedServer(t *testing.T, retryAfter func() string) (*httptest.Server, *[]time.Time) {
	t.Helper()
	var mu sync.Mutex
	var hits []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, time.Now())
		n := len(hits)
		mu.Unlock()
		if n == 1 {
			if ra := retryAfter(); ra != "" {
				w.Header().Set("Retry-After", ra)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("page"))
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

// A 429 is retried after the delay its Retry-After asks for, in seconds or as
// an HTTP date, rather than the short linear delay used for other failures
func TestFetchFile_429HonoursRetryAfter(t *testing.T) {
	withFastRetryDelay(t)
	cases := []struct {
		name       string
		retryAfter func() string
		// the HTTP date only has whole seconds, so it can land up to one
		// second early
		min time.Duration
	}{
		{"seconds", func() string { return "1" }, time.Second},
		{"HTTP date", func() string { return time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat) }, time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server, hits := rateLimitedServer(t, c.retryAfter)

			file, err := FetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL}, 1, 1, nil)
			if err != nil {
				t.Fatalf("expected the retry to succeed, got: %v", err)
			}
			if string(file.Data) != "page" {
				t.Errorf("file data = %q, want %q", file.Data, "page")
			}
			if len(*hits) != 2 {
				t.Fatalf("server saw %d requests, want 2", len(*hits))
			}
			if gap := (*hits)[1].Sub((*hits)[0]); gap < c.min || gap > 3*time.Second {
				t.Errorf("retried after %v, want the Retry-After delay (>= %v)", gap, c.min)
			}
		})
	}
}

// A 429 without Retry-After waits rateLimitDelay, not retryDelay
func TestFetchFile_429WithoutRetryAfter(t *testing.T) {
	withFastRetryDelay(t)
	orig := rateLimitDelay
	rateLimitDelay = 300 * time.Millisecond
	t.Cleanup(func() { rateLimitDelay = orig })

	server, hits := rateLimitedServer(t, func() string { return "" })
	if _, err := FetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL}, 1, 1, nil); err != nil {
		t.Fatalf("expected the retry to succeed, got: %v", err)
	}
	if gap := (*hits)[1].Sub((*hits)[0]); gap < rateLimitDelay {
		t.Errorf("retried after %v, want at least rateLimitDelay (%v)", gap, rateLimitDelay)
	}
}

// Cancelling during a Retry-After wait returns at once, so the app's Cancel
// doesn't hang for up to a minute
func TestFetchFile_CancelDuringBackoff(t *testing.T) {
	server, hits := rateLimitedServer(t, func() string { return "60" })

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	_, err := FetchFile(ctx, mangahttp.RequestParams{URL: server.URL}, 1, 3, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	var se *mangahttp.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusTooManyRequests {
		t.Errorf("err = %v, want it to keep the 429 that caused the wait", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("returned after %v, want right after the cancel", elapsed)
	}
	if len(*hits) != 1 {
		t.Errorf("server saw %d requests, want 1 (no retry after cancelling)", len(*hits))
	}
}

// pacedSite is a fetchChapterTestSite that also paces its pages
type pacedSite struct {
	fetchChapterTestSite
	waits atomic.Int32
}

func (p *pacedSite) WaitPage() { p.waits.Add(1) }

// FetchChapter asks a PagePacer site for a slot before every page request,
// retries included
func TestFetchChapter_PacesPages(t *testing.T) {
	withFastRetryDelay(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// the first request fails, so one page needs a retry
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("page"))
	}))
	defer server.Close()

	site := &pacedSite{fetchChapterTestSite: fetchChapterTestSite{Grabber: &grabber.Grabber{
		URL: server.URL,
		Settings: &grabber.Settings{
			MaxConcurrency: grabber.MaxConcurrency{Chapters: 1, Pages: 1},
			Retry:          1,
		},
	}}}
	chapter := &grabber.Chapter{Number: 1, Pages: []grabber.Page{
		{Number: 1, URL: server.URL + "/1"},
		{Number: 2, URL: server.URL + "/2"},
		{Number: 3, URL: server.URL + "/3"},
	}}

	if _, err := FetchChapter(context.Background(), site, chapter, func(page, progress int, err error) {}); err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	if got, want := site.waits.Load(), requests.Load(); got != want || want != 4 {
		t.Errorf("WaitPage called %d times for %d requests, want 4 each", got, want)
	}
}

// Page images get an image Accept: imgur answers a document-first one with an
// HTML viewer page instead of the image
func TestFetchChapter_ImageAccept(t *testing.T) {
	var accept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
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
		Pages:  []grabber.Page{{Number: 1, URL: server.URL + "/page"}},
	}

	if _, err := FetchChapter(context.Background(), site, chapter, func(page, progress int, err error) {}); err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	if accept != mangahttp.ImageAccept {
		t.Errorf("Accept = %q, want the image Accept %q", accept, mangahttp.ImageAccept)
	}
}

// serveStatus answers the first n requests with status, recording each URI
func serveStatus(status, n int, uris *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*uris = append(*uris, r.URL.RequestURI())
		if len(*uris) <= n {
			w.WriteHeader(status)
			return
		}
		w.Write([]byte("ok"))
	}))
}

func TestFetchFile_RetryFlakyBustsCache(t *testing.T) {
	withFastRetryDelay(t)

	var uris []string
	server := serveStatus(http.StatusNotFound, 3, &uris)
	defer server.Close()

	file, err := fetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL + "/a.jpg?sig=B&x=a%2Fb"}, 1, 1, nil, nil, true)
	if err != nil || string(file.Data) != "ok" {
		t.Fatalf("expected the retries to succeed, got %v", err)
	}
	if len(uris) != 4 || uris[0] != "/a.jpg?sig=B&x=a%2Fb" {
		t.Fatalf("unexpected requests %q", uris)
	}
	for _, u := range uris[1:] {
		if !strings.HasPrefix(u, "/a.jpg?sig=B&x=a%2Fb&retry=") || strings.Count(u, "retry=") != 1 {
			t.Errorf("retry %q should keep the query and add one retry=", u)
		}
	}
}

func TestFetchFile_NotFoundWithoutOptInIsRetriedPlain(t *testing.T) {
	withFastRetryDelay(t)

	var uris []string
	server := serveStatus(http.StatusNotFound, 100, &uris)
	defer server.Close()

	if _, err := fetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL + "/a.jpg"}, 1, 1, nil, nil, false); err == nil {
		t.Fatal("expected an error")
	}
	if len(uris) != 2 || uris[1] != "/a.jpg" {
		t.Errorf("expected one plain retry only, got %q", uris)
	}
}

func TestFetchFile_RetryFlakyZeroRetries(t *testing.T) {
	withFastRetryDelay(t)

	var uris []string
	server := serveStatus(http.StatusNotFound, 100, &uris)
	defer server.Close()

	fetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL + "/a.jpg"}, 1, 0, nil, nil, true)
	if len(uris) != 1 {
		t.Errorf("--retry 0 must not retry, got %q", uris)
	}
}

func TestFetchFile_RetryFlaky5xx(t *testing.T) {
	withFastRetryDelay(t)

	var uris []string
	server := serveStatus(http.StatusInternalServerError, 5, &uris)
	defer server.Close()

	file, err := fetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL + "/a.jpg"}, 1, 1, nil, nil, true)
	if err != nil || string(file.Data) != "ok" || len(uris) != 6 {
		t.Fatalf("expected success on the 6th request, got %v after %d", err, len(uris))
	}
}

func TestFetchFile_5xxWithoutOptInKeepsPlainRetry(t *testing.T) {
	withFastRetryDelay(t)

	var uris []string
	server := serveStatus(http.StatusInternalServerError, 100, &uris)
	defer server.Close()

	if _, err := fetchFile(context.Background(), mangahttp.RequestParams{URL: server.URL + "/a.jpg"}, 1, 1, nil, nil, false); err == nil {
		t.Fatal("expected an error")
	}
	if len(uris) != 2 || uris[1] != "/a.jpg" {
		t.Errorf("expected one plain retry only, got %q", uris)
	}
}
