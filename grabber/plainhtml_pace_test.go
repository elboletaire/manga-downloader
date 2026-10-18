// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"testing"
	"time"
)

// A host with a pace caps the downloads to its limit, keeping lower settings
func TestPlainHTMLPaceCapsConcurrency(t *testing.T) {
	var site Site = NewPlainHTML(&Grabber{URL: "https://www.bloomscans.com/vamos/", Settings: &Settings{MaxConcurrency: MaxConcurrency{Chapters: 5, Pages: 10}}})
	if _, ok := site.(PagePacer); !ok {
		t.Fatal("*PlainHTML doesn't implement PagePacer")
	}
	want := plainHTMLPaces["bloomscans.com"].max
	if got := site.GetMaxConcurrency(); got != want {
		t.Errorf("GetMaxConcurrency() = %+v, want %+v", got, want)
	}

	lower := NewPlainHTML(&Grabber{URL: "https://bloomscans.com/vamos/", Settings: &Settings{MaxConcurrency: MaxConcurrency{Chapters: 1, Pages: 1}}})
	if got := lower.GetMaxConcurrency(); got != (MaxConcurrency{Chapters: 1, Pages: 1}) {
		t.Errorf("lower settings changed to %+v", got)
	}

	other := NewPlainHTML(&Grabber{URL: "https://example.com/series/", Settings: &Settings{MaxConcurrency: MaxConcurrency{Chapters: 5, Pages: 10}}})
	if got := other.GetMaxConcurrency(); got != (MaxConcurrency{Chapters: 5, Pages: 10}) {
		t.Errorf("a host without a pace got %+v", got)
	}
}

// WaitPage spaces the page requests of a paced host, and does nothing elsewhere
func TestPlainHTMLWaitPagePaces(t *testing.T) {
	saved := plainHTMLPaces["bloomscans.com"]
	defer func() { plainHTMLPaces["bloomscans.com"] = saved }()
	plainHTMLPaces["bloomscans.com"] = plainHTMLPace{max: saved.max, pacer: &pacer{interval: 20 * time.Millisecond}}

	paced := NewPlainHTML(&Grabber{URL: "https://bloomscans.com/vamos/", Settings: &Settings{}})
	start := time.Now()
	for range 3 {
		paced.WaitPage()
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Errorf("3 page waits took %v, want at least 2 intervals", elapsed)
	}

	free := NewPlainHTML(&Grabber{URL: "https://example.com/series/", Settings: &Settings{}})
	start = time.Now()
	for range 3 {
		free.WaitPage()
	}
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Errorf("an unpaced host waited %v", elapsed)
	}
}
