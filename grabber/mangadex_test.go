// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"slices"
	"sync"
	"testing"
	"time"
)

func TestMangadexChapterURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "chapter URL",
			url:  "https://mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79",
			want: true,
		},
		{
			name: "chapter URL with query",
			url:  "https://mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79?page=5",
			want: true,
		},
		{
			name: "www. prefix (308 to apex)",
			url:  "https://www.mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79",
			want: true,
		},
		{
			name: "uppercase UUID",
			url:  "https://mangadex.org/chapter/C780A907-8CF7-4D64-97FD-750519F8FF79",
			want: true,
		},
		{
			name: "series URL",
			url:  "https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece",
			want: false,
		},
		{
			name: "non-mangadex URL",
			url:  "https://example.com/chapter/123",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mangadexChapterURL(tt.url)
			if got != tt.want {
				t.Errorf("mangadexChapterURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// The site caps the configured concurrency: lower settings are kept, higher
// ones (the defaults included) are clamped to what the image servers take
func TestMangadexGetMaxConcurrency(t *testing.T) {
	cases := []struct {
		set, want MaxConcurrency
	}{
		{MaxConcurrency{Chapters: 5, Pages: 10}, mangadexMaxConcurrency},
		{MaxConcurrency{Chapters: 1, Pages: 2}, MaxConcurrency{Chapters: 1, Pages: 2}},
		{MaxConcurrency{Chapters: 1, Pages: 10}, MaxConcurrency{Chapters: 1, Pages: mangadexMaxConcurrency.Pages}},
	}
	for _, c := range cases {
		m := NewMangadex(&Grabber{Settings: &Settings{MaxConcurrency: c.set}})
		if got := m.GetMaxConcurrency(); got != c.want {
			t.Errorf("GetMaxConcurrency() with %+v = %+v, want %+v", c.set, got, c.want)
		}
		// the settings themselves are left alone
		if m.Settings.MaxConcurrency != c.set {
			t.Errorf("settings changed to %+v", m.Settings.MaxConcurrency)
		}
	}
}

// Mangadex paces its pages, through the interface the downloader looks for
func TestMangadexIsPagePacer(t *testing.T) {
	var site Site = NewMangadex(&Grabber{Settings: &Settings{}})
	if _, ok := site.(PagePacer); !ok {
		t.Fatal("*Mangadex doesn't implement PagePacer")
	}
}

// The pacer spaces its slots one interval apart, even for concurrent callers
func TestPacerSpacesSlots(t *testing.T) {
	p := &pacer{interval: 20 * time.Millisecond}
	const n = 6
	var mu sync.Mutex
	var at []time.Time
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.wait()
			mu.Lock()
			at = append(at, time.Now())
			mu.Unlock()
		}()
	}
	wg.Wait()
	slices.SortFunc(at, func(a, b time.Time) int { return a.Compare(b) })
	// n slots take (n-1) intervals; allow a little scheduling slack
	if span := at[n-1].Sub(at[0]); span < (n-1)*p.interval-5*time.Millisecond {
		t.Errorf("%d slots spanned %v, want at least %v", n, span, (n-1)*p.interval)
	}
}
