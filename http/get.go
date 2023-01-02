package http

import (
	"bytes"
	"io"
)

// Get is a helper method for obtaining online files via GET call
func Get(params Params) (body io.ReadCloser, err error) {
	return request("GET", params)
}

// GetText is a helper method for obtaining online files as string via GET call
func GetText(params Params) (text string, err error) {
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
