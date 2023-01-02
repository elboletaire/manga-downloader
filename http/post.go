package http

import "io"

func Post(params Params) (body io.ReadCloser, err error) {
	return request("POST", params)
}
