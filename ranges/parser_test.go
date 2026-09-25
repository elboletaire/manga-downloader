// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ranges

import (
	"reflect"
	"sort"
	"testing"
)

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

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Range
	}{
		{"single", "5", []Range{{5, 5}}},
		{"chapter zero", "0", []Range{{0, 0}}},
		{"decimals", "10.5-11.5", []Range{{10.5, 11.5}}},
		{"reversed clamps to its first bound", "10-5", []Range{{10, 10}}},
		{"spaces around the parts are ignored", "1-3, 7", []Range{{1, 3}, {7, 7}}},
		{"duplicates are kept as typed", "3,3,1-5", []Range{{3, 3}, {3, 3}, {1, 5}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q): %s", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseRejectsInvalidRanges(t *testing.T) {
	// negatives are meaningless when filtering by chapter number, so Parse
	// must refuse them instead of quietly filtering nothing
	for _, in := range []string{"", "-1", "1--3", "3-", "-", "abc", "1-abc", "1,,3", "1-2-3"} {
		if rngs, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %v, expected an error", in, rngs)
		}
	}
}

func TestParseRelative(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Range
	}{
		{"last element", "-1", []Range{{-1, -1}}},
		{"last three", "-3--1", []Range{{-3, -1}}},
		{"mixed, kept in order", "2--1", []Range{{2, -1}}},
		{"mixed and positive", "2,-1", []Range{{2, 2}, {-1, -1}}},
		{"reversed negatives clamp to their first bound", "-1--3", []Range{{-1, -1}}},
		{"positives still work", "1-10,12", []Range{{1, 10}, {12, 12}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRelative(tt.in)
			if err != nil {
				t.Fatalf("ParseRelative(%q): %s", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseRelative(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseRelativeRejectsInvalidRanges(t *testing.T) {
	for _, in := range []string{"", "-", "--1", "1---2", "-1-", "x-1"} {
		if rngs, err := ParseRelative(in); err == nil {
			t.Errorf("ParseRelative(%q) = %v, expected an error", in, rngs)
		}
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		total int
		want  []int
	}{
		{"a single position", "2", 10, []int{2}},
		{"a positive range", "2,5-7", 10, []int{2, 5, 6, 7}},
		{"the last page", "-1", 10, []int{10}},
		{"the last three pages", "-3--1", 10, []int{8, 9, 10}},
		{"mixed positive and negative", "2,-1", 10, []int{2, 10}},
		{"from a position to the end", "8--1", 10, []int{8, 9, 10}},
		{"negatives resolve per total", "-2", 3, []int{2}},
		{"duplicates and overlaps collapse", "1-3,2,3-4,1", 10, []int{1, 2, 3, 4}},
		{"the whole set can be selected", "1--1", 4, []int{1, 2, 3, 4}},

		// out of range bounds are a no-op, not an error: a chapter simply may
		// not have the page the user asked to skip
		{"a position past the end selects nothing", "99", 10, nil},
		{"a negative past the start selects nothing", "-99", 10, nil},
		{"zero selects nothing", "0", 10, nil},
		{"a range past the end is clamped", "8-99", 10, []int{8, 9, 10}},
		{"a range past the start is clamped", "-99--8", 10, []int{1, 2, 3}},
		{"a mixed range reversed by the total selects nothing", "2--1", 1, nil},
		{"no pages at all selects nothing", "-1", 0, nil},

		// fractional bounds only select the positions they fully cover
		{"fractional bounds", "1.5-3", 10, []int{2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rngs, err := ParseRelative(tt.in)
			if err != nil {
				t.Fatalf("ParseRelative(%q): %s", tt.in, err)
			}

			selected := Resolve(rngs, tt.total)
			got := []int{}
			for i := range selected {
				got = append(got, i)
			}
			sort.Ints(got)

			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Resolve(%q, %d) = %v, want %v", tt.in, tt.total, got, tt.want)
			}
		})
	}
}
