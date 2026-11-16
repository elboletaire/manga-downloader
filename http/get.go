// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"bytes"
	"io"
	"net/http"
)

// Get is a helper method for obtaining online files via GET call
func Get(params Params) (body io.ReadCloser, err error) {
	body, _, err = request("GET", params)
	return
}

// GetWithHeader is Get that also returns the response headers, for a page that
// states something in one (a chapter list's X-Pages count)
func GetWithHeader(params Params) (io.ReadCloser, http.Header, error) {
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
