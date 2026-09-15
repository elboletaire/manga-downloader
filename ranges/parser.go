// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ranges

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Range represents a range of numbers.
//
// Bounds can be negative, in which case they count from the end of whatever
// the range gets applied to (-1 being the last element). That only means
// something to a caller that knows that size, so only ParseRelative produces
// negative bounds, and only Resolve turns them into positions.
type Range struct {
	Begin float64
	End   float64
}

// rangePart matches a single comma separated part: either one number, or a
// "begin-end" pair. Both bounds can be negative, so the separator can't be
// found by splitting on "-" ("-3--1" is a valid part).
var rangePart = regexp.MustCompile(`^(-?\d+(?:\.\d+)?)(?:-(-?\d+(?:\.\d+)?))?$`)

// Parse parses a string and returns a slice of ranges, i.e. "1-10,12,15-20".
//
// Negative bounds are rejected: Parse filters chapters by their number, where
// counting from the end means nothing (and where a chapter can legitimately be
// numbered 0). Use ParseRelative for the ranges that do support them.
func Parse(rnge string) ([]Range, error) {
	return parse(rnge, false)
}

// ParseRelative parses the same syntax as Parse, additionally accepting
// negative bounds, which count from the end of the set the ranges are applied
// to: -1 is the last element, -2 the second to last, and "-3--1" the last
// three. Resolve turns them into positions once that size is known.
func ParseRelative(rnge string) ([]Range, error) {
	return parse(rnge, true)
}

func parse(rnge string, relative bool) (rngs []Range, err error) {
	for _, part := range strings.Split(rnge, ",") {
		part = strings.TrimSpace(part)

		m := rangePart.FindStringSubmatch(part)
		if m == nil {
			return nil, fmt.Errorf("invalid range %q", part)
		}

		// both bounds already matched the number pattern, so they can't fail
		begin, _ := strconv.ParseFloat(m[1], 64)
		end := begin
		if m[2] != "" {
			end, _ = strconv.ParseFloat(m[2], 64)
		}

		if !relative && (begin < 0 || end < 0) {
			return nil, fmt.Errorf("invalid range %q: negative values are not supported here", part)
		}

		// a reversed range ("10-5") is clamped to its first bound, as it has
		// always been. Only when both bounds sit on the same side of zero,
		// though: "2--1" (from the second element to the last) is perfectly
		// ordered, just not comparably so until Resolve knows the total.
		if sameSign(begin, end) && end < begin {
			end = begin
		}

		rngs = append(rngs, Range{
			Begin: begin,
			End:   end,
		})
	}

	return rngs, nil
}

// Resolve returns the set of 1-based positions the ranges select out of total
// elements, resolving negative bounds against total (-1 being total itself).
// Positions outside 1..total are clamped away, and a range falling entirely
// outside it selects nothing, so an out of range bound is a no-op rather than
// an error. Duplicates and overlaps collapse, the result being a set.
func Resolve(rngs []Range, total int) map[int]bool {
	selected := map[int]bool{}
	if total <= 0 {
		return selected
	}

	for _, r := range rngs {
		// ceil/floor so a fractional bound never selects a position it only
		// partially covers: "1.5-3" is positions 2 and 3
		lo := int(math.Ceil(resolveBound(r.Begin, total)))
		hi := int(math.Floor(resolveBound(r.End, total)))

		// entirely outside, or still reversed once resolved (i.e. "2--1"
		// against a single element)
		if hi < lo || hi < 1 || lo > total {
			continue
		}

		if lo < 1 {
			lo = 1
		}
		if hi > total {
			hi = total
		}

		for i := lo; i <= hi; i++ {
			selected[i] = true
		}
	}

	return selected
}

// resolveBound turns a possibly negative bound into a 1-based position, -1
// being the last element. A zero bound stays zero, which sits outside the
// 1-based positions and so selects nothing on its own.
func resolveBound(b float64, total int) float64 {
	if b < 0 {
		return float64(total) + b + 1
	}

	return b
}

func sameSign(a, b float64) bool {
	return (a < 0) == (b < 0)
}
