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
			name:  "number before a colon, no chapter word",
			title: "Tokyo Manji Revengers 204: Give Me a Hand",
			url:   "https://example.com/manga/chapter-204-give-me-a-hand/",
			want:  204,
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
		// A number before a colon, without the chapter word: prefixed by the label
		{"Tokyo Manji Revengers 204: Give Me a Hand", "Chapter 204 - Give Me a Hand", "Give Me a Hand"},
		// No number: the title as is, unlabelled
		{"Kaiju no 8 Ex – Kaiju no 8 Manga Online", "Kaiju no 8 Ex – Kaiju no 8 Manga Online", "Kaiju no 8 Ex – Kaiju no 8 Manga Online"},
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
        <!-- no number: kept unnumbered -->
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
	entries := toivoLiteChapterList(doc, base)

	// 3 numbered entries and the unnumbered announcement, none dropped
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}

	wantTitles := []string{"Chapter 41", "Chapter 63.2", "Chapter 7", "Site Announcement"}
	wantNums := []float64{41, 63.2, 7, 0}
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

// toivoLiteUnnumberedListHTML is a trimmed copy of the sidebar of a Toivo Lite
// site whose listing mixes numbered chapters with extras and a numbered post
// that has no "Chapter" word
const toivoLiteUnnumberedListHTML = `<!doctype html>
<html><body>
<div id="Chapters_List"><div class="ceo_latest_comics_widget"><ul>
  <li><a href="https://kaijuno8chapters.com/manga/kaiju-no-8-chapter-26/">Kaiju no 8, Chapter 26</a></li>
  <li><a href="https://kaijuno8chapters.com/manga/kaiju-no-8-ex-kaiju-no-8-manga-online/">Kaiju no 8 Ex &#8211; Kaiju no 8 Manga Online</a></li>
  <li><a href="https://w30.read-tokyorevengers.com/manga/chapter-204-give-me-a-hand/">Tokyo Manji Revengers 204: Give Me a Hand</a></li>
  <li><a href="https://w28.jujutsukaisen-online.com/manga/jujutsu-kaisen-modulo-chapter-25/">Jujutsu Kaisen Modulo &#8211; Chapter 25</a></li>
  <li><a href="https://w30.read-tokyorevengers.com/manga/tokyo-manji-revengers-chapter-12-1-extra-zero/">Tokyo Manji Revengers &#8211; Chapter 12.1 Extra Zero</a></li>
  <li><a href="https://w14.maid-sama.com/manga/chapter-oneshot-special-3/">Chapter Oneshot Special 3</a></li>
</ul></div></div>
</body></html>`

func TestToivoLiteChapterListUnnumbered(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(toivoLiteUnnumberedListHTML))
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}

	entries := toivoLiteChapterList(doc, "https://example.com")
	if len(entries) != 6 {
		t.Fatalf("got %d entries, want 6", len(entries))
	}

	wantLabels := []string{
		"Chapter 26",
		"Kaiju no 8 Ex – Kaiju no 8 Manga Online",
		"Chapter 204 - Give Me a Hand",
		"Chapter 25",
		"Chapter 12.1 Extra Zero",
		"Chapter Oneshot Special 3",
	}
	wantNums := []float64{26, 0, 204, 25, 12.1, 0}
	for i, e := range entries {
		c := Chapter{Title: e.Title, Number: e.Number, Label: e.Label}
		if got := ChapterLabel(c); got != wantLabels[i] {
			t.Errorf("entry[%d] label = %q, want %q", i, got, wantLabels[i])
		}
		if e.Number != wantNums[i] {
			t.Errorf("entry[%d].Number = %v, want %v", i, e.Number, wantNums[i])
		}
		if unnumbered := Unnumbered(&ToivoLiteChapter{Chapter: c}); unnumbered != (wantNums[i] == 0) {
			t.Errorf("entry[%d] Unnumbered = %v, want %v", i, unnumbered, wantNums[i] == 0)
		}
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
	entries := toivoLiteChapterList(doc, "https://example.com")

	if len(entries) != 1 {
		t.Fatalf("got %d entries (want 1); outer <li> may have been counted", len(entries))
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

	t.Run("lazy reader takes the real URL", func(t *testing.T) {
		// a data: src, a protocol-free placeholder GIF src, and a plain src,
		// plus a noscript copy of an image, which must not be counted twice
		const html = `<!doctype html><html><body>
<div class="entry-content">
  <img src="data:image/svg+xml;base64,PHN2Zz4=" data-lazyloaded="1" data-src="https://cdn.example.com/p1.jpg">
  <img src="data:image/svg+xml,%3Csvg%3E" data-lazy-src="https://cdn.example.com/p2.jpg">
  <img class="lazy" src="//w59.example.com/plugins/a3-lazy-load/assets/images/lazy_placeholder.gif" data-src="https://cdn.example.com/p3.png">
  <img src="https://cdn.example.com/p4.jpg">
  <img src="data:image/svg+xml;base64,PHN2Zz4=">
  <noscript><img src="https://cdn.example.com/p3.png"></noscript>
</div>
</body></html>`
		doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
		pages, err := toivoLiteEntryContentPages(doc, "Chapter 2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{
			"https://cdn.example.com/p1.jpg",
			"https://cdn.example.com/p2.jpg",
			"https://cdn.example.com/p3.png",
			"https://cdn.example.com/p4.jpg",
		}
		if len(pages) != len(want) {
			t.Fatalf("got %d pages, want %d: %+v", len(pages), len(want), pages)
		}
		for i, p := range pages {
			if p.URL != want[i] || p.Number != int64(i+1) {
				t.Errorf("pages[%d] = %d %q, want %d %q", i, p.Number, p.URL, i+1, want[i])
			}
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

// TestToivoLiteSameSite checks the image hosts that get the site's Referer:
// the site's own domain and its subdomains, not a CDN
func TestToivoLiteSameSite(t *testing.T) {
	const site = "https://w20.1punchman.com/"
	tests := []struct {
		url  string
		want bool
	}{
		{"https://1punchman.com/wp-content/uploads/a.png", true},
		{"https://w10.1punchman.com/wp-content/uploads/a.png", true},
		{"https://images.mangafreak.me/mangas/onepunch_man/a.jpg", false},
		{"https://blogger.googleusercontent.com/img/b/a.jpg", false},
		{"https://1punchman.com.evil.example/a.png", false},
	}
	for _, tt := range tests {
		if got := toivoLiteSameSite(tt.url, site); got != tt.want {
			t.Errorf("toivoLiteSameSite(%q, %q) = %v, want %v", tt.url, site, got, tt.want)
		}
	}
}
