package ranges

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// Range represents a range of numbers
type Range struct {
	Start int64
	End   int64
}

// String returns a string representation of a range
func (r Range) String() string {
	if r.Start == r.End {
		return strconv.FormatInt(r.Start, 10)
	}

	return fmt.Sprintf("%d-%d", r.Start, r.End)
}

// Parse parses a string and returns a slice of ranges
func Parse(rawRanges string) []Range {
	co := strings.Split(rawRanges, ",")
	var ranges []Range

	for _, part := range co {
		r, err := parseSingleRange(part)
		if err != nil {
			slog.Warn("error parsing range %q: %s", part, err.Error())
			continue
		}

		ranges = append(ranges, r)
	}

	return ranges
}

func parseSingleRange(r string) (Range, error) {
	r = strings.TrimSpace(r)
	rangeSplit := strings.Split(r, "-")
	if len(rangeSplit) == 2 {
		begin, err := strconv.ParseInt(rangeSplit[0], 10, 64)
		if err != nil {
			return Range{}, err
		}

		end, err := strconv.ParseInt(rangeSplit[1], 10, 64)
		if err != nil {
			return Range{}, err
		}

		return Range{Start: begin, End: end}, nil
	}

	// try single number
	num, err := strconv.ParseInt(r, 10, 64)
	if err != nil {
		return Range{}, err
	}

	return Range{Start: num, End: num}, nil
}
