package ranges

import (
	"strconv"
	"strings"
)

// Range represents a range of numbers
type Range struct {
	Begin int64
	End   int64
}

// Parse parses a string and returns a slice of ranges
func Parse(rnge string) (rngs []Range, err error) {
	co := strings.Split(rnge, ",")
	var cur int64
	var begin int64
	var end int64

	for _, part := range co {
		in := strings.Split(part, "-")
		if cur, err = strconv.ParseInt(in[0], 10, 64); err != nil {
			return
		}
		if len(in) == 2 {
			begin = cur
			if end, err = strconv.ParseInt(in[1], 10, 64); err != nil {
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
