// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ranges

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseVolumes(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    [][]Range
		wantErr string
	}{
		{"two volumes", "1-7;8-16", [][]Range{{{1, 7}}, {{8, 16}}}, ""},
		{"spaces", " 1-7 ; 8-16 ", [][]Range{{{1, 7}}, {{8, 16}}}, ""},
		{"commas within a volume", "1-8,10;9", [][]Range{{{1, 8}, {10, 10}}, {{9, 9}}}, ""},
		{"decimals", "168.1-170;262.5", [][]Range{{{168.1, 170}}, {{262.5, 262.5}}}, ""},
		{"single volume", "1-10", [][]Range{{{1, 10}}}, ""},
		{"empty group keeps the numbering", "1-7;;20-30", [][]Range{{{1, 7}}, nil, {{20, 30}}}, ""},
		{"leading empty group", ";5-9", [][]Range{nil, {{5, 9}}}, ""},
		{"trailing semicolon", "1-7;8-16;", [][]Range{{{1, 7}}, {{8, 16}}}, ""},
		{"overlap within a group is fine", "1-5,3-7;8", [][]Range{{{1, 5}, {3, 7}}, {{8, 8}}}, ""},
		{"empty", "", nil, "no volumes"},
		{"only separators", " ; ;", nil, "no volumes"},
		{"invalid group", "1-7;abc", nil, "volume 2"},
		{"negative", "1-7;-3", nil, "volume 2"},
		{"overlapping groups", "1-7;7-16", nil, "volumes 1 and 2 overlap (1-7 and 7-16)"},
		{"overlap with a later group", "1-3;5;2.5", nil, "volumes 1 and 3 overlap"},
	}
	for _, tt := range tests {
		got, err := ParseVolumes(tt.in)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: ParseVolumes(%q) error = %v, want one containing %q", tt.name, tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: ParseVolumes(%q) error = %v", tt.name, tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: ParseVolumes(%q) = %v, want %v", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestRangeString(t *testing.T) {
	for r, want := range map[Range]string{{5, 5}: "5", {1, 7}: "1-7", {10.5, 12}: "10.5-12"} {
		if got := r.String(); got != want {
			t.Errorf("%v.String() = %q, want %q", r, got, want)
		}
	}
}
