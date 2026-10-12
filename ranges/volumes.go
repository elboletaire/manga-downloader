// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ranges

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseVolumes parses volume groups separated by semicolons, each in Parse's
// syntax: "1-7;8-16,18" is volume 1 holding chapters 1 to 7, and volume 2
// holding 8 to 16 plus 18. A volume's number is its position, so an empty
// group keeps the count going ("1-7;;20-30" has no volume 2), while trailing
// empty groups are dropped. Groups can't overlap, as a chapter can only go
// into one volume; ranges within the same group can.
func ParseVolumes(s string) ([][]Range, error) {
	segments := strings.Split(s, ";")
	// trailing empty groups (a final ";") number nothing
	for len(segments) > 0 && strings.TrimSpace(segments[len(segments)-1]) == "" {
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 0 {
		return nil, errors.New("no volumes given")
	}

	volumes := make([][]Range, len(segments))
	for i, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		rngs, err := Parse(seg)
		if err != nil {
			return nil, fmt.Errorf("volume %d: %w", i+1, err)
		}
		volumes[i] = rngs
	}

	for i := range volumes {
		for j := i + 1; j < len(volumes); j++ {
			if a, b, ok := overlap(volumes[i], volumes[j]); ok {
				return nil, fmt.Errorf("volumes %d and %d overlap (%s and %s)", i+1, j+1, a, b)
			}
		}
	}

	return volumes, nil
}

// overlap returns the first pair of ranges of a and b sharing a number
func overlap(a, b []Range) (string, string, bool) {
	for _, ra := range a {
		for _, rb := range b {
			if ra.Begin <= rb.End && rb.Begin <= ra.End {
				return ra.String(), rb.String(), true
			}
		}
	}
	return "", "", false
}

// String formats the range in Parse's syntax ("5", "1-7", "10.5-12")
func (r Range) String() string {
	if r.Begin == r.End {
		return formatNumber(r.Begin)
	}
	return formatNumber(r.Begin) + "-" + formatNumber(r.End)
}

func formatNumber(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}
