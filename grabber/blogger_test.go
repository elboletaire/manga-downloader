// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseBloggerTitle(t *testing.T) {
	cases := []struct {
		title  string
		want   bloggerPost
		wantOK bool
	}{
		{"One Piece Manga 1196 [Español]", bloggerPost{Series: "One Piece", Number: 1196}, true},
		{"Kenichi 2 | Manga 27 Español ", bloggerPost{Series: "Kenichi 2", Number: 27}, true},
		{"Fairy Tail 100 Years Quest 222 Español", bloggerPost{Series: "Fairy Tail 100 Years Quest", Number: 222}, true},
		{"Blue Lock Manga 364 Parte 2 [Español]", bloggerPost{Series: "Blue Lock", Number: 364, Part: "2"}, true},
		{"Blue Lock Manga 364 [Español]", bloggerPost{Series: "Blue Lock", Number: 364}, true},
		{"Versus Manga 29.5 Español", bloggerPost{Series: "Versus", Number: 29.5}, true},
		{"Fairy Tail Re: Fantasia 09 Español (Final)", bloggerPost{Series: "Fairy Tail Re: Fantasia", Number: 9}, true},
		{"Jujutsu Kaisen: Módulo Manga 18 Español ", bloggerPost{Series: "Jujutsu Kaisen: Módulo", Number: 18}, true},
		{"Blue Lock  (Capítulo Especial de Nagi y Reo", bloggerPost{}, false},
		{"JUJUTSU KAISEN × JJK MODULO", bloggerPost{}, false},
		{"JAP JAP RAW", bloggerPost{}, false},
		{"Leer Blue Lock Manga 361 Español Online", bloggerPost{Series: "Blue Lock", Number: 361}, true},
		{"Blue Lock Capitulo 356 En Español", bloggerPost{Series: "Blue Lock", Number: 356}, true},
		{"Jujutsu Kaisen, Chapter 260", bloggerPost{Series: "Jujutsu Kaisen", Number: 260}, true},
		{"BLACK CLOVER, CHAPTER 393", bloggerPost{Series: "BLACK CLOVER", Number: 393}, true},
		{"Dragon Ball Super Manga Chapter 92", bloggerPost{Series: "Dragon Ball Super", Number: 92}, true},
	}

	for _, c := range cases {
		got, ok := parseBloggerTitle(c.title)
		if ok != c.wantOK || got != c.want {
			t.Errorf("parseBloggerTitle(%q) = %+v, %v; want %+v, %v", c.title, got, ok, c.want, c.wantOK)
		}
	}
}

func TestBloggerKey(t *testing.T) {
	same := [][2]string{
		{"Jujutsu Kaisen: Módulo", "jujutsu kaisen modulo"},
		{"One Piece", "ONE PIECE"},
		{"Shuumatsu no Valkyrie: Gods’ Apocalypse", "shuumatsu no valkyrie gods apocalypse"},
	}
	for _, c := range same {
		if got := bloggerKey(c[0]); got != bloggerKey(c[1]) {
			t.Errorf("bloggerKey(%q) = %q, want the key of %q (%q)", c[0], got, c[1], bloggerKey(c[1]))
		}
	}
}

func TestBloggerChapters(t *testing.T) {
	entries := []bloggerEntry{
		{"One Piece Manga 1196 [Español]", "https://example.com/2026/10/one-piece-manga-1196-espanol.html"},
		{"One Piece Vol 115 SBS Español ", "https://example.com/2024/01/one-piece-vol-115.html"},
		{"Blue Lock Manga 364 Parte 2 [Español]", "https://example.com/2026/10/blue-lock-364-parte-2.html"},
		{"Blue Lock Manga 364 [Español]", "https://example.com/2026/10/blue-lock-364.html"},
		{"JAP JAP RAW", "https://example.com/2026/10/raw.html"},
	}

	chapters, title := bloggerChapters(entries, "one piece", "es")
	if title != "One Piece" {
		t.Errorf("title = %q, want %q", title, "One Piece")
	}
	if len(chapters) != 1 {
		t.Fatalf("got %d chapters, want 1", len(chapters))
	}
	c := chapters[0].(*BloggerChapter)
	if c.Number != 1196 || ChapterLabel(c) != "Chapter 1196" || c.Language != "es" || c.URL != entries[0].URL {
		t.Errorf("chapter = %+v (%s), want number 1196 in es", c.Chapter, c.URL)
	}

	chapters, _ = bloggerChapters(entries, "Blue Lock", "es")
	if len(chapters) != 2 {
		t.Fatalf("got %d Blue Lock chapters, want 2", len(chapters))
	}
	if part := chapters[0].(*BloggerChapter); ChapterLabel(part) != "Chapter 364 Part 2" || part.Title != "" || part.Number != 364 {
		t.Errorf("part chapter = %q %q (%v), want %q (364)", ChapterLabel(part), part.Title, part.Number, "Chapter 364 Part 2")
	}
}

func TestBloggerResolveSeriesURL(t *testing.T) {
	const post = `<html><body><h3 class='post-title entry-title'>
<a href='https://www.animeallstar1.com/2026/10/kenichi-2-manga-27-espanol.html'>Kenichi 2 | Manga 27 Español</a>
</h3></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(post))
	if err != nil {
		t.Fatal(err)
	}

	b := &Blogger{Grabber: &Grabber{URL: "https://www.animeallstar1.com/2026/10/kenichi-2-manga-27-espanol.html"}, doc: doc}
	if err := b.resolveSeriesURL(); err != nil {
		t.Fatalf("resolveSeriesURL() error = %v", err)
	}
	if b.query != "Kenichi 2" {
		t.Errorf("query = %q, want %q", b.query, "Kenichi 2")
	}

	// a series search is already a series: nothing to resolve
	s := &Blogger{Grabber: &Grabber{URL: "https://www.animeallstar1.com/search?q=One+Piece"}, query: "One Piece"}
	if err := s.resolveSeriesURL(); err != nil || s.query != "One Piece" {
		t.Errorf("resolveSeriesURL() on a series = %q, %v", s.query, err)
	}
}

func TestBloggerPages(t *testing.T) {
	// the markup of a real chapter post, trimmed: the cover and badges carry
	// an alt text, the uploaded pages have alt=""
	const post = `<html><body><div class='post-body entry-content'>
<img alt="[]" height="0" src="https://blogger.googleusercontent.com/img/cover.jpg" width="0" />
<div class="separator"><a href="https://blogger.googleusercontent.com/img/p1.jpg"><img alt="" border="0" src="https://blogger.googleusercontent.com/img/p1.jpg"/></a></div>
<img alt="" src="/img/p2.jpg"/>
<a href="x"><img alt='Descripción de la imagen' src="https://blogger.googleusercontent.com/img/footer.jpg"/></a>
<img alt="" src=""/>
</div></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(post))
	if err != nil {
		t.Fatal(err)
	}
	postURL, _ := url.Parse("https://www.animeallstar1.com/2026/10/one-piece-manga-1196-espanol.html")

	pages := bloggerPages(doc, postURL)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2: %+v", len(pages), pages)
	}
	if pages[0].URL != "https://blogger.googleusercontent.com/img/p1.jpg" || pages[0].Number != 1 {
		t.Errorf("page 1 = %+v", pages[0])
	}
	if pages[1].URL != "https://www.animeallstar1.com/img/p2.jpg" || pages[1].Number != 2 {
		t.Errorf("page 2 = %+v", pages[1])
	}
}

func TestBloggerPagesTextarea(t *testing.T) {
	const post = `<html><body><textarea id='zeist-raw-data' style='display:none;'><div class="separator"><a href="https://x/1.jpg"><img alt="" src="https://x/1.jpg"/></a></div><img alt="" src="https://x/2.jpg"/></textarea></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(post))
	if err != nil {
		t.Fatal(err)
	}
	postURL, _ := url.Parse("https://blog.example/2026/10/a-chapter-1.html")

	pages := bloggerPages(doc, postURL)
	if len(pages) != 2 || pages[1].URL != "https://x/2.jpg" || pages[1].Number != 2 {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestBloggerFingerprint(t *testing.T) {
	cases := map[string]bool{
		`<link href='https://www.blogger.com/static/v1/widgets/4165133002-widget_css_bundle.css' rel='stylesheet'/>`:    true,
		`<script type="text/javascript" src="https://www.blogger.com/static/v1/widgets/950101909-widgets.js"></script>`: true,
		`<link href='https://example.com/style.css' rel='stylesheet'/>`:                                                 false,
	}
	for html, want := range cases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader("<html><head>" + html + "</head></html>"))
		if err != nil {
			t.Fatal(err)
		}
		if got := doc.Find(bloggerFingerprint).Length() > 0; got != want {
			t.Errorf("fingerprint(%s) = %v, want %v", html, got, want)
		}
	}
}

func TestBloggerURLShape(t *testing.T) {
	cases := []struct {
		url   string
		query string
		ok    bool
	}{
		{"https://www.animeallstar1.com/search?q=One+Piece", "One Piece", true},
		{"https://www.animeallstar1.com/2026/10/one-piece-manga-1196-espanol.html", "", true},
		{"https://example.com/2026/10/some-post.html", "", true},
		{"https://www.animeallstar1.com/search", "", false},
		{"https://www.animeallstar1.com/search/label/Nuevo", "", false},
		{"https://www.animeallstar1.com/p/lista-se-series-y-capitulos.html", "", false},
	}
	for _, c := range cases {
		query, ok := bloggerURLShape(c.url)
		if query != c.query || ok != c.ok {
			t.Errorf("bloggerURLShape(%q) = %q, %v; want %q, %v", c.url, query, ok, c.query, c.ok)
		}
	}
}

func TestStripKindWord(t *testing.T) {
	cases := map[string]string{
		"One Piece Manga":            "One Piece",
		"One Piece | Manga":          "One Piece",
		"Kenichi 2: Manhwa":          "Kenichi 2",
		"One Piece":                  "One Piece",
		"Manga":                      "",
		"Chainsaw Man Manga Chapter": "Chainsaw Man",
		"Jujutsu Kaisen, Chapter":    "Jujutsu Kaisen",
	}
	for in, want := range cases {
		if got := stripKindWord(in); got != want {
			t.Errorf("stripKindWord(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBloggerPostTitle(t *testing.T) {
	cases := map[string]string{
		// the h3 heading of most templates
		`<h3 class='post-title'>Blue Lock Manga 364</h3>`: "Blue Lock",
		// a template with an h1 heading and no h3
		`<h1 class='post-title'>Blue Lock Manga 365 Español Online</h1>`: "Blue Lock",
		// no heading: the og:title, whose blog name suffix is dropped
		`<meta content='Blue Lock Manga 366 - mangadelsiglot' property='og:title'/>`: "Blue Lock",
		// no heading or og:title: the <title>, after a blog title that has no number
		`<title>mangadelsiglot</title><title>Blue Lock Manga 367 | Blog</title>`: "Blue Lock",
	}
	for markup, want := range cases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader("<html><head>" + markup + "</head><body></body></html>"))
		if err != nil {
			t.Fatal(err)
		}
		post, ok := bloggerPostTitle(doc)
		if !ok || post.Series != want {
			t.Errorf("bloggerPostTitle(%s) = %+v, %v; want series %q", markup, post, ok, want)
		}
	}
}

func TestStripBlogName(t *testing.T) {
	cases := map[string]string{
		"Blue Lock Manga 366 - mangadelsiglot": "Blue Lock Manga 366",
		"Blue Lock Manga 366 | Blog Name":      "Blue Lock Manga 366",
		"Kenichi 2 | Manga 27 Español":         "Kenichi 2 | Manga 27 Español",
		"Blue Lock Manga 366":                  "Blue Lock Manga 366",
	}
	for in, want := range cases {
		if got := stripBlogName(in); got != want {
			t.Errorf("stripBlogName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBloggerSeriesPost(t *testing.T) {
	const page = `<html><head><meta content='Aldo no Isekai Tensei' property='og:title'/><title>Aldo no Isekai Tensei</title></head></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}

	b := &Blogger{Grabber: &Grabber{URL: "https://www.re-yume.my.id/2026/04/aldo-no-isekai-tensei.html"}, doc: doc}
	if err := b.resolveSeriesURL(); err != nil || b.query != "Aldo no Isekai Tensei" {
		t.Errorf("resolveSeriesURL() = %q, %v", b.query, err)
	}

	tagged, _ := goquery.NewDocumentFromReader(strings.NewReader(`<html><head><meta content='Leer Aldo no Isekai Tensei [Español] - ReYume' property='og:title'/></head></html>`))
	b = &Blogger{Grabber: &Grabber{URL: "https://x.example/2026/04/a.html"}, doc: tagged}
	if err := b.resolveSeriesURL(); err != nil || b.query != "Aldo no Isekai Tensei" {
		t.Errorf("tagged series post = %q, %v", b.query, err)
	}

	chapter, _ := goquery.NewDocumentFromReader(strings.NewReader(`<html><head><meta content='Aldo no Isekai Tensei Chapter 05' property='og:title'/></head></html>`))
	b = &Blogger{Grabber: &Grabber{URL: "https://x.example/2026/04/a.html"}, doc: chapter}
	if err := b.resolveSeriesURL(); err != nil || b.query != "Aldo no Isekai Tensei" {
		t.Errorf("chapter post = %q, %v", b.query, err)
	}

	empty, _ := goquery.NewDocumentFromReader(strings.NewReader(`<html></html>`))
	b = &Blogger{Grabber: &Grabber{URL: "https://x.example/2026/04/a.html"}, doc: empty}
	if err := b.resolveSeriesURL(); err == nil {
		t.Error("a post with no title should not resolve")
	}
}

// bloggerFixture reads a Blogger fixture from testdata/blogger, with the
// placeholder host of its feed links replaced by the test server's.
func bloggerFixture(t *testing.T, name, host string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "blogger", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "https://blog.test", host)
}

// bloggerDoc parses an HTML string into a document for the Blogger helpers.
func bloggerDoc(t *testing.T, markup string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(markup))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestZeistFingerprint(t *testing.T) {
	cases := map[string]bool{
		"* Name    : ZeistManga\n* Version : 5.3":   true,
		"* Name : AniManga\n* Revision : r1":        true,
		"* Name : Shadow Çeviri\n* Version : 5.5.4": false,
		"ZeistManga is a theme name in a credit":    false,
	}
	for markup, want := range cases {
		if got := zeistFingerprint.MatchString(markup); got != want {
			t.Errorf("zeistFingerprint(%q) = %v, want %v", markup, got, want)
		}
	}
}

func TestZeistLabels(t *testing.T) {
	series := bloggerDoc(t, bloggerFixture(t, "zeist_series.html", ""))
	if got := zeistLabels(series, ""); len(got) == 0 || got[0] != "Tonikaku Kawaii" {
		t.Errorf("series labels = %q, want the script's label first", got)
	}

	// the reader's own chapter list names the series; a search query comes first
	reader := bloggerDoc(t, bloggerFixture(t, "zeist_reader_post.html", ""))
	if got := zeistLabels(reader, ""); !slices.Equal(got, []string{"Ogami Tsumiki To Kinichijou"}) {
		t.Errorf("reader labels = %q", got)
	}
	if got := zeistLabels(reader, "Ogami"); len(got) < 2 || got[0] != "Ogami" {
		t.Errorf("labels after a search query = %q", got)
	}

	// a series post with no script label is named by the head of its title
	headed := bloggerDoc(t, `<html><head><meta content='Juujika no Rokunin - Manga Ai-land | جزيرة المانجا' property='og:title'/></head></html>`)
	if got := zeistLabels(headed, ""); len(got) == 0 || got[0] != "Juujika no Rokunin" {
		t.Errorf("headed labels = %q, want Juujika no Rokunin first", got)
	}

	// a data-label on the chapter list names the series over its scripts
	labelled := bloggerDoc(t, `<html><nav id="nextPrevJSbottom" data-label="BE BLUES"></nav><script>clwd.run('Other')</script></html>`)
	if got := zeistLabels(labelled, ""); len(got) == 0 || got[0] != "BE BLUES" {
		t.Errorf("data-label labels = %q, want BE BLUES first", got)
	}
}

func TestZeistChapters(t *testing.T) {
	entries := []bloggerFeedEntry{
		feedEntry("Tonikaku Kawaii", "https://blog.test/series.html", ""),
		feedEntry("Cap 362", "https://blog.test/cap-362.html", "<img src='https://img.test/362.jpg'>"),
		feedEntry("Cap 350.5", "https://blog.test/cap-3505.html", ""),
		feedEntry("Chapter 02", "https://blog.test/chapter-02.html", ""),
		feedEntry("Sin numero", "https://blog.test/none.html", ""),
	}
	chapters := zeistChapters(entries, "Tonikaku Kawaii", "es")
	if len(chapters) != 4 {
		t.Fatalf("got %d chapters, want 4", len(chapters))
	}
	if n := chapters[2].(*BloggerChapter).Number; n != 2 {
		t.Errorf("third chapter number = %v, want 2 (Chapter 02)", n)
	}
	// a post with no number is listed by its title, unnumbered
	if last := chapters[3]; !Unnumbered(last) || ChapterLabel(last) != "Sin numero" {
		t.Errorf("unnumbered post = %q (unnumbered %v), want %q", ChapterLabel(last), Unnumbered(last), "Sin numero")
	}
	first := chapters[0].(*BloggerChapter)
	if first.Number != 362 || ChapterLabel(first) != "Chapter 362" || first.feed != "<img src='https://img.test/362.jpg'>" {
		t.Errorf("first chapter = %+v (%q)", first, first.feed)
	}
	if n := chapters[1].(*BloggerChapter).Number; n != 350.5 {
		t.Errorf("second chapter number = %v, want 350.5", n)
	}

	// a label that is itself a number is a series, not its chapter
	kenichi := zeistChapters([]bloggerFeedEntry{feedEntry("Kenichi 2", "https://blog.test/k.html", "")}, "Kenichi 2", "es")
	if len(kenichi) != 0 {
		t.Errorf("the series post of Kenichi 2 listed as %d chapters", len(kenichi))
	}
}

// feedEntry builds one post of a Blogger JSON feed.
func feedEntry(title, href, content string) bloggerFeedEntry {
	var e bloggerFeedEntry
	e.Title.Value = title
	e.Content.Value = content
	e.Links = []struct {
		Rel  string `json:"rel"`
		Href string `json:"href"`
	}{{Rel: "alternate", Href: href}}
	return e
}

// TestBloggerZeistListing lists a ZeistManga series from its label feed: the
// series post is left out, and the chapter posts are numbered by their titles.
func TestBloggerZeistListing(t *testing.T) {
	series := bloggerFixture(t, "zeist_series.html", "")
	empty := `{"feed":{"openSearch$totalResults":{"$t":"4"},"entry":[]}}`
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2022/04/tonikaku-kawaii.html":
			fmt.Fprint(w, series)
		case "/feeds/posts/default/-/Tonikaku Kawaii":
			if r.URL.Query().Get("start-index") != "1" {
				fmt.Fprint(w, empty)
				return
			}
			fmt.Fprint(w, bloggerFixture(t, "zeist_label_feed.json", srv.URL))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := NewBlogger(&Grabber{URL: srv.URL + "/2022/04/tonikaku-kawaii.html", Settings: &Settings{}})
	if ok, err := b.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want a ZeistManga Blogger", ok, err)
	}
	if title, err := b.FetchTitle(); err != nil || title != "Tonikaku Kawaii" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := b.FetchChapters()
	if len(errs) != 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d chapters, %v; want 3", len(chapters), errs)
	}
	wantURL := srv.URL + "/2026/10/cap-362.html"
	if c := chapters[0].(*BloggerChapter); c.Number != 362 || c.URL != wantURL || c.Language != "es" {
		t.Errorf("first chapter = %+v (%s)", c.Chapter, c.URL)
	}
	if c := chapters[1].(*BloggerChapter); c.Number != 350.5 {
		t.Errorf("second chapter = %v, want 350.5", c.Number)
	}
}

// TestBloggerZeistPagination reads a label feed longer than one page of 150.
func TestBloggerZeistPagination(t *testing.T) {
	const total = 152
	series := bloggerFixture(t, "zeist_series.html", "")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2022/04/tonikaku-kawaii.html":
			fmt.Fprint(w, series)
		case "/feeds/posts/default/-/Tonikaku Kawaii":
			start, _ := strconv.Atoi(r.URL.Query().Get("start-index"))
			var entries []map[string]any
			for i := start; i < start+150 && i <= total; i++ {
				entries = append(entries, map[string]any{
					"title": map[string]any{"$t": fmt.Sprintf("Cap %d", i)},
					"link":  []map[string]any{{"rel": "alternate", "href": fmt.Sprintf("%s/cap-%d.html", srv.URL, i)}},
				})
			}
			json.NewEncoder(w).Encode(map[string]any{"feed": map[string]any{
				"openSearch$totalResults": map[string]any{"$t": strconv.Itoa(total)},
				"entry":                   entries,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := NewBlogger(&Grabber{URL: srv.URL + "/2022/04/tonikaku-kawaii.html", Settings: &Settings{}})
	if ok, err := b.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v", ok, err)
	}
	chapters, errs := b.FetchChapters()
	if len(errs) != 0 || len(chapters) != total {
		t.Fatalf("FetchChapters() = %d chapters, %v; want %d", len(chapters), errs, total)
	}
	if last := chapters[total-1].(*BloggerChapter); last.Number != total {
		t.Errorf("last chapter = %v, want %d", last.Number, total)
	}
}

// TestBloggerZeistPages reads the pages of ZeistManga's reader, named by alt, and
// of a template whose post body is a .check-box, which leaves out the images
// around it.
func TestBloggerZeistPages(t *testing.T) {
	postURL, _ := url.Parse("https://blog.test/2026/09/ogami-tsumiki-to-kinichijou-chapter-114.html")

	pages := bloggerPages(bloggerDoc(t, bloggerFixture(t, "zeist_reader_post.html", "")), postURL)
	if len(pages) != 3 || pages[2].URL != "https://galeri-lepoy.my.id/images/2026/09/22/114_003.jpg" || pages[2].Number != 3 {
		t.Fatalf("reader pages = %+v", pages)
	}

	pages = bloggerPages(bloggerDoc(t, bloggerFixture(t, "blogger_check_box_post.html", "")), postURL)
	if len(pages) != 3 {
		t.Fatalf("check-box pages = %d, want 3: %+v", len(pages), pages)
	}
	for _, p := range pages {
		if strings.Contains(p.URL, "nacionanime") {
			t.Errorf("a decoration outside .check-box is a page: %s", p.URL)
		}
	}
}

// TestBloggerZeistScriptPost lists a pasted chapter post of a script-rendered
// reader: its page is empty, so the pages come from the post's feed entry.
func TestBloggerZeistScriptPost(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2026/09/ogami-tsumiki-to-kinichijou-chapter-114.html":
			fmt.Fprint(w, bloggerFixture(t, "zeist_script_post.html", ""))
		case "/feeds/posts/default/-/Ogami Tsumiki To Kinichijou":
			fmt.Fprint(w, bloggerFixture(t, "zeist_script_feed.json", srv.URL))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// the pasted chapter names its series through its reader's chapter list
	b := NewBlogger(&Grabber{URL: srv.URL + "/2026/09/ogami-tsumiki-to-kinichijou-chapter-114.html", Settings: &Settings{}})
	if ok, err := b.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v", ok, err)
	}
	chapters, errs := b.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v; want 2", len(chapters), errs)
	}

	chapter, err := b.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if len(chapter.Pages) != 3 || chapter.Pages[0].URL != "https://galeri-lepoy.my.id/images/2026/09/22/114_001.jpg" {
		t.Errorf("pages = %+v", chapter.Pages)
	}
}

// TestBloggerPlainListing keeps a plain Blogger site on its search feed: its
// label feed is never asked for.
func TestBloggerPlainListing(t *testing.T) {
	search, err := os.ReadFile(filepath.Join("testdata", "identify", "blogger_search.html"))
	if err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/search":
			w.Write(search)
		case r.URL.Path == "/feeds/posts/summary":
			fmt.Fprint(w, bloggerFixture(t, "blogger_summary_feed.json", srv.URL))
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := NewBlogger(&Grabber{URL: srv.URL + "/search?q=One+Piece", Settings: &Settings{}})
	if ok, err := b.Test(); err != nil || !ok {
		t.Fatalf("Test() = %v, %v", ok, err)
	}
	if b.zeist {
		t.Fatal("a plain Blogger page is not a ZeistManga theme")
	}
	chapters, errs := b.FetchChapters()
	if len(errs) != 0 || len(chapters) != 1 || chapters[0].(*BloggerChapter).Number != 1196 {
		t.Fatalf("FetchChapters() = %d chapters, %v; want One Piece 1196 alone", len(chapters), errs)
	}
}

// TestBloggerZeistFallsBackToSearch lists a ZeistManga chapter post that names no
// label: its series comes from its title, and the search feed lists it.
func TestBloggerZeistFallsBackToSearch(t *testing.T) {
	const post = `<html><head><title>Shangri-la Frontier Chapter 281 - Re-Yume</title>` +
		`<meta content='Shangri-la Frontier Chapter 281 - Re-Yume' property='og:title'/>` +
		`<script src="https://www.blogger.com/static/v1/widgets/950101909-widgets.js"></script>` +
		`<style id='page-skin-1' type='text/css'>* Name : ZeistManga</style></head><body></body></html>`
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2026/09/shangri-la-frontier-chapter-281.html":
			fmt.Fprint(w, post)
		case "/feeds/posts/summary":
			if r.URL.Query().Get("q") != "Shangri-la Frontier" {
				t.Errorf("search query = %q", r.URL.Query().Get("q"))
			}
			fmt.Fprintf(w, `{"feed":{"openSearch$totalResults":{"$t":"1"},"entry":[{"title":{"$t":"Shangri-la Frontier Chapter 281"},"link":[{"rel":"alternate","href":%q}]}]}}`,
				srv.URL+"/2026/09/shangri-la-frontier-chapter-281.html")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := NewBlogger(&Grabber{URL: srv.URL + "/2026/09/shangri-la-frontier-chapter-281.html", Settings: &Settings{}})
	if ok, err := b.Test(); err != nil || !ok || !b.zeist {
		t.Fatalf("Test() = %v, %v; want a ZeistManga Blogger", ok, err)
	}
	chapters, errs := b.FetchChapters()
	if len(errs) != 0 || len(chapters) != 1 || chapters[0].(*BloggerChapter).Number != 281 {
		t.Fatalf("FetchChapters() = %d chapters, %v; want chapter 281", len(chapters), errs)
	}
}

// TestBloggerDataLabelChapterList lists a ZeistManga-like template whose chapter
// list names its label in data-labelchapter and carries no theme header; its
// unnumbered post is kept by its title.
func TestBloggerDataLabelChapterList(t *testing.T) {
	series := bloggerFixture(t, "apenasuma_series.html", "")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/2024/06/eu-quero-ser-um-ator_15.html":
			fmt.Fprint(w, series)
		case "/feeds/posts/default/-/Eu quero ser um Ator":
			fmt.Fprint(w, bloggerFixture(t, "apenasuma_label_feed.json", srv.URL))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := NewBlogger(&Grabber{URL: srv.URL + "/2024/06/eu-quero-ser-um-ator_15.html", Settings: &Settings{}})
	if ok, err := b.Test(); err != nil || !ok || !b.zeist {
		t.Fatalf("Test() = %v, %v, zeist %v; want a label-listed Blogger", ok, err, b.zeist)
	}
	if title, err := b.FetchTitle(); err != nil || title != "Eu quero ser um Ator" {
		t.Errorf("FetchTitle() = %q, %v", title, err)
	}
	chapters, errs := b.FetchChapters()
	if len(errs) != 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters() = %d chapters, %v; want 2", len(chapters), errs)
	}
	if c := chapters[0].(*BloggerChapter); c.Number != 140 {
		t.Errorf("first chapter = %v, want 140", c.Number)
	}
	if last := chapters[1]; !Unnumbered(last) || ChapterLabel(last) != "Epílogo" {
		t.Errorf("unnumbered post = %q, want Epílogo unnumbered", ChapterLabel(last))
	}
}
