// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

func TestSplitLocked(t *testing.T) {
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 1},
		&grabber.Chapter{Number: 2, Locked: true},
		&grabber.Chapter{Number: 3},
	}
	open, locked := SplitLocked(chapters)
	if len(open) != 2 || open[0].GetNumber() != 1 || open[1].GetNumber() != 3 {
		t.Errorf("open = %v, want chapters 1 and 3", open)
	}
	if len(locked) != 1 || locked[0].GetNumber() != 2 {
		t.Errorf("locked = %v, want chapter 2", locked)
	}
}

func TestLockedMessage(t *testing.T) {
	mk := func(nums ...float64) grabber.Filterables {
		var out grabber.Filterables
		for _, n := range nums {
			out = append(out, &grabber.Chapter{Number: n, Locked: true})
		}
		return out
	}
	tests := []struct {
		locked grabber.Filterables
		want   string
	}{
		{mk(134), "skipping 1 locked chapter: 134"},
		{mk(134, 135, 136), "skipping 3 locked chapters: 134-136"},
		{mk(134, 135, 136, 140, 141.5), "skipping 5 locked chapters: 134-136, 140, 141.5"},
	}
	for _, tt := range tests {
		if got := LockedMessage(tt.locked); got != tt.want {
			t.Errorf("LockedMessage = %q, want %q", got, tt.want)
		}
	}
}
