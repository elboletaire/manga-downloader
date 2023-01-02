package http

import (
	"errors"
	"io"
	"net/http"
)

type Params interface {
	GetURL() string
	GetReferer() string
}

type RequestParams struct {
	URL     string
	Referer string
}

func (r RequestParams) GetURL() string {
	return r.URL
}

func (r RequestParams) GetReferer() string {
	return r.Referer
}

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
		err = errors.New("received non 200 response code")
		return
	}

	body = resp.Body
	return
}
