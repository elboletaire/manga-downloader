package ranges

import (
	"os"
	"strconv"
	"strings"
)

// Range represents a range of numbers
type Range struct {
	Begin float64
	End   float64
}

// Parse parses a string and returns a slice of ranges
func Parse(rnge string) (rngs []Range, err error) {
	co := strings.Split(rnge, ",")
	var cur float64
	var begin float64
	var end float64

	for _, part := range co {
		in := strings.Split(part, "-")
		if cur, err = strconv.ParseFloat(in[0], 64); err != nil {
			return
		}
		if len(in) == 2 {
			begin = cur
			if end, err = strconv.ParseFloat(in[1], 64); err != nil {
				return
			}
		}

		if begin == 0 {
			begin = cur
		}
		if end < cur {
			end = cur
		}
		if begin > cur {
			continue
		}

		rngs = append(rngs, Range{
			Begin: begin,
			End:   end,
		})

		begin = 0
		end = 0
	}

	return
}

// ParseVolumes parses a semicolon-delimited string of chapter ranges into
// a slice of range-slices, where each inner slice represents one volume.
// Commas within a segment work the same as in Parse (e.g. "1-8,10" is valid).
func ParseVolumes(s string) ([][]Range, error) {
	segments := strings.Split(s, ";")
	volumes := make([][]Range, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		rngs, err := Parse(seg)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, rngs)
	}
	return volumes, nil
}

// ParseVolumesFile reads a plain-text file where each non-blank, non-comment
// line is a chapter range (parsed by Parse). Lines starting with "#" are
// treated as comments and skipped. Each line becomes one volume.
func ParseVolumesFile(path string) ([][]Range, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	volumes := make([][]Range, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rngs, err := Parse(line)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, rngs)
	}
	return volumes, nil
}
