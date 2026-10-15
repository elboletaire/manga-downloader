//go:build demo

package demo

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"image"
	"image/jpeg"
	"io"
	"math"
	stdhttp "net/http"
	"strconv"
	"sync"
	"time"
)

// pageHost is the fake host of every page URL. The transport answers it with
// a placeholder image and refuses everything else, so the demo build can't
// reach the network.
const pageHost = "demo.invalid"

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

// pageURL is the URL of page n (1-based) of the chapter with the given id
func pageURL(chapterID string, n int) string {
	return fmt.Sprintf("https://%s/%s/%03d.jpg", pageHost, chapterID, n)
}

// pages answers page images, each after its pageDelay times scale
type pages struct {
	scale float64
}

// RoundTrip serves the placeholder for a page URL and fails any other request
// naming it
func (p pages) RoundTrip(req *stdhttp.Request) (*stdhttp.Response, error) {
	if req.URL.Host != pageHost {
		return nil, fmt.Errorf("demo: the demo build has no network, refused %s %s", req.Method, req.URL)
	}

	delay := time.Duration(float64(pageDelay(req.URL.String())) * p.scale)
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-time.After(delay):
	}

	body := placeholderJPEG()
	return &stdhttp.Response{
		Status:        "200 OK",
		StatusCode:    stdhttp.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        stdhttp.Header{"Content-Type": {"image/jpeg"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
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
