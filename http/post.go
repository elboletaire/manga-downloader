package http

import "io"

// Post sends a POST request to the given URL
func Post(params Params) (body io.ReadCloser, err error) {
	return request("POST", params)
}
