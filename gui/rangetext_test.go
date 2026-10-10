// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import "testing"

func TestBundleRange(t *testing.T) {
	tests := []struct {
		name     string
		numbers  []float64
		selected []bool
		want     string
	}{
		{"empty", nil, nil, ""},
		{"none selected", []float64{1, 2}, []bool{false, false}, ""},
		{"single", []float64{1, 2, 3}, []bool{false, true, false}, "2"},
		{"run", []float64{1, 2, 3, 4}, []bool{true, true, true, false}, "1-3"},
		{"gap breaks run", []float64{1, 2, 3, 4, 5}, []bool{true, true, false, true, true}, "1-2,4-5"},
		{"unlisted numbers do not break runs", []float64{1, 2, 4, 5}, []bool{true, true, true, true}, "1-5"},
		{"fractional", []float64{5, 6, 7, 7.5, 8}, []bool{true, false, true, true, false}, "5,7-7.5"},
		{"duplicates in a run", []float64{3, 3, 4}, []bool{true, true, true}, "3-4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bundleRange(tt.numbers, tt.selected); got != tt.want {
				t.Errorf("bundleRange() = %q, want %q", got, tt.want)
			}
		})
	}
}
