// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestWordPressCategoryChapters(t *testing.T) {
	var posts []wpPost
	err := json.Unmarshal([]byte(`[
		{"link":"https://site.test/one-piece-manga-1196-espanol/","title":{"rendered":"One Piece Manga 1196 Espa&ntilde;ol"}},
		{"link":"https://site.test/one-piece-1184/","title":{"rendered":"One Piece 1184"}},
		{"link":"https://site.test/anuncio/","title":{"rendered":"Aviso a los lectores"}}
	]`), &posts)
	if err != nil {
		t.Fatal(err)
	}

	chapters := wpChapters(posts, "es")
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2 (the announcement has no number)", len(chapters))
	}

	want := []struct {
		title string
		num   float64
		url   string
	}{
		{"Chapter 1196", 1196, "https://site.test/one-piece-manga-1196-espanol/"},
		{"Chapter 1184", 1184, "https://site.test/one-piece-1184/"},
	}
	for i, w := range want {
		c := chapters[i].(*WordPressCategoryChapter)
		if c.Title != w.title || c.Number != w.num || c.URL != w.url || c.Language != "es" {
			t.Errorf("chapter %d = %+v, want %+v", i, c, w)
		}
	}
}

func TestWordPressChapterNumber(t *testing.T) {
	cases := []struct {
		title string
		num   float64
		part  string
		ok    bool
	}{
		{"Gachiakuta 172 parte 2", 172, "2", true},
		{"Gachiakuta 172", 172, "", true},
		{"Gachiakuta 140.5", 140.5, "", true},
		{"One Piece Manga 1196 Español", 1196, "", true},
		{"Jujutsu Kaisen Chapter 200", 200, "", true},
		{"BLACK CLOVER, CHAPTER 388", 388, "", true},
		{"Aviso a los lectores", 0, "", false},
	}
	for _, c := range cases {
		num, part, ok := wpChapterNumber(c.title)
		if num != c.num || part != c.part || ok != c.ok {
			t.Errorf("wpChapterNumber(%q) = %v, %q, %v; want %v, %q, %v", c.title, num, part, ok, c.num, c.part, c.ok)
		}
	}
}

func TestWordPressCategoryPartChapters(t *testing.T) {
	var posts []wpPost
	err := json.Unmarshal([]byte(`[
		{"link":"https://site.test/gachiakuta-172-parte-2/","title":{"rendered":"Gachiakuta 172 parte 2"}},
		{"link":"https://site.test/gachiakuta-172/","title":{"rendered":"Gachiakuta 172"}}
	]`), &posts)
	if err != nil {
		t.Fatal(err)
	}

	chapters := wpChapters(posts, "")
	want := []string{"Chapter 172 Part 2", "Chapter 172"}
	if len(chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		if c := chapters[i].GetTitle(); c != w {
			t.Errorf("chapter %d title = %q, want %q", i, c, w)
		}
	}
}

func TestWordPressCategoryHiddenImages(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div class="entry-content">
			<img class="alignleft" alt="" src="https://cdn.test/cover.jpeg" width="0" height="0">
			<img src="https://cdn.test/hidden.jpg" width="0px" height="5">
			<img src="https://cdn.test/01.jpg" width="727" height="1062">
		</div>`))
	if err != nil {
		t.Fatal(err)
	}

	pages := wpPageImages(doc)
	if len(pages) != 1 || pages[0].URL != "https://cdn.test/01.jpg" {
		t.Fatalf("got %+v, want only 01.jpg", pages)
	}
}

func TestWordPressCategoryTestKeepsLanguage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html lang="en-US"><head><link rel="alternate" href="/wp-json/wp/v2/categories/3" /></head>
			<body class="archive category category-3"></body></html>`)
	}))
	defer srv.Close()

	w := NewWordPressCategory(&Grabber{URL: srv.URL + "/category/x/"})
	ok, err := w.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want true, nil", ok, err)
	}
	if w.language != "en" {
		t.Errorf("language = %q, want %q", w.language, "en")
	}
}

func TestWordPressCategoryPageImages(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div class="entry-content">
			<img src="data:image/png;base64,AAAA" data-src="https://cdn.test/01.jpg">
			<img src="https://cdn.test/02.jpg">
			<img src="https://cdn.test/02.jpg">
			<img src="data:image/png;base64,BBBB" data-src="https://cdn.test/03.webp">
		</div>
		<img src="https://cdn.test/logo.png">`))
	if err != nil {
		t.Fatal(err)
	}

	pages := wpPageImages(doc)
	want := []string{"https://cdn.test/01.jpg", "https://cdn.test/02.jpg", "https://cdn.test/03.webp"}
	if len(pages) != len(want) {
		t.Fatalf("got %d pages, want %d: %+v", len(pages), len(want), pages)
	}
	for i, p := range pages {
		if p.URL != want[i] || p.Number != int64(i+1) {
			t.Errorf("page %d = %+v, want URL %s", i, p, want[i])
		}
	}
}

func TestWordPressCategoryFromPage(t *testing.T) {
	cases := []struct {
		name string
		html string
		want int
	}{
		{
			"category archive",
			`<html><head><link rel="alternate" href="https://site.test/wp-json/wp/v2/categories/3" /></head>
			<body class="archive category category-3"></body></html>`,
			3,
		},
		{
			// the REST link alone isn't enough: a tag archive or a page
			// linking a category resource must not be taken for a series
			"category link outside an archive",
			`<html><head><link rel="alternate" href="https://site.test/wp-json/wp/v2/categories/3" /></head>
			<body class="page"></body></html>`,
			0,
		},
		{
			"unrelated page",
			`<html><body class="home"><a href="/category/x/">x</a></body></html>`,
			0,
		},
	}

	for _, c := range cases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(c.html))
		if err != nil {
			t.Fatal(err)
		}
		w := NewWordPressCategory(&Grabber{URL: "https://site.test/category/x/"})
		got, err := w.categoryFromPage(doc)
		if err != nil {
			t.Errorf("%s: error %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: categoryFromPage() = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestWordPressCategoryRESTFailureIsNotThisSite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/wp-json/wp/v2/posts/404") {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "<html>not json</html>")
	}))
	defer srv.Close()

	cases := map[string]string{
		"404 from the REST route": `<html><head><link rel="alternate" href="` + srv.URL + `/wp-json/wp/v2/posts/404" /></head><body class="single"></body></html>`,
		"body isn't JSON":         `<html><head><link rel="alternate" href="` + srv.URL + `/wp-json/wp/v2/posts/5" /></head><body class="single"></body></html>`,
	}
	for name, page := range cases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
		if err != nil {
			t.Fatal(err)
		}
		w := NewWordPressCategory(&Grabber{URL: srv.URL + "/post/"})
		got, err := w.categoryFromPage(doc)
		if err != nil || got != 0 {
			t.Errorf("%s: categoryFromPage() = %d, %v; want 0, nil", name, got, err)
		}
	}
}

func TestWordPressPostsSeries(t *testing.T) {
	posts := func(titles ...string) []wpPost {
		var ps []wpPost
		for _, t := range titles {
			ps = append(ps, wpPost{Title: struct {
				Rendered string `json:"rendered"`
			}{t}})
		}
		return ps
	}

	cases := []struct {
		name   string
		titles []string
		want   string
		ok     bool
	}{
		{"generic category", []string{"Jujutsu Kaisen, Chapter 260", "Jujutsu Kaisen, Chapter 259", "Garry K. Johnson in Jujutsu Kaisen"}, "Jujutsu Kaisen", true},
		{"entities and case", []string{"BLACK CLOVER, CHAPTER 393", "BLACK CLOVER, TABATA&#8217;S AUTOBIOGRAPHY", "Black Clover Manga 392"}, "BLACK CLOVER", true},
		{"majority wins over a stray post", []string{"One Piece Manga 1196 Español", "One Piece 1184", "Kenichi 2 Manga 27"}, "One Piece", true},
		{"no majority", []string{"One Piece 1196", "Kenichi 2 Manga 27", "Bleach 700"}, "", false},
		{"most common spelling wins", []string{"BLACK CLOVER, CHAPTER 388", "Black Clover 387", "Black Clover 386"}, "Black Clover", true},
		{"no numbered posts", []string{"Aviso a los lectores"}, "", false},
	}
	for _, c := range cases {
		got, ok := wpPostsSeries(posts(c.titles...))
		if got != c.want || ok != c.ok {
			t.Errorf("%s: wpPostsSeries() = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestWordPressCategorySeriesTitleFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"name":"Manga Series"}`)
	}))
	defer srv.Close()

	w := NewWordPressCategory(&Grabber{URL: srv.URL + "/category/manga/"})
	w.categoryID = 2
	title, err := w.seriesTitle([]wpPost{{Link: "https://site.test/about/"}})
	if err != nil || title != "Manga Series" {
		t.Errorf("seriesTitle() = %q, %v; want %q", title, err, "Manga Series")
	}
}

func TestWordPressCategoryLazyPlaceholders(t *testing.T) {
	// the Bv loader's placeholder: an SVG whose bv-img-url is the real page
	svg := func(url string) string {
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(
			[]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="727" height="1062" bv-img-url="`+url+`"></svg>`))
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div class="entry-content">
			<img src="https://cdn.test/01.jpg" class="bv-image-preloaded">
			<img src="` + svg("https://cdn.test/02.jpg") + `" class="bv-image-lazyload">
			<img src="data:image/gif;base64,R0lGOD" data-lazy-src="https://cdn.test/03.jpg">
			<img src="data:image/gif;base64,R0lGOD">
		</div>`))
	if err != nil {
		t.Fatal(err)
	}

	pages := wpPageImages(doc)
	want := []string{"https://cdn.test/01.jpg", "https://cdn.test/02.jpg", "https://cdn.test/03.jpg"}
	if len(pages) != len(want) {
		t.Fatalf("got %d pages, want %d: %+v", len(pages), len(want), pages)
	}
	for i, p := range pages {
		if p.URL != want[i] {
			t.Errorf("page %d = %q, want %q", i, p.URL, want[i])
		}
	}
}
