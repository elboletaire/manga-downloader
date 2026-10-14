// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
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

	chapters := wpChapters(posts)
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
