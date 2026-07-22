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
			name:     "templetoons.com double-escaped RSC pages array",
			selector: "img",
			html:     `<html><body><script>self.__next_f.push([1,"...{\"pages\":[\"https://media.templetoons.com/1.jpg\",\"https://media.templetoons.com/2.jpg\"]}..."])</script></body></html>`,
			want:     []string{"https://media.templetoons.com/1.jpg", "https://media.templetoons.com/2.jpg"},
		},
		{
			name:     "FoOlSlide var pages JSON array (deathtollscans)",
			selector: "img.open",
			html:     `<html><body><script>var pages = [{"id":1,"url":"https:\/\/a.co\/1.png","thumb_url":"https:\/\/a.co\/t1.png"},{"id":2,"url":"https:\/\/a.co\/2.png","thumb_url":"https:\/\/a.co\/t2.png"}]; var next_chapter = "https://a.co/next/";</script></body></html>`,
			want:     []string{"https://a.co/1.png", "https://a.co/2.png"},
		},
		{
			name:     "ritharscans.com Alpine.js x-data immersiveReader blob",
			selector: "img",
			html:     `<html><body><div x-data="immersiveReader({ pages: [{&quot;path&quot;:&quot;series\/webtoon\/abc\/chapters\/def\/001.jpg&quot;,&quot;width&quot;:1080}], baseLink: 'https://a.co/storage/' })"></div></body></html>`,
			want:     []string{"https://a.co/storage/series/webtoon/abc/chapters/def/001.jpg"},
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
		{
			name:     "obfuscated data-src array assignment (mangakatana)",
			selector: "#imgs img",
			html: `<html><body><div id="imgs"><img data-src="#"/><img data-src="#"/></div>` +
				`<script>var ytaw=['https://a.co/1.jpg',];var thzq=['https://a.co/1.jpg','https://a.co/2.jpg',];` +
				`function kxatz(){for(i=thzq.length-1;i>=0;i--){var obj=$('#imgs img:eq('+i+')');obj.attr('data-src', thzq[i]);}}</script>` +
				`</body></html>`,
			want: []string{"https://a.co/1.jpg", "https://a.co/2.jpg"},
		},
		{
			name:     "var pages JSON blob (dynasty-scans)",
			selector: "#reader img",
			html:     `<html><body><div id="reader"><img src="/system/releases/000/1/00.webp"/></div><script>var pages = [{"image":"/system/releases/000/1/00.webp","name":"00","width":1,"height":1},{"image":"/system/releases/000/1/01.webp","name":"01","width":1,"height":1}];</script></body></html>`,
			want:     []string{"/system/releases/000/1/00.webp", "/system/releases/000/1/01.webp"},
		},
		{
			name:     "uid attribute takes priority over an unswapped lazy-load placeholder (asmotoon)",
			selector: "img.myImage",
			html:     `<html><body><img src="/assets/images/placeholder.svg" uid="abc123" class="myImage"/></body></html>`,
			want:     []string{"https://cdn.meowing.org/uploads/abc123"},
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

func TestResolveURL(t *testing.T) {
	cases := []struct {
		name string
		mURL string
		href string
		want string
	}{
		{
			name: "already absolute",
			mURL: "https://tcbonepiecechapters.com/mangas/5/one-piece",
			href: "https://tcbonepiecechapters.com/chapters/1100",
			want: "https://tcbonepiecechapters.com/chapters/1100",
		},
		{
			name: "root-relative (leading slash), matches old BaseUrl()-prefixing behaviour",
			mURL: "https://asurascans.com/comics/foo",
			href: "/comics/foo/chapter/1",
			want: "https://asurascans.com/comics/foo/chapter/1",
		},
		{
			name: "directory-relative, no leading slash (templetoons.com)",
			mURL: "https://templetoons.com/comic/bl-antidote",
			href: "bl-antidote/chapter-88",
			want: "https://templetoons.com/comic/bl-antidote/chapter-88",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := PlainHTML{Grabber: &Grabber{URL: c.mURL}}
			if got := m.resolveURL(c.href); got != c.want {
				t.Errorf("resolveURL(%q) with mURL=%q = %q, want %q", c.href, c.mURL, got, c.want)
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
