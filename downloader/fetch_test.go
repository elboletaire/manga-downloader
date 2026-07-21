package downloader

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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

	file, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 1)
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

	file, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 1)
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

	_, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 1)
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

	_, err := FetchFile(mangahttp.RequestParams{URL: server.URL}, 1, 0)
	if err == nil {
		t.Fatal("expected an error on first failure")
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Errorf("expected exactly 1 request (no retries), got %d", got)
	}
}
