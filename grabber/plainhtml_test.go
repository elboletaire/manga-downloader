// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
		{"Bölüm 12", 12, true},         // turkish (eldermanga)
		{"Bolum 3", 3, true},           // turkish, no diacritics
		{"507. Bölüm Manga Oku", 507, true},
		{"375.1. Bölüm", 375.1, true}, // turkish decimal
		{"#124 .Bölüm", 124, true},    // turkish, number first (trmanga)
		{"İlk Bölüm", 0, false},       // turkish, no number
		{"Notice", 0, false},          // announcements have no number
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
// "chapter-<number>", which FetchChapters should parse as a fallback. It also
// covers the <small> "2 weeks ago" relative-date noise mangahub nests inside
// every chapter row, which must not leak into the parsed chapter title.
func TestFetchChaptersURLFallback(t *testing.T) {
	html := `<html><body><ul>
		<li class="list-group-item"><a href="https://mangahub.io/chapter/one-piece_142/chapter-1188">#1188 - Chapter 1188: Wailing Void<small>2 weeks ago</small></a></li>
		<li class="list-group-item"><a href="https://mangahub.io/chapter/one-piece_142/chapter-1187">#1187 - One Piece 1187<small>3 weeks ago</small></a></li>
		<li class="list-group-item"><a href="https://mangahub.io/chapter/one-piece_142/chapter-1181">#1181 - Ch. 1181<small>04-23-2026</small></a></li>
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
	if got := chapters[0].GetTitle(); strings.Contains(got, "ago") {
		t.Errorf("chapter title %q leaked the <small> relative-date noise", got)
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
			name:     "silentquill.net double-escaped RSC pages objects (relative urls)",
			selector: "img",
			html:     `<html><body><script>self.__next_f.push([1,"...{\"pages\":[{\"url\":\"/img/p/1-a\",\"width\":null,\"height\":null},{\"url\":\"/img/p/2-b\",\"width\":null,\"height\":null}],\"x\":1}..."])</script></body></html>`,
			want:     []string{"/img/p/1-a", "/img/p/2-b"},
		},
		{
			name:     "templetoons.com double-escaped RSC array under an obfuscated key",
			selector: "img",
			html:     `<html><head><link rel="canonical" href="https://templetoons.com/comic/bl-antidote/chapter-88"></head><body><script>self.__next_f.push([1,"...{\"gzap5a2\":[\"https://media.templetoons.com/1.jpg\",\"https://media.templetoons.com/2.jpg\"]}..."])</script></body></html>`,
			want:     []string{"https://media.templetoons.com/1.jpg", "https://media.templetoons.com/2.jpg"},
		},
		{
			name:     "arbitrary-key URL array on another Next.js site is not a page list",
			selector: "img",
			html:     `<html><head><link rel="canonical" href="https://a.co/comic/x/chapter-1"></head><body><script>self.__next_f.push([1,"...{\"gzap5a2\":[\"https://media.a.co/1.jpg\",\"https://media.a.co/2.jpg\"]}..."])</script><img src="https://a.co/fallback.jpg"/></body></html>`,
			want:     []string{"https://a.co/fallback.jpg"},
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
			name:     "ritharscans.com Alpine.js JSON.parse blob with absolute paths",
			selector: "img",
			html:     `<html><body><div x-data="immersiveReader({ canRead: true, pages: JSON.parse('[{\u0022path\u0022:\u0022https:\\\/\\\/cdn.a.co\\\/x\\\/001.jpg\u0022,\u0022width\u0022:500},{\u0022path\u0022:\u0022https:\\\/\\\/cdn.a.co\\\/x\\\/002.jpg\u0022,\u0022width\u0022:500}]'), baseLink: 'https:\/\/cdn.a.co\/' })"></div></body></html>`,
			want:     []string{"https://cdn.a.co/x/001.jpg", "https://cdn.a.co/x/002.jpg"},
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
		{
			name:     "uid attribute built from a template-literal script (writerscans)",
			selector: "img.myImage",
			html: `<html><body><div id="pages">` +
				`<img src="/placeholder.svg" uid="abc123" class="myImage"/>` +
				`<img src="/placeholder.svg" uid="def456" class="myImage"/>` +
				`</div><script>let realUrl = ` + "`https://cdn.example.com/uploads/${uid}`" + `;</script></body></html>`,
			want: []string{"https://cdn.example.com/uploads/abc123", "https://cdn.example.com/uploads/def456"},
		},
		{
			name:     "uid attribute built from a JS template literal (mistscans)",
			selector: "#pages img",
			html: `<html><body><div id="pages">` +
				`<img src="/placeholder.svg" uid="1.avif" class="lazy"/>` +
				`<img src="/placeholder.svg" uid="2.avif" class="lazy"/>` +
				`</div><script>let realUrl = ` + "`https://cdn.example.org/uploads/${uid}`" + `;</script></body></html>`,
			want: []string{"https://cdn.example.org/uploads/1.avif", "https://cdn.example.org/uploads/2.avif"},
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

// TestFetchChaptersSkipsLockedRowsAndSanitizesTitles exercises a
// mangastream-style chapter selector: gem-locked chapters use a "#" overlay
// link instead of a real URL and must be skipped by the Rows selector, and
// chapter titles coming from whitespace-heavy markup must be collapsed like
// series titles already are.
func TestFetchChaptersSkipsLockedRowsAndSanitizesTitles(t *testing.T) {
	html := `<html><body><ul id="chapterlist">
		<li data-num="2"><a class="chapter-link-overlay" href="#"></a><span class="chapternum">Chapter    2</span></li>
		<li data-num="1"><a class="chapter-link-overlay" href="https://example.com/manga-chapter-1/"></a><span class="chapternum">Chapter    1</span></li>
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
	if chap.URL != "https://example.com/manga-chapter-1/" {
		t.Errorf("unexpected chapter URL: %q", chap.URL)
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

// PlainHTML.Test must only claim a site whose rows it can number: an entry
// matching rows with no readable chapter number would list nothing, and that
// shadows the later grabbers (tcb) that can. The entry used here is the
// li.wp-manga-chapter one, with Chapter/Link selectors on the <a>.
func TestPlainHTMLTestNeedsReadableNumbers(t *testing.T) {
	cases := []struct {
		name string
		html string
		want bool
	}{
		{
			name: "rows without a readable number are declined",
			html: `<ul><li class="wp-manga-chapter"><a href="https://example.com/manga/berserk/berserk-326/">Berserk 326</a></li></ul>`,
			want: false,
		},
		{
			name: "rows with a number in the text are claimed",
			html: `<ul><li class="wp-manga-chapter"><a href="https://example.com/manga/berserk/berserk-326/">Chapter 326</a></li></ul>`,
			want: true,
		},
		{
			name: "rows numbered from the reader url are claimed",
			html: `<ul><li class="wp-manga-chapter"><a href="https://example.com/manga/berserk/chapter-326/">Berserk</a></li></ul>`,
			want: true,
		},
		{
			name: "a page with no matching rows is declined as before",
			html: `<ul><li>nothing here</li></ul>`,
			want: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "<html><body>"+c.html+"</body></html>")
			}))
			defer srv.Close()

			m := NewPlainHTML(&Grabber{URL: srv.URL + "/manga/berserk/"})
			got, err := m.Test()
			if err != nil {
				t.Fatalf("Test() error = %v", err)
			}
			if got != c.want {
				t.Errorf("Test() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestStripSeriesTitle(t *testing.T) {
	cases := []struct {
		title, series, want string
	}{
		{"One Piece Chapter 5", "One Piece", "Chapter 5"},
		{"one piece - Chapter 5", "One Piece", "Chapter 5"},
		{"One Piece", "One Piece", ""},
		{"Dan Chapter 1", "Dan", "Chapter 1"},           // whole word only
		{"Danger Chapter 1", "Dan", "Danger Chapter 1"}, // not a prefix of a word
		{"Chapter 5", "One Piece", "Chapter 5"},         // no series prefix
		{"Chapter 5", "", "Chapter 5"},                  // unknown series title
	}
	for _, c := range cases {
		if got := stripSeriesTitle(c.title, c.series); got != c.want {
			t.Errorf("stripSeriesTitle(%q, %q) = %q, want %q", c.title, c.series, got, c.want)
		}
	}
}

// TestFetchChaptersTitles covers the title clean-up in FetchChapters: the
// series title is dropped from the chapter title, ChapterTitleStrip removes the
// site's own noise, and a title left with just the number is labelled "Chapter N".
func TestFetchChaptersTitles(t *testing.T) {
	html := `<html><head></head><body>
		<h1 class="title">One Piece</h1>
		<ul id="chapterlist">
			<li><a href="https://example.com/c/1"><span class="n">Chapter 1</span><span class="t">One Piece Chapter 1</span></a></li>
			<li><a href="https://example.com/c/2"><span class="n">Chapter 2.5</span><span class="t">Chapter 2.5</span></a></li>
			<li><a href="https://example.com/c/3"><span class="n">Chapter 3</span><span class="t">#3 Bölüm</span></a></li>
			<li><a href="https://example.com/c/4"><span class="n">Chapter 4</span><span class="t">4</span></a></li>
			<li><a href="https://example.com/c/5"><span class="n">Chapter 5</span><span class="t">  </span></a></li>
			<li><a href="https://example.com/c/6"><span class="n">Announcement</span><span class="t">One Piece</span></a></li>
		</ul></body></html>`
	m := PlainHTML{
		doc: docFromHTML(t, html),
		site: SiteSelector{
			Title:             "h1.title",
			Rows:              "#chapterlist li a",
			Chapter:           "span.n",
			ChapterTitle:      "span.t",
			ChapterTitleStrip: `^#?\d+\s*Bölüm$`,
		},
	}
	m.rows = m.doc.Find(m.site.Rows)

	chapters, errs := m.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	// the announcement row has no chapter number, so it is dropped
	want := []string{"Chapter 1", "Chapter 2.5", "Chapter 3", "Chapter 4", "Chapter 5"}
	if len(chapters) != len(want) {
		t.Fatalf("expected %d chapters, got %d", len(want), len(chapters))
	}
	for i, c := range chapters {
		if got := c.GetTitle(); got != want[i] {
			t.Errorf("chapter %d title = %q, want %q", i, got, want[i])
		}
	}
}

// TestSeriesURLCandidates tests the SeriesLink selector precedence and generic
// candidate extraction (breadcrumbs, back-links, parent paths).
func TestSeriesURLCandidates(t *testing.T) {
	tests := []struct {
		name      string
		pageURL   string
		html      string
		selectors []SiteSelector
		check     func(candidates []string) bool
	}{
		{
			name:    "SeriesLink selector comes first",
			pageURL: "https://example.com/chapter-5/",
			html:    `<html><body><a class="back-link" href="/series/slug/">Back</a><a href="/other/">Other</a></body></html>`,
			selectors: []SiteSelector{
				{SeriesLink: ".back-link"},
			},
			check: func(candidates []string) bool {
				return len(candidates) > 0 && strings.Contains(candidates[0], "/series/slug/")
			},
		},
		{
			name:      "reject listing paths like /manga/",
			pageURL:   "https://example.com/series/slug/chapter-1/",
			html:      `<html><body><a href="/manga/">List</a></body></html>`,
			selectors: []SiteSelector{},
			check: func(candidates []string) bool {
				for _, c := range candidates {
					if strings.Contains(c, "/manga/") && !strings.Contains(c, "/series/") {
						return false
					}
				}
				return true
			},
		},
		{
			name:      "root-relative parent path is a candidate",
			pageURL:   "https://example.com/comics/my-series/chapter/5",
			html:      `<html><body><a href="/comics/my-series">Series</a></body></html>`,
			selectors: []SiteSelector{},
			check: func(candidates []string) bool {
				for _, c := range candidates {
					if strings.Contains(c, "/comics/my-series") {
						return true
					}
				}
				return false
			},
		},
		{
			name:    "SeriesLink from matching Image selector beats parent path ordering",
			pageURL: "https://example.com/chapter-5/",
			html: `<html><body>` +
				`<img class="reader-img" src="page1.jpg"/>` +
				`<a class="back" href="/series/the-slug/">Back</a>` +
				`<a href="/chapter-4/">Prev</a>` +
				`</body></html>`,
			selectors: []SiteSelector{
				{Image: "img.reader-img", SeriesLink: "a.back"},
			},
			check: func(candidates []string) bool {
				return len(candidates) > 0 && strings.Contains(candidates[0], "/series/the-slug/")
			},
		},
		{
			name:    "SeriesLink skipped when Image selector does not match",
			pageURL: "https://example.com/chapter-5/",
			html: `<html><body>` +
				`<a class="back" href="/series/other-series/">Back</a>` +
				`</body></html>`,
			selectors: []SiteSelector{
				// Image selector won't match (no picture elements on the page)
				{Image: "picture img", SeriesLink: "a.back"},
			},
			check: func(candidates []string) bool {
				for _, c := range candidates {
					if strings.Contains(c, "/series/other-series/") {
						return false // SeriesLink should have been skipped
					}
				}
				return true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docFromHTML(t, tt.html)
			candidates := seriesURLCandidates(doc, tt.pageURL, tt.selectors)
			if !tt.check(candidates) {
				t.Errorf("seriesURLCandidates check failed. Got: %v", candidates)
			}
		})
	}
}

// TestIsListingPath verifies that paths like /manga/, /series/, /comics/ are
// rejected as listing pages rather than series pages.
func TestIsListingPath(t *testing.T) {
	tests := []struct {
		url      string
		wantTrue bool
	}{
		{"https://example.com/manga/", true},
		{"https://example.com/series/", true},
		{"https://example.com/comics/", true},
		{"https://example.com/manga/slug/", false},
		{"https://example.com/series/slug/", false},
		{"https://example.com/", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if isListingPath(tt.url) != tt.wantTrue {
				t.Errorf("isListingPath got %v, want %v", !tt.wantTrue, tt.wantTrue)
			}
		})
	}
}

// TestResolveSeriesURLFetchCap verifies that resolveSeriesURL fetches at most
// 3 candidates even when more are available, and returns the first that works.
func TestResolveSeriesURLFetchCap(t *testing.T) {
	// Series page uses the mangapill-style selector so testCandidateSeriesURL
	// can recognise it. The path /manga/the-series/ has two segments, so
	// isListingPath won't filter it out.
	const seriesHTML = `<html><body>
<h1>My Series</h1>
<div id="chapters"><ul data-filter-list="">
  <a href="/chapters/1">Chapter 1</a>
  <a href="/chapters/2">Chapter 2</a>
</ul></div>
</body></html>`

	fetches := 0
	mux := http.NewServeMux()
	// Candidates 1 and 2 return no rows; candidate 3 is the real series page.
	// Candidate 4 must never be fetched.
	for _, path := range []string{"/miss1/", "/miss2/"} {
		p := path
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			fetches++
			_, _ = w.Write([]byte(`<html><body><p>nothing</p></body></html>`))
		})
	}
	mux.HandleFunc("/manga/the-series/", func(w http.ResponseWriter, r *http.Request) {
		fetches++
		_, _ = w.Write([]byte(seriesHTML))
	})
	mux.HandleFunc("/miss4/", func(w http.ResponseWriter, r *http.Request) {
		fetches++
		t.Error("4th candidate fetched; cap should have stopped at 3")
		_, _ = w.Write([]byte(`<html><body></body></html>`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Reader HTML whose breadcrumbs produce four candidates in order.
	// All URLs are absolute so they won't be filtered by isListingPath
	// (single-segment paths like /manga/ would be).
	readerHTML := `<html><body>` +
		`<nav aria-label="Breadcrumb">` +
		`<a href="` + srv.URL + `/miss1/">A</a>` +
		`<a href="` + srv.URL + `/miss2/">B</a>` +
		`<a href="` + srv.URL + `/manga/the-series/">C</a>` +
		`<a href="` + srv.URL + `/miss4/">D</a>` +
		`</nav>` +
		`</body></html>`
	mux.HandleFunc("/chapter-5/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readerHTML))
	})

	m := NewPlainHTML(&Grabber{URL: srv.URL + "/chapter-5/", Settings: &Settings{}})
	ok, err := m.Test()
	if err != nil || ok {
		t.Fatalf("Test() = %v, %v; expected false (resolution deferred to resolveSeriesURL)", ok, err)
	}
	if !m.resolveSeriesURL() {
		t.Fatal("resolveSeriesURL() = false; expected series to be resolved")
	}
	if fetches != 3 {
		t.Errorf("fetched %d candidates, want 3 (cap at 3)", fetches)
	}
}
