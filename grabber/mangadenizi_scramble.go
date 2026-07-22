// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"

	"golang.org/x/image/webp"
)

// mangadenizi.net serves reader-page images tile-scrambled ("tiled-v1"): the
// image is split into a grid of tiles whose rows and columns are shuffled
// with a seeded PRNG, and the reader's own JS unshuffles them into a <canvas>
// on load. The scramble method/grid/seed are handed to us in plaintext by the
// JSON API (no secret keys, no per-session state), so it can be reversed
// deterministically without a browser. The algorithm below is a straight
// port of the site's own (minified but unobfuscated) descrambling code found
// in its Nuxt build output.
const (
	// scrambleDefaultSeed is the xorshift32 fallback seed used whenever a
	// derived seed would otherwise be zero (xorshift32 can't recover from a
	// zero state)
	scrambleDefaultSeed uint32 = 2463534242
	// scrambleColumnConst/scrambleRowConst are constants XORed into the page
	// seed to derive independent column/row shuffle seeds
	scrambleColumnConst uint32 = 2246822507
	scrambleRowConst    uint32 = 2654435769
)

// mangadeniziScramble holds the descrambling parameters for a single page,
// as returned verbatim by the reader JSON API
type mangadeniziScramble struct {
	Method string `json:"method"`
	Grid   int    `json:"grid"`
	Seed   uint32 `json:"seed"`
}

// descrambleMangadeniziImage un-shuffles raw image bytes according to the
// page's scramble parameters, re-encoding the result as PNG. Pages with
// method "none" (or empty) are returned unmodified. The image's real
// dimensions are read from its own decoded bounds rather than trusting the
// API's width/height fields, which are sometimes reported as 0 (observed on
// some manga, e.g. one-piece) - scrambling only ever reorders whole tiles,
// never resizes, so the scrambled image's dimensions always equal the
// original's.
func descrambleMangadeniziImage(data []byte, scramble mangadeniziScramble) ([]byte, error) {
	switch scramble.Method {
	case "", "none":
		return data, nil
	case "tiled-v1":
		// handled below
	default:
		return nil, fmt.Errorf("mangadenizi: unsupported scramble method %q", scramble.Method)
	}

	src, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("mangadenizi: decoding scrambled image: %w", err)
	}

	bounds := src.Bounds()
	dst := tiledV1Descramble(src, bounds.Dx(), bounds.Dy(), scramble.Grid, scramble.Seed)

	buf := &bytes.Buffer{}
	if err := png.Encode(buf, dst); err != nil {
		return nil, fmt.Errorf("mangadenizi: encoding descrambled image: %w", err)
	}

	return buf.Bytes(), nil
}

// segment is a [offset, offset+length) span along one image axis
type segment struct {
	offset int
	length int
}

// splitAxis divides total into parts segments as evenly as possible
// (matching the site's own rounding), mirroring its "Wt" function
func splitAxis(total, parts int) []segment {
	if total < 1 {
		total = 1
	}
	if parts < 1 {
		parts = 1
	}
	if parts > total {
		parts = total
	}

	segs := make([]segment, parts)
	for i := 0; i < parts; i++ {
		offset := i * total / parts
		end := (i + 1) * total / parts
		length := end - offset
		if length < 1 {
			length = 1
		}
		segs[i] = segment{offset: offset, length: length}
	}

	return segs
}

// xorshift32 returns a generator function producing the same sequence as the
// site's own xorshift32 PRNG (Marsaglia's algorithm) for the given seed
func xorshift32(seed uint32) func() uint32 {
	x := seed
	if x == 0 {
		x = scrambleDefaultSeed
	}
	return func() uint32 {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		return x
	}
}

// deriveSeed XORs the page seed with a per-axis constant, matching the
// site's own seed derivation (falls back to the default seed on a zero
// result, since xorshift32 can't run from a zero state)
func deriveSeed(seed, k uint32) uint32 {
	n := seed ^ k
	if n == 0 {
		return scrambleDefaultSeed
	}
	return n
}

// fisherYates returns a pseudo-random permutation of [0, n) using the site's
// own xorshift32-driven Fisher-Yates shuffle
func fisherYates(n int, seed uint32) []int {
	if n < 1 {
		n = 1
	}
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}

	next := xorshift32(seed)
	for l := n - 1; l > 0; l-- {
		u := int(next() % uint32(l+1))
		perm[l], perm[u] = perm[u], perm[l]
	}

	return perm
}

// permutedSegments lays out segs[perm[i]]'s lengths back-to-back in
// permutation order, i.e. it computes where each shuffled tile landed in the
// scrambled image, mirroring the site's own "Gt" function
func permutedSegments(segs []segment, perm []int) []segment {
	out := make([]segment, len(perm))
	offset := 0
	for i, s := range perm {
		length := 1
		if s >= 0 && s < len(segs) {
			length = segs[s].length
		}
		out[i] = segment{offset: offset, length: length}
		offset += length
	}
	return out
}

// tiledV1Descramble reverses the "tiled-v1" scramble: src's grid tiles are
// copied back to their original row/column, producing the original image.
// width/height are the *original* (unscrambled) image dimensions, as
// reported by the API - they match src's dimensions exactly, since
// scrambling only ever reorders whole tiles, never resizes them.
func tiledV1Descramble(src image.Image, width, height, grid int, seed uint32) image.Image {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	f := grid
	if f < 1 {
		f = 1
	}
	if f > width {
		f = width
	}
	if f > height {
		f = height
	}

	colSegs := splitAxis(width, f)
	rowSegs := splitAxis(height, f)

	colPerm := fisherYates(f, deriveSeed(seed, scrambleColumnConst))
	rowPerm := fisherYates(f, deriveSeed(seed, scrambleRowConst))

	srcColSegs := permutedSegments(colSegs, colPerm)
	srcRowSegs := permutedSegments(rowSegs, rowPerm)

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for row := 0; row < f; row++ {
		destRow := rowPerm[row]
		destRowSeg := rowSegs[destRow]
		srcRowSeg := srcRowSegs[row]

		for col := 0; col < f; col++ {
			destCol := colPerm[col]
			destColSeg := colSegs[destCol]
			srcColSeg := srcColSegs[col]

			srcRect := image.Rect(
				srcColSeg.offset, srcRowSeg.offset,
				srcColSeg.offset+srcColSeg.length, srcRowSeg.offset+srcRowSeg.length,
			)
			destPt := image.Pt(destColSeg.offset, destRowSeg.offset)

			draw.Draw(dst, image.Rectangle{Min: destPt, Max: destPt.Add(srcRect.Size())}, src, srcRect.Min, draw.Src)
		}
	}

	return dst
}
