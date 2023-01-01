package http

import (
	"bytes"
	"errors"
	"io"
	"net/http"
)

// GetParams is a struct for passing parameters to the Get method
type GetParams struct {
	URL     string
	Referer string
}

// Get is a helper method for obtaining online files via GET call
func Get(params GetParams) (body io.ReadCloser, err error) {
	tr := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	req, _ := http.NewRequest("GET", params.URL, nil)
	if params.Referer != "" {
		req.Header.Add("Referer", params.Referer)
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

// GetText is a helper method for obtaining online files as string via GET call
func GetText(params GetParams) (text string, err error) {
	body, err := Get(params)
	if err != nil {
		return
	}
	defer body.Close()

	buff := new(bytes.Buffer)
	io.Copy(buff, body)

	text = buff.String()

	return
}
