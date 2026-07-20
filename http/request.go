package http

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// request sends a request to the given URL
func request(t string, params Params) (body io.ReadCloser, err error) {
	tr := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest(t, params.GetURL(), nil)
	req.Header.Set("User-Agent", sessionUserAgent(userAgent))
	if cookies := sessionCookies(req.URL.Hostname()); cookies != "" {
		req.Header.Set("Cookie", cookies)
	}
	// some WAFs (e.g. ddos-guard) reject requests missing these browser headers
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if ref := params.GetReferer(); ref != "" {
		// browsers always send at least the root path in the referer; some
		// image cdns reject referers without it
		if u, err := url.Parse(ref); err == nil && u.Path == "" {
			ref += "/"
		}
		req.Header.Add("Referer", ref)
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("received %d response code", resp.StatusCode)
		return
	}

	body = resp.Body
	return
}
