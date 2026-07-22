// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseChapterNumber(t *testing.T) {
	cases := []struct {
		in     string
		want   float64
		wantOk bool
	}{
		{"Chapter 10", 10, true},
		{"chapter 290", 290, true}, // lowercase (dragontea)
		{"Chapter 10.5", 10.5, true},
		{"Ch. 5", 5, true},
		{"C. 12", 12, true},
		{"Capítulo 1188", 1188, true}, // spanish, accented (zonatmo)
		{"Capitulo 7", 7, true},       // spanish, no accent
		{"Chapitre 42", 42, true},     // french (sushiscan)
		{"  Chapter   3  ", 3, true},  // surrounding/inner whitespace
		{"Volume 18", 18, true},       // volume fallback (sushiscan)
		{"Vol. 2", 2, true},
		{"Vol.2 Chapter 15", 15, true}, // chapter preferred over volume
		{"Notice", 0, false},           // announcements have no number
		{"", 0, false},
	}

	for _, c := range cases {
		got, ok := parseChapterNumber(c.in)
		if ok != c.wantOk || (ok && got != c.want) {
			t.Errorf("parseChapterNumber(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.wantOk)
		}
	}
}

func docFromHTML(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("could not parse html: %v", err)
	}
	return doc
}

func TestGetPlainHTMLImageURL(t *testing.T) {
	cases := []struct {
		name     string
		selector string
		html     string
		want     []string
	}{
		{
			name:     "chapImages javascript variable",
			selector: "img",
			html:     `<html><body><script>var chapImages = 'https://a.co/1.jpg,https://a.co/2.jpg'</script></body></html>`,
			want:     []string{"https://a.co/1.jpg", "https://a.co/2.jpg"},
		},
		{
			name:     "arraydata hidden layer",
			selector: "img",
			html:     `<html><body><div id="arraydata">https://a.co/1.jpg,https://a.co/2.jpg</div></body></html>`,
			want:     []string{"https://a.co/1.jpg", "https://a.co/2.jpg"},
		},
		{
			name:     "ts_reader javascript call (sushiscan)",
			selector: "#readerarea img",
			html:     `<html><body><script>ts_reader.run({"post_id":1,"sources":[{"source":"Server 1","images":["https:\/\/a.co\/1.jpg","https:\/\/a.co\/2.jpg"]}]});</script></body></html>`,
			want:     []string{"https://a.co/1.jpg", "https://a.co/2.jpg"},
		},
		{
			name:     "plain img src",
			selector: "div.reading-content img",
			html:     `<html><body><div class="reading-content"><img src="https://a.co/1.jpg"/><img src="https://a.co/2.jpg"/></div></body></html>`,
			want:     []string{"https://a.co/1.jpg", "https://a.co/2.jpg"},
		},
		{
			name:     "data-src fallback when src is a data uri",
			selector: "div.reading-content img",
			html:     `<html><body><div class="reading-content"><img src="data:image/gif;base64,abc" data-src="https://a.co/1.jpg"/></div></body></html>`,
			want:     []string{"https://a.co/1.jpg"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := getPlainHTMLImageURL(c.selector, docFromHTML(t, c.html))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("getPlainHTMLImageURL() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestFetchChaptersSkipsLockedRowsAndSanitizesTitles exercises the elftoon.com
// selector: gem-locked chapters use a "#" overlay link instead of a real URL
// and must be skipped by the Rows selector, and chapter titles coming from
// whitespace-heavy markup (a real quirk of elftoon's markup) must be
// collapsed like series titles already are.
func TestFetchChaptersSkipsLockedRowsAndSanitizesTitles(t *testing.T) {
	html := `<html><body><ul id="chapterlist">
		<li data-num="2"><a class="chapter-link-overlay" href="#"></a><span class="chapternum">Chapter    2</span></li>
		<li data-num="1"><a class="chapter-link-overlay" href="https://elftoon.com/manga-chapter-1/"></a><span class="chapternum">Chapter    1</span></li>
	</ul></body></html>`

	m := PlainHTML{
		doc: docFromHTML(t, html),
		site: SiteSelector{
			Rows:         `#chapterlist li:has(a.chapter-link-overlay[href^="http"])`,
			Chapter:      ".chapternum",
			ChapterTitle: ".chapternum",
			Link:         "a.chapter-link-overlay",
		},
	}
	m.rows = m.doc.Find(m.site.Rows)

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 1 {
		t.Fatalf("expected only the unlocked chapter to be returned, got %d", len(chapters))
	}

	chap := chapters[0].(*PlainHTMLChapter)
	if chap.GetNumber() != 1 {
		t.Errorf("expected chapter number 1, got %v", chap.GetNumber())
	}
	if chap.GetTitle() != "Chapter 1" {
		t.Errorf("expected sanitized title %q, got %q", "Chapter 1", chap.GetTitle())
	}
	if chap.URL != "https://elftoon.com/manga-chapter-1/" {
		t.Errorf("unexpected chapter URL: %q", chap.URL)
	}
}

func TestSanitizeTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  One   Piece  ", "One Piece"},
		{"One\n\tPiece", "One Piece"},
		{"One Piece", "One Piece"},
	}
	for _, c := range cases {
		if got := sanitizeTitle(c.in); got != c.want {
			t.Errorf("sanitizeTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
