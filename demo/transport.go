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
	"hash/fnv"
	"image"
	"image/jpeg"
	"io"
	"io/fs"
	"math"
	stdhttp "net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

//go:embed fixtures
var fixtureFiles embed.FS

// Page delays fall in [minPageDelay, maxPageDelay). They're fixed per URL so
// runs and the light/dark GIF twins stay identical, and varied so pages arrive
// one by one instead of in lockstep.
const (
	minPageDelay = 1200 * time.Millisecond
	maxPageDelay = 4800 * time.Millisecond
)

// delayEnv scales every page delay by a positive factor, so a tape can shorten
// or stretch its downloads. Unset means 1
const delayEnv = "MANGA_DOWNLOADER_DEMO_DELAY"

// pageDelay is how long the page at url takes to arrive: the same URL always
// gets the same delay
func pageDelay(url string) time.Duration {
	h := fnv.New64a()
	h.Write([]byte(url))
	span := uint64(maxPageDelay - minPageDelay)
	return minPageDelay + time.Duration(h.Sum64()%span)
}

// parseDelayScale reads delayEnv's value: empty means 1, anything else must be
// a finite positive number
func parseDelayScale(raw string) (float64, error) {
	if raw == "" {
		return 1, nil
	}
	scale, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		return 0, fmt.Errorf("demo: %s=%q, want a positive number such as 0.4", delayEnv, raw)
	}
	return scale, nil
}

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
	scale, err := parseDelayScale(os.Getenv(delayEnv))
	if err != nil {
		panic(err)
	}
	fixtures, err := newFixtures(files, scale)
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
	// scale multiplies every page delay, see delayEnv
	scale float64
}

// newFixtures reads the manifest at the root of files; scale multiplies the
// page delays
func newFixtures(files fs.FS, scale float64) (*fixtures, error) {
	raw, err := fs.ReadFile(files, manifestName)
	if err != nil {
		return nil, err
	}
	var list []entry
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("demo: reading %s: %w", manifestName, err)
	}

	f := &fixtures{files: files, entries: make(map[string]entry, len(list)), scale: scale}
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
		return placeholder(req, f.scale)
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

// placeholder answers a page image after its pageDelay, scaled by scale, with
// the placeholder JPEG
func placeholder(req *stdhttp.Request, scale float64) (*stdhttp.Response, error) {
	delay := time.Duration(float64(pageDelay(req.URL.String())) * scale)
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-time.After(delay):
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
