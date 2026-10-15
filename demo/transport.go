//go:build demo

// Package demo answers the CLI's HTTP from recorded fixtures, so the VHS tapes
// in assets/demos run offline and show the same output every time. Only the
// demo build (-tags demo, make build/demo) includes it; see README.md.
package demo

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	stdhttp "net/http"
	"os"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

//go:embed fixtures
var fixtureFiles embed.FS

// pageDelay is how long every page image takes to arrive. A fixed delay keeps
// the progress bars moving without letting the runs drift apart.
const pageDelay = 300 * time.Millisecond

// Install routes the CLI's requests through the fixtures, or through the live
// sites while recording (see record.go). It's called from cmd's init.
func Install() {
	if dir := os.Getenv(recordEnv); dir != "" {
		r, err := newRecorder(dir)
		if err != nil {
			panic(err)
		}
		http.SetRoundTripper(r)
		return
	}

	files, err := fs.Sub(fixtureFiles, "fixtures")
	if err != nil {
		panic(err)
	}
	fixtures, err := newFixtures(files)
	if err != nil {
		panic(err)
	}
	http.SetRoundTripper(fixtures)
}

// manifestName is the index of the recorded responses, at the root of the fixtures
const manifestName = "manifest.json"

// entry is one recorded response in the manifest
type entry struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Status int    `json:"status"`
	// Location is the redirect target of a 3xx response
	Location string `json:"location,omitempty"`
	// Image entries have no body: pages are answered with a placeholder
	Image bool `json:"image,omitempty"`
	// File is the body's name under bodies/, empty for images and empty bodies
	File string `json:"file,omitempty"`
}

// key identifies a request, as the manifest indexes it
func key(method, url string) string {
	return method + " " + url
}

// fixtures serves the recorded responses
type fixtures struct {
	files   fs.FS
	entries map[string]entry
}

// newFixtures reads the manifest at the root of files
func newFixtures(files fs.FS) (*fixtures, error) {
	raw, err := fs.ReadFile(files, manifestName)
	if err != nil {
		return nil, err
	}
	var list []entry
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("demo: reading %s: %w", manifestName, err)
	}

	f := &fixtures{files: files, entries: make(map[string]entry, len(list))}
	for _, e := range list {
		f.entries[key(e.Method, e.URL)] = e
	}
	return f, nil
}

// RoundTrip answers a request from the fixtures. A request that was never
// recorded fails with its URL, so a site change or a new tape step shows up as
// a broken run instead of a silent network call.
func (f *fixtures) RoundTrip(req *stdhttp.Request) (*stdhttp.Response, error) {
	e, ok := f.entries[key(req.Method, req.URL.String())]
	if !ok {
		return nil, fmt.Errorf("demo: no fixture for %s %s (record it, see demo/README.md)", req.Method, req.URL)
	}
	if e.Image {
		return placeholder(req)
	}
	var body []byte
	if e.File != "" {
		var err error
		if body, err = fs.ReadFile(f.files, "bodies/"+e.File); err != nil {
			return nil, err
		}
	}
	return response(req, e.Status, e.Location, body), nil
}

// response builds the answer to req; location is set on redirects, so the
// client follows them to their own entries
func response(req *stdhttp.Request, status int, location string, body []byte) *stdhttp.Response {
	resp := &stdhttp.Response{
		Status:        fmt.Sprintf("%d %s", status, stdhttp.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        stdhttp.Header{},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
	if location != "" {
		resp.Header.Set("Location", location)
	}
	return resp
}

// placeholder answers a page image after pageDelay with the placeholder JPEG
func placeholder(req *stdhttp.Request) (*stdhttp.Response, error) {
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-time.After(pageDelay):
	}
	resp := response(req, stdhttp.StatusOK, "", placeholderJPEG())
	resp.Header.Set("Content-Type", "image/jpeg")
	return resp, nil
}

// placeholderJPEG is the one image every page is answered with: a small plain
// JPEG, so the packing and conversion steps still see a real image
var placeholderJPEG = sync.OnceValue(func() []byte {
	img := image.NewGray(image.Rect(0, 0, 240, 360))
	for i := range img.Pix {
		img.Pix[i] = 0xf0
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		panic(err)
	}
	return buf.Bytes()
})
