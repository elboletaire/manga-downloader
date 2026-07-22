// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"bytes"
	"io"
)

// Post sends a POST request to the given URL
func Post(params Params) (body io.ReadCloser, err error) {
	return request("POST", params)
}

// PostText sends a POST request and returns the response body as a string
func PostText(params Params) (text string, err error) {
	body, err := Post(params)
	if err != nil {
		return
	}
	defer body.Close()

	buff := new(bytes.Buffer)
	io.Copy(buff, body)

	text = buff.String()

	return
}
