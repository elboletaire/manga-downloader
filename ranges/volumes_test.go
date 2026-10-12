// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ranges

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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
		{"two volumes", "1-7/8-16", [][]Range{{{1, 7}}, {{8, 16}}}, ""},
		{"spaces", " 1-7 / 8-16 ", [][]Range{{{1, 7}}, {{8, 16}}}, ""},
		{"commas within a volume", "1-8,10/9", [][]Range{{{1, 8}, {10, 10}}, {{9, 9}}}, ""},
		{"decimals", "168.1-170/262.5", [][]Range{{{168.1, 170}}, {{262.5, 262.5}}}, ""},
		{"single volume", "1-10", [][]Range{{{1, 10}}}, ""},
		{"empty group keeps the numbering", "1-7//20-30", [][]Range{{{1, 7}}, nil, {{20, 30}}}, ""},
		{"leading empty group", "/5-9", [][]Range{nil, {{5, 9}}}, ""},
		{"trailing slash", "1-7/8-16/", [][]Range{{{1, 7}}, {{8, 16}}}, ""},
		{"overlap within a group is fine", "1-5,3-7/8", [][]Range{{{1, 5}, {3, 7}}, {{8, 8}}}, ""},
		{"empty", "", nil, "no volumes"},
		{"only separators", " / /", nil, "no volumes"},
		{"invalid group", "1-7/abc", nil, "volume 2"},
		{"negative", "1-7/-3", nil, "volume 2"},
		{"overlapping groups", "1-7/7-16", nil, "volumes 1 and 2 overlap (1-7 and 7-16)"},
		{"overlap with a later group", "1-3/5/2.5", nil, "volumes 1 and 3 overlap"},
		{"named volume", "16: 112-114", [][]Range{15: {{112, 114}}}, ""},
		{"named volumes continue", "1-7/5: 30-34/35-40", [][]Range{{{1, 7}}, 4: {{30, 34}}, {{35, 40}}}, ""},
		{"naming the implicit number", "1: 1-7/2: 8", [][]Range{{{1, 7}}, {{8, 8}}}, ""},
		{"named out of order", "3: 10/1: 1-2", [][]Range{{{1, 2}}, nil, {{10, 10}}}, ""},
		{"empty group after a named one", "2: 5//9", [][]Range{nil, {{5, 5}}, nil, {{9, 9}}}, ""},
		{"spaces around the colon", " 4 :  1-3 /5", [][]Range{3: {{1, 3}}, {{5, 5}}}, ""},
		{"no spaces", "2:1-3/4", [][]Range{1: {{1, 3}}, {{4, 4}}}, ""},
		{"decimal chapters after a name", "2: 10.5-12", [][]Range{1: {{10.5, 12}}}, ""},
		{"named twice", "1-7/1: 8-9", nil, "volume 1 is given twice"},
		{"implicit clashes with named", "5: 1/4: 2/3", nil, "volume 5 is given twice"},
		{"decimal volume", "4.5: 1-3", nil, `volume 1: invalid volume number "4.5"`},
		{"zero volume", "1/0: 3", nil, `volume 2: invalid volume number "0", volumes start at 1`},
		{"negative volume", "-2: 3", nil, `invalid volume number "-2"`},
		{"missing volume number", ": 3", nil, `invalid volume number ""`},
		{"text volume number", "v1: 3", nil, `invalid volume number "v1"`},
		{"two colons", "1: 2: 3", nil, `volume 1: invalid range "2: 3"`},
		{"named with no chapters", "1-3/4:", nil, "volume 4: no chapters given"},
		{"huge volume number", "100000: 1", nil, "volume numbers go up to 9999"},
		{"named overlap", "1-7/9: 5", nil, "volumes 1 and 9 overlap (1-7 and 5)"},
		{"bad range in a named group", "3: abc", nil, `volume 3: invalid range "abc"`},
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

// TestParseVolumesShared runs the cases gui/frontend/src/volumes.ts runs too
func TestParseVolumesShared(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "volumes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases struct {
		Parse []struct {
			In      string                  `json:"in"`
			Volumes map[string][][2]float64 `json:"volumes"`
			Error   string                  `json:"error"`
		} `json:"parse"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases.Parse {
		got, err := ParseVolumes(tt.In)
		if tt.Error != "" {
			if err == nil || !strings.Contains(err.Error(), tt.Error) {
				t.Errorf("ParseVolumes(%q) error = %v, want one containing %q", tt.In, err, tt.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVolumes(%q) error = %v", tt.In, err)
			continue
		}
		want := map[string][][2]float64{}
		for i, g := range got {
			for _, r := range g {
				n := strconv.Itoa(i + 1)
				want[n] = append(want[n], [2]float64{r.Begin, r.End})
			}
		}
		if !reflect.DeepEqual(want, tt.Volumes) {
			t.Errorf("ParseVolumes(%q) = %v, want %v", tt.In, want, tt.Volumes)
		}
	}
}
