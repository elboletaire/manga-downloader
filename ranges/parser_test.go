package ranges

import "testing"

func TestRangesParsing(t *testing.T) {
	rngs, err := Parse("1-20,23,55-1059")
	if err != nil {
		t.Error(err)
	}
	if len(rngs) != 3 {
		t.Error("Expected 3 ranges")
	}
	if rngs[0].Begin != 1 || rngs[0].End != 20 {
		t.Error("Expected range 1-20")
	}
	if rngs[1].Begin != 23 || rngs[1].End != 23 {
		t.Error("Expected range 23-23")
	}
	if rngs[2].Begin != 55 || rngs[2].End != 1059 {
		t.Error("Expected range 55-1059")
	}
}
