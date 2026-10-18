// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"
)

// lrsPayloadRe matches the encrypted chapter payload of bloomscans-style
// readers, whose scrambled pages carry their seeds in their 20-hex keys.
var lrsPayloadRe = regexp.MustCompile(`var obs = atob\("([^"]+)"\);\s*var offset = (\d+);\s*var enc = "([^"]+)";`)

// lrsPage is what the reader needs to undo one scrambled page
type lrsPage struct {
	cols, rows    int
	shuffleSeed   uint32
	transformSeed uint32
}

// lrsScrambles returns the scrambled pages of a reader page, keyed by their
// 20-hex key, or nil when the page carries no encrypted payload
func lrsScrambles(html string) map[string]lrsPage {
	m := lrsPayloadRe.FindStringSubmatch(html)
	if m == nil {
		return nil
	}
	obs, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		return nil
	}
	offset, err := strconv.Atoi(m[2])
	if err != nil {
		return nil
	}
	enc, err := base64.StdEncoding.DecodeString(m[3])
	if err != nil || len(enc) == 0 {
		return nil
	}

	key := make([]byte, len(obs))
	for i, c := range obs {
		if int(c) < offset {
			return nil
		}
		key[i] = c - byte(offset)
	}
	if len(key) == 0 {
		return nil
	}
	plain := make([]byte, len(enc))
	for i, c := range enc {
		plain[i] = c ^ key[i%len(key)]
	}

	var payload struct {
		Pages map[string]struct {
			Cols      int  `json:"cols"`
			Rows      int  `json:"rows"`
			Scrambled bool `json:"scrambled"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil
	}

	scrambles := map[string]lrsPage{}
	for k, p := range payload.Pages {
		if !p.Scrambled || len(k) < 16 || p.Cols < 1 || p.Rows < 1 {
			continue
		}
		shuffle, err := strconv.ParseUint(k[:8], 16, 32)
		if err != nil {
			continue
		}
		transform, err := strconv.ParseUint(k[8:16], 16, 32)
		if err != nil {
			continue
		}
		scrambles[k] = lrsPage{
			cols:          p.Cols,
			rows:          p.Rows,
			shuffleSeed:   uint32(shuffle),
			transformSeed: uint32(transform),
		}
	}
	return scrambles
}

// lrsFor returns the scramble of the page whose image URL carries one of the
// payload's keys as a path segment
func lrsFor(scrambles map[string]lrsPage, img string) (lrsPage, bool) {
	u, err := url.Parse(img)
	if err != nil {
		return lrsPage{}, false
	}
	for seg := range strings.SplitSeq(u.Path, "/") {
		if p, ok := scrambles[seg]; ok {
			return p, true
		}
	}
	return lrsPage{}, false
}

// lrsNext advances the reader's LCG and returns its value in [0, 1)
func lrsNext(s *uint32) float64 {
	*s = *s*1664525 + 1013904223
	return float64(*s) / 4294967296
}

// lrsPermutation is the reader's shuffle of the n tiles: slot i ends up at a[i]
func lrsPermutation(n int, seed uint32) []int {
	a := make([]int, n)
	for i := range a {
		a[i] = i
	}
	s := seed
	for i := n - 1; i > 0; i-- {
		j := int(math.Floor(lrsNext(&s) * float64(i+1)))
		a[i], a[j] = a[j], a[i]
	}
	return a
}

// lrsTurns is the reader's per-tile transform: 1 mirrors horizontally, 2
// vertically, 3 both, anything else leaves the tile as it is
func lrsTurns(n int, seed uint32) []int {
	t := make([]int, n)
	s := seed
	for k := range t {
		if r := int(math.Floor(lrsNext(&s) * 6)); r >= 1 && r <= 3 {
			t[k] = r
		}
	}
	return t
}

// lrsUnscramble undoes a page's tile grid and re-encodes it as PNG, which
// keeps the tile edges exact. The grid follows the decoded size, as the
// reader's canvas does.
func lrsUnscramble(data []byte, p lrsPage) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding scrambled image: %w", err)
	}

	bounds := src.Bounds()
	tileW, tileH := bounds.Dx()/p.cols, bounds.Dy()/p.rows
	if tileW < 1 || tileH < 1 {
		return nil, fmt.Errorf("a %dx%d image is too small for %dx%d tiles", bounds.Dx(), bounds.Dy(), p.cols, p.rows)
	}

	in := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(in, in.Bounds(), src, bounds.Min, draw.Src)
	out := image.NewRGBA(in.Bounds())
	draw.Draw(out, out.Bounds(), image.White, image.Point{}, draw.Src)

	n := p.cols * p.rows
	shuffle := lrsPermutation(n, p.shuffleSeed)
	turns := lrsTurns(n, p.transformSeed)
	for k := range n {
		// the reader draws scrambled cell k into cell shuffle[k], mirrored by
		// turns[k]; the mirror is its own inverse, so it can be applied here
		from, to := k, shuffle[k]
		fromX, fromY := (from%p.cols)*tileW, (from/p.cols)*tileH
		toX, toY := (to%p.cols)*tileW, (to/p.cols)*tileH
		mirrorX := turns[k] == 1 || turns[k] == 3
		mirrorY := turns[k] == 2 || turns[k] == 3
		for y := range tileH {
			sy := y
			if mirrorY {
				sy = tileH - 1 - y
			}
			for x := range tileW {
				sx := x
				if mirrorX {
					sx = tileW - 1 - x
				}
				si := in.PixOffset(fromX+sx, fromY+sy)
				di := out.PixOffset(toX+x, toY+y)
				copy(out.Pix[di:di+4], in.Pix[si:si+4])
			}
		}
	}

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, out); err != nil {
		return nil, fmt.Errorf("encoding unscrambled image: %w", err)
	}
	return buf.Bytes(), nil
}

// lrsTransform returns the Transform that undoes a page's scrambling
func lrsTransform(p lrsPage) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		return lrsUnscramble(data, p)
	}
}
