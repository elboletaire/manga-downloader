// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ranges

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// maxVolume bounds an explicit volume number, as volumes are indexed by it
const maxVolume = 9999

// volumePrefix matches a group naming its volume: "16:112-114"
var volumePrefix = regexp.MustCompile(`(?s)^([^:]*):(.*)$`)

// wholeNumber is what a volume number has to be, matching grabber.Chapter's
// integer Volume
var wholeNumber = regexp.MustCompile(`^\d+$`)

// ParseVolumes parses volume groups separated by slashes, each in Parse's
// syntax: "1-7/8-16,18" is volume 1 holding chapters 1 to 7, and volume 2
// holding 8 to 16 plus 18. Volume N is the Nth element of the result.
//
// A group can name its volume with an "N:" prefix ("16:112-114"); one
// without continues from the previous group's number, so "1-7/5:30/31-40" is
// volumes 1, 5 and 6. An empty group keeps the count going ("1-7//20-30" has
// no volume 2), while trailing empty groups are dropped. Spaces around "/"
// and ":" are fine. Neither clashes with Parse, which takes neither. A volume can't
// be given twice, and groups can't overlap, as a chapter can only go into one
// volume; ranges within the same group can.
func ParseVolumes(s string) ([][]Range, error) {
	segments := strings.Split(s, "/")
	// trailing empty groups (a final "/") number nothing
	for len(segments) > 0 && strings.TrimSpace(segments[len(segments)-1]) == "" {
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 0 {
		return nil, errors.New("no volumes given")
	}

	var volumes [][]Range
	number := 0
	for _, seg := range segments {
		number++
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if m := volumePrefix.FindStringSubmatch(seg); m != nil {
			n, err := volumeNumber(strings.TrimSpace(m[1]))
			if err != nil {
				return nil, fmt.Errorf("volume %d: %w", number, err)
			}
			number = n
			seg = strings.TrimSpace(m[2])
			if seg == "" {
				return nil, fmt.Errorf("volume %d: no chapters given", number)
			}
		}
		if number > maxVolume {
			return nil, fmt.Errorf("volume %d: volume numbers go up to %d", number, maxVolume)
		}
		rngs, err := Parse(seg)
		if err != nil {
			return nil, fmt.Errorf("volume %d: %w", number, err)
		}
		for len(volumes) < number {
			volumes = append(volumes, nil)
		}
		if volumes[number-1] != nil {
			return nil, fmt.Errorf("volume %d is given twice", number)
		}
		volumes[number-1] = rngs
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

// volumeNumber parses the N of an "N:" prefix, a whole number from 1
func volumeNumber(s string) (int, error) {
	if !wholeNumber.MatchString(s) {
		return 0, fmt.Errorf("invalid volume number %q, it must be a whole number like 4", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil || n > maxVolume {
		return 0, fmt.Errorf("invalid volume number %q, volume numbers go up to %d", s, maxVolume)
	}
	if n == 0 {
		return 0, errors.New(`invalid volume number "0", volumes start at 1`)
	}
	return n, nil
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
