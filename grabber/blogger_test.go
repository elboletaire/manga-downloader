// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
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
