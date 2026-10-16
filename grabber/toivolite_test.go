// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestToivoLiteChapterNum(t *testing.T) {
	tests := []struct {
		name  string
		title string
		url   string
		want  float64
		ok    bool
	}{
		{
			name:  "whole number from title",
			title: "JOJOLands, Chapter 41",
			url:   "https://w9.jojolandsmanga.com/manga/jojolands-chapter-41/",
			want:  41,
			ok:    true,
		},
		{
			name:  "decimal number from title",
			title: "The Apothecary Diaries, Chapter 63.2",
			url:   "https://w9.the-apothecary-diaries.com/manga/the-apothecary-diaries-chapter-63-2/",
			want:  63.2,
			ok:    true,
		},
		{
			name:  "decimal number from slug fallback (hyphen-separated)",
			title: "Chapter without decimal in title",
			url:   "https://example.com/manga/series-chapter-59-2/",
			want:  59.2,
			ok:    true,
		},
		{
			name:  "whole number from slug fallback",
			title: "no chapter keyword here",
			url:   "https://example.com/manga/series-chapter-12/",
			want:  12,
			ok:    true,
		},
		{
			name:  "no number anywhere",
			title: "Site Announcement",
			url:   "https://example.com/announcement/",
			want:  0,
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toivoLiteChapterNum(tt.title, tt.url)
			if ok != tt.ok {
				t.Errorf("toivoLiteChapterNum(%q, %q) ok = %v, want %v", tt.title, tt.url, ok, tt.ok)
				return
			}
			if ok && got != tt.want {
				t.Errorf("toivoLiteChapterNum(%q, %q) = %v, want %v", tt.title, tt.url, got, tt.want)
			}
		})
	}
}

func TestToivoLiteChapterTitle(t *testing.T) {
	tests := []struct {
		raw, label, siteTitle string
	}{
		{"JOJOLands, Chapter 41", "Chapter 41", ""},
		{"The Apothecary Diaries, Chapter 63.2", "Chapter 63.2", ""},
		// No series prefix
		{"Chapter 7", "Chapter 7", ""},
		{"Chapter 7: Foo", "Chapter 7: Foo", "Chapter 7: Foo"},
		// Lowercase "chapter" keyword in source: the label names it
		{"Series Name, chapter 5", "Chapter 5", ""},
		{"Series Name, Chapter 5: Foo", "Chapter 5: Foo", "Chapter 5: Foo"},
	}
	for _, tt := range tests {
		title, label := toivoLiteChapterTitle(tt.raw)
		c := Chapter{Title: title, Label: label}
		if got := ChapterLabel(c); got != tt.label || c.GetTitle() != tt.siteTitle {
			t.Errorf("toivoLiteChapterTitle(%q): label %q, title %q, want %q, %q", tt.raw, got, c.GetTitle(), tt.label, tt.siteTitle)
		}
	}
}

func TestParseChapterSlugNum(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantNum float64
		wantStr string
		wantOK  bool
	}{
		{
			name:    "whole chapter",
			url:     "https://example.com/manga/series-chapter-41/",
			wantNum: 41,
			wantStr: "41",
			wantOK:  true,
		},
		{
			name:    "decimal chapter (hyphenated)",
			url:     "https://example.com/manga/series-chapter-63-2/",
			wantNum: 63.2,
			wantStr: "63.2",
			wantOK:  true,
		},
		{
			name:   "no chapter in URL",
			url:    "https://example.com/manga/my-series/",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num, numStr, ok := parseChapterSlugNum(tt.url)
			if ok != tt.wantOK {
				t.Errorf("parseChapterSlugNum(%q) ok = %v, want %v", tt.url, ok, tt.wantOK)
				return
			}
			if !ok {
				return
			}
			if num != tt.wantNum {
				t.Errorf("parseChapterSlugNum(%q) num = %v, want %v", tt.url, num, tt.wantNum)
			}
			if numStr != tt.wantStr {
				t.Errorf("parseChapterSlugNum(%q) numStr = %q, want %q", tt.url, numStr, tt.wantStr)
			}
		})
	}
}

// toivoLiteChapterListHTML is a minimal fixture for the Toivo Lite sidebar
// structure. The outer #Chapters_List <li> must NOT appear as a chapter.
const toivoLiteChapterListHTML = `<!doctype html>
<html><body>
<ul>
  <li id="Chapters_List">
    <div class="ceo_latest_comics_widget">
      <ul>
        <li><a href="https://example.com/manga/series-chapter-41/">JOJOLands, Chapter 41</a></li>
        <li><a href="https://example.com/manga/series-chapter-63-2/">Series, Chapter 63.2</a></li>
        <!-- link with no href: must be skipped -->
        <li><a>No href chapter</a></li>
        <!-- relative URL: must be expanded -->
        <li><a href="/manga/series-chapter-7/">Series, Chapter 7</a></li>
        <!-- unparseable number: silently dropped, error collected -->
        <li><a href="https://example.com/announcement/">Site Announcement</a></li>
      </ul>
    </div>
  </li>
</ul>
</body></html>`

func TestToivoLiteChapterList(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(toivoLiteChapterListHTML))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}

	const base = "https://example.com"
	entries, errs := toivoLiteChapterList(doc, base)

	// 3 parseable entries; 1 error for the announcement
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	if len(errs) != 1 {
		t.Fatalf("got %d errs, want 1", len(errs))
	}

	wantTitles := []string{"Chapter 41", "Chapter 63.2", "Chapter 7"}
	wantNums := []float64{41, 63.2, 7}
	for i, e := range entries {
		if got := ChapterLabel(Chapter{Title: e.Title, Number: e.Number, Label: e.Label}); got != wantTitles[i] {
			t.Errorf("entry[%d] label = %q, want %q", i, got, wantTitles[i])
		}
		if e.Number != wantNums[i] {
			t.Errorf("entry[%d].Number = %v, want %v", i, e.Number, wantNums[i])
		}
	}

	// Relative URL must have been expanded
	if !strings.HasPrefix(entries[2].URL, base) {
		t.Errorf("relative URL not expanded: %q", entries[2].URL)
	}
}

// TestToivoLiteChapterListOuterLiScoping checks the outer #Chapters_List <li>
// is not treated as a chapter entry.
func TestToivoLiteChapterListOuterLiScoping(t *testing.T) {
	const html = `<!doctype html><html><body>
<ul>
  <li id="Chapters_List">
    <div class="ceo_latest_comics_widget">
      <ul>
        <li><a href="https://example.com/manga/chapter-1/">Chapter 1</a></li>
      </ul>
    </div>
  </li>
</ul>
</body></html>`

	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	entries, errs := toivoLiteChapterList(doc, "https://example.com")

	if len(entries) != 1 {
		t.Fatalf("got %d entries (want 1); outer <li> may have been counted", len(entries))
	}
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestToivoLiteEntryContentPages(t *testing.T) {
	t.Run("normal chapter", func(t *testing.T) {
		const html = `<!doctype html><html><body>
<div class="entry-content">
  <img src="https://cdn.example.com/page1.jpg">
  <img src="https://cdn.example.com/page2.jpg">
  <img src="">
</div>
</body></html>`
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		pages, err := toivoLiteEntryContentPages(doc, "Chapter 1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pages) != 2 {
			t.Fatalf("got %d pages, want 2", len(pages))
		}
		if pages[0].URL != "https://cdn.example.com/page1.jpg" {
			t.Errorf("pages[0].URL = %q", pages[0].URL)
		}
	})

	t.Run("not available yet placeholder", func(t *testing.T) {
		const html = `<!doctype html><html><body>
<div class="entry-content">
  <p>This chapter is not available yet. Please check back later.</p>
  <img src="https://cdn.example.com/cover.jpg">
</div>
</body></html>`
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		_, err := toivoLiteEntryContentPages(doc, "Chapter 99")
		if err == nil {
			t.Fatal("expected error for 'not available yet', got nil")
		}
		if !strings.Contains(err.Error(), "not available yet") {
			t.Errorf("error %q should mention 'not available yet'", err)
		}
	})

	t.Run("empty chapter returns error", func(t *testing.T) {
		const html = `<!doctype html><html><body>
<div class="entry-content">
  <p>No images here.</p>
</div>
</body></html>`
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		_, err := toivoLiteEntryContentPages(doc, "Chapter 5")
		if err == nil {
			t.Fatal("expected error for empty chapter, got nil")
		}
		if !strings.Contains(err.Error(), "no pages found") {
			t.Errorf("error %q should mention 'no pages found'", err)
		}
	})
}

// TestToivoLiteTest smoke-tests Test() for each domain in toivoLiteSites.
func TestToivoLiteTest(t *testing.T) {
	yes := func(url string) {
		t.Helper()
		got, err := NewToivoLite(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || !got {
			t.Errorf("Test(%q) = %v, %v; want true, nil", url, got, err)
		}
	}
	no := func(url string) {
		t.Helper()
		got, err := NewToivoLite(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || got {
			t.Errorf("Test(%q) = %v, %v; want false, nil", url, got, err)
		}
	}

	// jojolandsmanga.com
	yes("https://jojolandsmanga.com/")
	yes("https://w9.jojolandsmanga.com/manga/jojolands-chapter-41/")
	yes("https://JOJOLANDSMANGA.COM/")
	no("https://myjojolandsmanga.com/")
	no("https://example.com/?site=jojolandsmanga.com")

	// the-apothecary-diaries.com
	yes("https://the-apothecary-diaries.com/")
	yes("https://w9.the-apothecary-diaries.com/manga/the-apothecary-diaries-chapter-63-2/")
	yes("https://THE-APOTHECARY-DIARIES.COM/")
	no("https://mythe-apothecary-diaries.com/")
	no("https://example.com/?url=the-apothecary-diaries.com")

	// unrelated
	no("https://mangadex.org/")
	no("https://heavenly-delusion.com/") // handled by HeavenlyDelusion, not ToivoLite
}
