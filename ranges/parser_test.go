package ranges

import "testing"

func TestParse(t *testing.T) {
	testCases := []struct {
		name           string
		rawInput       string
		expectedRanges []Range
	}{
		{
			name:           "Single range",
			rawInput:       "1-20",
			expectedRanges: []Range{{Start: 1, End: 20}},
		},
		{
			name:           "Single number",
			rawInput:       "23",
			expectedRanges: []Range{{Start: 23, End: 23}},
		},
		{
			name:     "Multiple ranges",
			rawInput: "1-20,23,55-1059",
			expectedRanges: []Range{
				{Start: 1, End: 20},
				{Start: 23, End: 23},
				{Start: 55, End: 1059},
			},
		},
		{
			name:           "Empty input",
			rawInput:       "",
			expectedRanges: []Range{},
		},
		{
			name:           "Invalid input",
			rawInput:       "1-2-3",
			expectedRanges: []Range{},
		},
		{
			name:           "Invalid range",
			rawInput:       "1-",
			expectedRanges: []Range{},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			ranges := Parse(tt.rawInput)
			if len(ranges) != len(tt.expectedRanges) {
				t.Fatalf("Expected %d ranges, got %d", len(tt.expectedRanges), len(ranges))
			}

			for i, r := range ranges {
				if r.Start != tt.expectedRanges[i].Start || r.End != tt.expectedRanges[i].End {
					t.Fatalf("Expected range %v, got %v", tt.expectedRanges[i], r)
				}
			}
		})
	}
}
