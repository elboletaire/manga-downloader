package http

import (
	"fmt"
	"io"
	"net/http"
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

// request sends a request to the given URL
func request(t string, params Params) (body io.ReadCloser, err error) {
	tr := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest(t, params.GetURL(), nil)
	if params.GetReferer() != "" {
		req.Header.Add("Referer", params.GetReferer())
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
