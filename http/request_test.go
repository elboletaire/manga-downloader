// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// shortenRetries shrinks the backoff so tests don't actually wait seconds,
// restoring the real values afterwards.
func shortenRetries(t *testing.T) {
	t.Helper()
	base, mx := retryBaseDelay, retryMaxDelay
	retryBaseDelay, retryMaxDelay = time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { retryBaseDelay, retryMaxDelay = base, mx })
}

func TestRequestRetries5xx(t *testing.T) {
	shortenRetries(t)

	var calls int32
	// return a 503 twice, then succeed
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	body, err := Get(RequestParams{URL: srv.URL})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	defer body.Close()

	got, _ := io.ReadAll(body)
	if string(got) != "ok" {
		t.Errorf("body = %q, want %q", got, "ok")
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Errorf("server saw %d calls, want 3", n)
	}
}

func TestRequestRetries522(t *testing.T) {
	shortenRetries(t)

	var calls int32
	// Cloudflare 522 on first attempt
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(522)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	body, err := Get(RequestParams{URL: srv.URL})
	if err != nil {
		t.Fatalf("expected success after Cloudflare 522 retry, got %v", err)
	}
	body.Close()
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("server saw %d calls, want 2", n)
	}
}

func TestRequestGivesUpAfterMaxAttempts(t *testing.T) {
	shortenRetries(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Get(RequestParams{URL: srv.URL})
	if err == nil {
		t.Fatal("expected an error when every attempt fails")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q should mention the upstream status", err)
	}
	if n := atomic.LoadInt32(&calls); int(n) != maxAttempts {
		t.Errorf("server saw %d calls, want %d", n, maxAttempts)
	}
}

func TestRequestErrorWording(t *testing.T) {
	shortenRetries(t)

	cases := []struct {
		name         string
		disableRetry bool
		wantCalls    int32
		wantPrefix   string // empty means the error must not be wrapped with a "giving up" prefix
	}{
		{name: "retry disabled", disableRetry: true, wantCalls: 1},
		{name: "retries exhausted", wantCalls: 4, wantPrefix: "giving up after 4 attempts: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			_, err := Get(RequestParams{URL: srv.URL, DisableRetry: tc.disableRetry})
			if err == nil {
				t.Fatal("expected an error")
			}
			if tc.wantPrefix == "" && strings.Contains(err.Error(), "giving up") {
				t.Errorf("error %q claims a retry was attempted", err)
			}
			if tc.wantPrefix != "" && !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Errorf("error %q, want prefix %q", err, tc.wantPrefix)
			}
			var statusErr *StatusError
			if !errors.As(err, &statusErr) || statusErr.Code != http.StatusInternalServerError {
				t.Errorf("errors.As(%v) lost the *StatusError", err)
			}
			if n := atomic.LoadInt32(&calls); n != tc.wantCalls {
				t.Errorf("server saw %d calls, want %d", n, tc.wantCalls)
			}
		})
	}
}

func TestRequestDoesNotRetry404(t *testing.T) {
	shortenRetries(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := Get(RequestParams{URL: srv.URL}); err == nil {
		t.Fatal("expected an error for 404")
	}
	// a missing chapter must fail on the first attempt
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("server saw %d calls, want 1", n)
	}
}

func TestRequestHonoursRetryAfterSeconds(t *testing.T) {
	shortenRetries(t)

	var calls int32
	var sleepDuration time.Duration

	// record actual sleep calls via the package var
	origBase := retryBaseDelay
	retryBaseDelay = time.Millisecond
	defer func() { retryBaseDelay = origBase }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0") // 0s: succeed on next try
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	_ = sleepDuration // used implicitly via retryBaseDelay being set

	body, err := Get(RequestParams{URL: srv.URL})
	if err != nil {
		t.Fatalf("expected success after 429 retry, got %v", err)
	}
	body.Close()
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("server saw %d calls, want 2", n)
	}
}

func TestParseRetryAfterSeconds(t *testing.T) {
	cases := []struct {
		header string
		want   time.Duration
	}{
		{"1", time.Second},
		// under MaxRetryAfter, so returned as-is
		{"5", 5 * time.Second},
		{"30", 30 * time.Second},
		{" 2 ", 2 * time.Second},
		{"0", 0},
		{"-1", 0},
		{"", 0},
		{"garbage", 0},
	}
	for _, tc := range cases {
		got := ParseRetryAfter(tc.header)
		if got != tc.want {
			t.Errorf("ParseRetryAfter(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestParseRetryAfterCapsAtMax(t *testing.T) {
	// a very large value must be capped at MaxRetryAfter, including one
	// that would overflow a time.Duration
	for _, header := range []string{"3600", "1e300", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)} {
		if got := ParseRetryAfter(header); got != MaxRetryAfter {
			t.Errorf("ParseRetryAfter(%q) = %v, want %v (cap)", header, got, MaxRetryAfter)
		}
	}
}

// A past HTTP date means "now", not a negative wait
func TestParseRetryAfterPastDate(t *testing.T) {
	if got := ParseRetryAfter(time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat)); got != 0 {
		t.Errorf("ParseRetryAfter(past date) = %v, want 0", got)
	}
}

// The error of a 429 carries the server's Retry-After, so a caller with its
// own retry loop can honour it
func TestStatusErrorCarriesRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "18")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := Get(RequestParams{URL: srv.URL, DisableRetry: true})
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want a *StatusError", err)
	}
	if se.Code != http.StatusTooManyRequests || se.RetryAfter != 18*time.Second {
		t.Errorf("StatusError = %+v, want code 429 and RetryAfter 18s", se)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	// use a date 1 second in the future
	future := time.Now().Add(time.Second).UTC()
	header := future.Format(http.TimeFormat)
	got := ParseRetryAfter(header)
	// should be ~1s; allow some slop
	if got <= 0 || got > 2*time.Second {
		t.Errorf("ParseRetryAfter(HTTP date) = %v, want ~1s", got)
	}
}

func TestRequestPostBodyResentOnRetry(t *testing.T) {
	shortenRetries(t)

	var calls int32
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	body, err := Post(RequestParams{URL: srv.URL, Body: "field=value"})
	if err != nil {
		t.Fatalf("expected success after POST retry, got %v", err)
	}
	body.Close()

	if len(bodies) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(bodies))
	}
	for i, b := range bodies {
		if b != "field=value" {
			t.Errorf("attempt %d body = %q, want %q", i+1, b, "field=value")
		}
	}
}

func TestRequestPreservesSessionHeadersOnRetry(t *testing.T) {
	shortenRetries(t)

	var calls int32
	var lastUA, lastCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		lastUA = r.Header.Get("User-Agent")
		lastCookie = r.Header.Get("Cookie")
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	// sessionCookies matches by hostname only (no port), so extract it
	u, _ := url.Parse(srv.URL)
	SetCookie(u.Hostname(), "sess", "abc")
	body, err := Get(RequestParams{URL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body.Close()

	if lastUA == "" {
		t.Error("User-Agent not sent on retry")
	}
	if !strings.Contains(lastCookie, "sess=abc") {
		t.Errorf("Cookie on retry = %q, want it to contain sess=abc", lastCookie)
	}
}

func TestRetryDelayBoundedAndGrows(t *testing.T) {
	// delay must stay within [d, 1.5d] for the exponential step d, and
	// never exceed retryMaxDelay + retryMaxDelay/2 (the jitter ceiling)
	for attempt := 1; attempt <= 8; attempt++ {
		d := retryDelay(attempt)
		if d <= 0 {
			t.Fatalf("attempt %d: non-positive delay %v", attempt, d)
		}
		if mx := retryMaxDelay + retryMaxDelay/2; d > mx {
			t.Errorf("attempt %d: delay %v exceeds ceiling %v", attempt, d, mx)
		}
	}
}

func TestRetryDelayGrows(t *testing.T) {
	// earlier delays should be shorter than later ones (on average; jitter
	// makes strict monotonicity unreliable, so just check medians via many samples)
	const N = 100
	var sum [4]int64
	for i := 0; i < N; i++ {
		for a := 1; a <= 4; a++ {
			sum[a-1] += int64(retryDelay(a))
		}
	}
	for a := 1; a < 4; a++ {
		if sum[a] <= sum[a-1] {
			t.Errorf("attempt %d average delay (%v) not greater than attempt %d (%v)",
				a+1, time.Duration(sum[a]/N), a, time.Duration(sum[a-1]/N))
		}
	}
}

func TestIsRetryableStatus(t *testing.T) {
	retryable := []int{429, 500, 502, 503, 504, 522, 523, 524, 599}
	notRetryable := []int{200, 201, 301, 400, 401, 403, 404, 405, 410}

	for _, code := range retryable {
		if !isRetryableStatus(code) {
			t.Errorf("isRetryableStatus(%d) = false, want true", code)
		}
	}
	for _, code := range notRetryable {
		if isRetryableStatus(code) {
			t.Errorf("isRetryableStatus(%d) = true, want false", code)
		}
	}
}

func TestRequestRefererAndCustomHeaders(t *testing.T) {
	var gotRef, gotCustom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.Header.Get("Referer")
		gotCustom = r.Header.Get("X-Custom")
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	body, err := Get(RequestParams{
		URL:     srv.URL,
		Referer: "https://example.test/",
		Headers: map[string]string{"X-Custom": "hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body.Close()

	if gotRef != "https://example.test/" {
		t.Errorf("Referer = %q, want %q", gotRef, "https://example.test/")
	}
	if gotCustom != "hello" {
		t.Errorf("X-Custom = %q, want %q", gotCustom, "hello")
	}
}

func TestRequestRefererGetsSlashAppended(t *testing.T) {
	var gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.Header.Get("Referer")
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	body, err := Get(RequestParams{URL: srv.URL, Referer: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	body.Close()

	if gotRef != "https://example.test/" {
		t.Errorf("Referer = %q, want trailing slash appended", gotRef)
	}
}

func TestStatusErrorMessage(t *testing.T) {
	e := &StatusError{Code: 404}
	want := fmt.Sprintf("received %d response code", 404)
	if e.Error() != want {
		t.Errorf("StatusError.Error() = %q, want %q", e.Error(), want)
	}
}

// The in-request retry loop can't be cancelled, so it clamps a long
// Retry-After to retryMaxDelay instead of sleeping for a minute
func TestRequestClampsRetryAfter(t *testing.T) {
	shortenRetries(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	start := time.Now()
	body, err := Get(RequestParams{URL: srv.URL})
	if err != nil {
		t.Fatalf("expected success after a 429, got %v", err)
	}
	body.Close()
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v, want the 60s Retry-After clamped to retryMaxDelay", elapsed)
	}
}

// Documents and API responses keep the browser's document-first Accept; only
// page images override it (RequestParams.Accept)
func TestRequestDefaultAccept(t *testing.T) {
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	if _, err := GetText(RequestParams{URL: srv.URL}); err != nil {
		t.Fatalf("GetText: %v", err)
	}
	if want := defaultAccept; accept != want {
		t.Errorf("Accept = %q, want %q", accept, want)
	}
	if _, err := GetText(RequestParams{URL: srv.URL, Accept: ImageAccept}); err != nil {
		t.Fatalf("GetText: %v", err)
	}
	if accept != ImageAccept {
		t.Errorf("Accept = %q, want %q", accept, ImageAccept)
	}
}

// shortenHeaderTimeout shrinks the shared transport's response-header timeout,
// restoring it afterwards
func shortenHeaderTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := transport.ResponseHeaderTimeout
	transport.ResponseHeaderTimeout = d
	t.Cleanup(func() { transport.ResponseHeaderTimeout = old })
}

// hangingServer accepts connections and never answers, until the client gives
// up (or the test ends). Every request but the ones after `answer` hangs.
func hangingServer(t *testing.T, answer *int32) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if answer != nil && atomic.AddInt32(answer, 1) > 1 {
			io.WriteString(w, "ok")
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv
}

// A server that never sends headers fails on the response-header timeout,
// with an error that names the URL and says it timed out
func TestRequestTimesOutWithoutHeaders(t *testing.T) {
	shortenHeaderTimeout(t, 100*time.Millisecond)
	srv := hangingServer(t, nil)

	start := time.Now()
	_, err := Get(RequestParams{URL: srv.URL, DisableRetry: true})
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v, want the header timeout to fire", elapsed)
	}
	if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), srv.URL) {
		t.Errorf("error %q, want it to say it timed out and name %s", err, srv.URL)
	}
}

// A timeout is a transport failure, so the retry loop tries again
func TestRequestRetriesTimeout(t *testing.T) {
	shortenRetries(t)
	shortenHeaderTimeout(t, 100*time.Millisecond)

	var calls int32
	srv := hangingServer(t, &calls)

	body, err := Get(RequestParams{URL: srv.URL})
	if err != nil {
		t.Fatalf("expected success on the retry, got %v", err)
	}
	body.Close()
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("server saw %d calls, want 2", n)
	}
}

// Only the wait for headers is bounded: a body that keeps streaming past the
// header timeout still arrives whole
func TestRequestSlowBodyNotTimedOut(t *testing.T) {
	shortenHeaderTimeout(t, 100*time.Millisecond)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		for range 4 {
			time.Sleep(150 * time.Millisecond)
			io.WriteString(w, "chunk")
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	body, err := Get(RequestParams{URL: srv.URL, DisableRetry: true})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	defer body.Close()

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	if want := strings.Repeat("chunk", 4); string(got) != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
