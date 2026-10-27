// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"testing"
	"time"
)

func TestScrollStops(t *testing.T) {
	for name, tc := range map[string]struct {
		tracked  bool
		atBottom int
		stalled  time.Duration
		want     bool
	}{
		"untracked, settling at the bottom": {false, scrollSettleSteps, time.Hour, false},
		"untracked, settled":                {false, scrollSettleSteps + 1, 0, true},
		"tracked, still growing":            {true, 1000, scrollStall - time.Millisecond, false},
		"tracked, stalled":                  {true, 1, scrollStall, true},
		"tracked, stalled long":             {true, 1, time.Minute, true},
	} {
		if got := scrollStops(tc.tracked, tc.atBottom, tc.stalled); got != tc.want {
			t.Errorf("%s: scrollStops(%v, %d, %v) = %v, want %v", name, tc.tracked, tc.atBottom, tc.stalled, got, tc.want)
		}
	}
}

func TestStallStart(t *testing.T) {
	grew := time.Unix(100, 0)
	now := time.Unix(200, 0)
	for name, tc := range map[string]struct {
		progressGrew, arrived bool
		want                  time.Time
	}{
		"nothing changed":       {false, false, grew},
		"progress grew":         {true, false, now},
		"arrived at the bottom": {false, true, now},
	} {
		if got := stallStart(grew, tc.progressGrew, tc.arrived, now); !got.Equal(tc.want) {
			t.Errorf("%s: stallStart = %v, want %v", name, got, tc.want)
		}
	}
}

func TestParseScrollStep(t *testing.T) {
	for name, tc := range map[string]struct {
		raw     string
		want    scrollStep
		wantErr bool
	}{
		"bottom with a number": {`{"state":"bottom","progress":12}`, scrollStep{"bottom", 12}, false},
		"null progress":        {`{"state":"bottom","progress":null}`, scrollStep{"bottom", 0}, false},
		"missing progress":     {`{"state":"scrolled"}`, scrollStep{"scrolled", 0}, false},
		"done":                 {`{"state":"done","progress":3}`, scrollStep{"done", 3}, false},
		"not an object":        {`"bottom"`, scrollStep{}, true},
	} {
		got, err := parseScrollStep(tc.raw)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", name, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: parseScrollStep = %+v, want %+v", name, got, tc.want)
		}
	}
}
