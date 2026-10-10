// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestSettingsNormalizeClampsLimits(t *testing.T) {
	s := &Settings{
		MaxConcurrency: MaxConcurrency{Chapters: 200, Pages: 42},
		Retry:          9,
	}
	s.Normalize()

	if s.MaxConcurrency.Chapters != 5 || s.MaxConcurrency.Pages != 10 || s.Retry != 3 {
		t.Errorf("got chapters=%d pages=%d retry=%d, want 5/10/3",
			s.MaxConcurrency.Chapters, s.MaxConcurrency.Pages, s.Retry)
	}
}

func TestSettingsNormalizeKeepsValuesInRange(t *testing.T) {
	s := &Settings{
		MaxConcurrency: MaxConcurrency{Chapters: 0, Pages: 10},
		Retry:          0,
	}
	s.Normalize()

	// zero is a valid value (retries disabled) and must not be bumped up
	if s.MaxConcurrency.Chapters != 0 || s.MaxConcurrency.Pages != 10 || s.Retry != 0 {
		t.Errorf("got chapters=%d pages=%d retry=%d, want 0/10/0",
			s.MaxConcurrency.Chapters, s.MaxConcurrency.Pages, s.Retry)
	}
}
