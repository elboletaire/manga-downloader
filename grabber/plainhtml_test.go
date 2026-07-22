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

// TestFetchChaptersURLFallback covers mangahub.io-style rows where the
// visible text has no "Chapter"/"Volume" keyword (just the manga name and
// number, e.g. "One Piece 1187") but the reader link still contains
// "chapter-<number>", which FetchChapters should parse as a fallback.
func TestFetchChaptersURLFallback(t *testing.T) {
	html := `<html><body><ul>
		<li class="list-group-item"><a href="https://mangahub.io/chapter/one-piece_142/chapter-1188">#1188 - Chapter 1188: Wailing Void</a></li>
		<li class="list-group-item"><a href="https://mangahub.io/chapter/one-piece_142/chapter-1187">#1187 - One Piece 1187</a></li>
		<li class="list-group-item"><a href="https://mangahub.io/chapter/one-piece_142/chapter-1181">#1181 - Ch. 1181</a></li>
		<li class="list-group-item"><a href="https://mangahub.io/announcement">Site news</a></li>
	</ul></body></html>`

	doc := docFromHTML(t, html)
	m := PlainHTML{
		Grabber: &Grabber{URL: "https://mangahub.io/manga/one-piece_142"},
		doc:     doc,
		rows:    doc.Find("li.list-group-item"),
		site: SiteSelector{
			Link: "a",
		},
	}

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3 (announcement row should be skipped)", len(chapters))
	}
	want := []float64{1188, 1187, 1181}
	for i, c := range chapters {
		if got := c.GetNumber(); got != want[i] {
			t.Errorf("chapter %d: got number %v, want %v", i, got, want[i])
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

// TestFetchTitleStripsNoise covers mangahub.io-style h1 headings that nest a
// "Hot" badge and a huge <small> block of alternate-script/language titles,
// which must not end up in the fetched title (it would blow past filesystem
// filename length limits).
func TestFetchTitleStripsNoise(t *testing.T) {
	html := `<html><body><h1 class="_3xnDj">One Piece<a href="/popular" class="label label-hot manga-label">Hot</a><small>ワンピース; Ван Піс</small></h1></body></html>`
	m := PlainHTML{
		doc:  docFromHTML(t, html),
		site: SiteSelector{Title: "h1"},
	}

	got, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "One Piece" {
		t.Errorf("FetchTitle() = %q, want %q", got, "One Piece")
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
