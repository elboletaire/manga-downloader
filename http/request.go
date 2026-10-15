// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Params is an interface for request parameters
type Params interface {
	GetURL() string
	GetReferer() string
}

// RequestParams is a struct for base request parameters
type RequestParams struct {
	URL     string
	Referer string
	Origin  string
	// Headers are extra headers sent with the request (e.g. a CSRF token
	// read off a previously fetched page)
	Headers map[string]string
	// Form, if set, is sent as an application/x-www-form-urlencoded POST
	// body instead of an empty one
	Form url.Values
	// Body is an optional url-encoded form body; when set, it's sent as
	// application/x-www-form-urlencoded (used by wp-admin/admin-ajax.php
	// style endpoints, i.e. utoon.us)
	Body string
	// DisableRetry disables the HTTP-level exponential-backoff retry for
	// this request. Set it when the caller (e.g. downloader.FetchFile) has
	// its own retry loop so the two mechanisms don't multiply into absurd
	// wait times for page downloads.
	DisableRetry bool
	// Accept overrides the default Accept header. Page images set ImageAccept:
	// some image hosts answer a document-first Accept with an HTML page
	Accept string
}

// GetURL returns the request URL
func (r RequestParams) GetURL() string {
	return r.URL
}

// GetReferer returns the request referer
func (r RequestParams) GetReferer() string {
	return r.Referer
}

// userAgent is sent with every request: some sites block Go's default
// "Go-http-client" user agent with a 403/500
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"

// defaultAccept is sent unless RequestParams.Accept overrides it
const defaultAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"

// ImageAccept is the Accept header Chrome sends for an <img> request
const ImageAccept = "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"

// StatusError is returned for a response whose status isn't 200
type StatusError struct {
	Code int
	// RetryAfter is the capped Retry-After delay, zero when the server sent none
	// (retry loops that can wait it out, like downloader.FetchFile, honour it)
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("received %d response code", e.Code)
}

// Retry policy for transient upstream failures. Some sites sit on flaky
// origins that intermittently return Cloudflare 52x errors or 429s on
// otherwise valid requests. Without retries a single such blip fails a whole
// chapter or chapter list fetch.
//
// These are vars rather than consts so tests can shrink the delays.
var (
	maxAttempts    = 4
	retryBaseDelay = 500 * time.Millisecond
	retryMaxDelay  = 8 * time.Second
	// MaxRetryAfter caps the honoured Retry-After delay, so a misbehaving
	// server can't park a download for hours (Cloudflare's 1015 asks for 60s).
	MaxRetryAfter = 60 * time.Second
)

// isRetryableStatus reports whether a response code is worth another try.
// 5xx covers both genuine origin errors and Cloudflare's 52x family (522
// connection timed out, 523 origin unreachable, 524 timeout). 429 is included
// because backing off is exactly the right response to a rate-limit. Every
// 4xx other than 429 is excluded: a 404 for a chapter that doesn't exist must
// fail immediately rather than costing four requests and several seconds.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}

// retryDelay returns the backoff before the given attempt number (1-based),
// growing exponentially from retryBaseDelay up to retryMaxDelay, with up to
// 50% jitter so concurrent page downloads that fail together don't all come
// back at the same instant.
func retryDelay(attempt int) time.Duration {
	d := retryBaseDelay << (attempt - 1)
	if d > retryMaxDelay || d <= 0 {
		d = retryMaxDelay
	}
	return d + time.Duration(rand.Int63n(int64(d/2)+1))
}

// ParseRetryAfter parses a Retry-After header value (seconds integer or HTTP
// date) and returns the indicated delay, capped at MaxRetryAfter. Returns 0
// if the header is absent or unparseable.
func ParseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.ParseFloat(header, 64); err == nil {
		// compared as seconds first: a huge value would overflow Duration
		if math.IsNaN(secs) {
			return 0
		}
		if secs > MaxRetryAfter.Seconds() {
			return MaxRetryAfter
		}
		d = time.Duration(secs * float64(time.Second))
	} else if t, err := http.ParseTime(header); err == nil {
		d = time.Until(t)
	}
	if d <= 0 {
		return 0
	}
	return min(d, MaxRetryAfter)
}

// roundTripper, when set by SetRoundTripper, answers every request instead of
// the network. The demo build uses it to serve recorded fixtures (see demo/).
var roundTripper http.RoundTripper

// SetRoundTripper routes every request through rt instead of the network
func SetRoundTripper(rt http.RoundTripper) {
	roundTripper = rt
}

// request sends a request to the given URL, retrying transient failures with
// exponential backoff. Both GET and POST are retried safely: POST bodies are
// rebuilt from params on each attempt so the body reader is never exhausted.
// Retrying is skipped when RequestParams.DisableRetry is true.
func request(t string, params Params) (io.ReadCloser, error) {
	rp, _ := params.(RequestParams)
	attempts := maxAttempts
	if rp.DisableRetry {
		attempts = 1
	}

	var lastErr error
	var nextDelay time.Duration // overrides exponential backoff when set

	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			if nextDelay > 0 {
				time.Sleep(nextDelay)
				nextDelay = 0
			} else {
				time.Sleep(retryDelay(attempt - 1))
			}
		}

		body, retryAfterDelay, retryable, err := doRequest(t, params)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
		// honour a Retry-After delay if the server sent one; it replaces
		// the exponential delay for the next attempt. This loop can't be
		// cancelled, so it stays within retryMaxDelay (see StatusError.RetryAfter)
		if retryAfterDelay > 0 {
			nextDelay = min(retryAfterDelay, retryMaxDelay)
		}
	}

	// with a single attempt nothing was retried, so don't claim otherwise
	if attempts == 1 {
		return nil, lastErr
	}
	return nil, fmt.Errorf("giving up after %d attempts: %w", attempts, lastErr)
}

// doRequest performs a single attempt and reports whether to retry, plus any
// server-requested delay (Retry-After). The request is built fresh on every
// call so the body reader is never exhausted across retries.
func doRequest(t string, params Params) (body io.ReadCloser, retryAfter time.Duration, retryable bool, err error) {
	tr := roundTripper
	if tr == nil {
		tr = &http.Transport{DisableCompression: true}
	}
	client := &http.Client{Transport: tr}

	rp, _ := params.(RequestParams)

	var reqBody io.Reader
	if rp.Form != nil {
		reqBody = strings.NewReader(rp.Form.Encode())
	} else if rp.Body != "" {
		reqBody = strings.NewReader(rp.Body)
	}

	req, err := http.NewRequest(t, params.GetURL(), reqBody)
	if err != nil {
		// malformed URL: never going to succeed
		return nil, 0, false, err
	}
	req.Header.Set("User-Agent", sessionUserAgent(userAgent))
	if cookies := sessionCookies(req.URL.Hostname()); cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	// some WAFs (e.g. ddos-guard) reject requests missing these browser headers
	accept := rp.Accept
	if accept == "" {
		accept = defaultAccept
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if ref := params.GetReferer(); ref != "" {
		// browsers always send at least the root path in the referer; some
		// image cdns reject referers without it
		if u, parseErr := url.Parse(ref); parseErr == nil && u.Path == "" {
			ref += "/"
		}
		req.Header.Add("Referer", ref)
	}
	if rp.Origin != "" {
		req.Header.Set("Origin", rp.Origin)
	}
	if rp.Form != nil || rp.Body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range rp.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		// transport-level failures (connection reset, timeout, DNS hiccup)
		// are transient enough to be worth another attempt
		return nil, 0, true, err
	}

	// harvest any session cookies the server sets (e.g. a PHPSESSID tied to
	// a CSRF token read off the page), so subsequent requests to the same
	// site stay authenticated without a manual cookie jar
	for _, c := range resp.Cookies() {
		domain := strings.TrimPrefix(c.Domain, ".")
		if domain == "" {
			domain = req.URL.Hostname()
		}
		SetCookie(domain, c.Name, c.Value)
	}

	if resp.StatusCode != 200 {
		// drain a bounded amount before closing so the connection can go
		// back to the pool instead of being torn down on every failure
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()

		ra := ParseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, ra, isRetryableStatus(resp.StatusCode), &StatusError{Code: resp.StatusCode, RetryAfter: ra}
	}

	return resp.Body, 0, false, nil
}
