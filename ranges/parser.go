package ranges

import (
	"strconv"
	"strings"
)

type Range struct {
	Begin int64
	End   int64
}

type Ranges []Range

func Parse(rnge string) (rngs Ranges, err error) {
	co := strings.Split(rnge, ",")
	var cur int64
	var begin int64
	var end int64

	for _, part := range co {
		in := strings.Split(part, "-")
		cur, err = strconv.ParseInt(in[0], 10, 64)
		if err != nil {
			return
		}
		if len(in) == 2 {
			begin = cur
			end, err = strconv.ParseInt(in[1], 10, 64)
			if err != nil {
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
