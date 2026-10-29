// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"html"
	"strings"
	"testing"
)

func TestManhwa18Test(t *testing.T) {
	cases := map[string]bool{
		"https://manhwa18.net/manga/my-lewd-college-friends":     true,
		"https://www.manhwa18.net/manga/my-lewd-college-friends": true,
		"https://manhwa18.cc/webtoon/the-meek-cat":               false,
		"https://manhwa18tx.com/manhwa/taming-a-female-bully/":   false,
	}
	for url, want := range cases {
		m := &Manhwa18{Grabber: &Grabber{URL: url}}
		got, err := m.Test()
		if err != nil || got != want {
			t.Errorf("Test(%q) = %v, %v, want %v", url, got, err, want)
		}
	}
}

func TestManhwa18ResolveSeriesURL(t *testing.T) {
	cases := map[string]string{
		"https://manhwa18.net/manga/my-lewd-college-friends":                "https://manhwa18.net/manga/my-lewd-college-friends",
		"https://manhwa18.net/manga/my-lewd-college-friends/chapter-1-464":  "https://manhwa18.net/manga/my-lewd-college-friends",
		"https://manhwa18.net/manga/my-lewd-college-friends/chapter-60/":    "https://manhwa18.net/manga/my-lewd-college-friends",
		"https://manhwa18.net/manga/my-lewd-college-friends/chapter-60?x=1": "https://manhwa18.net/manga/my-lewd-college-friends",
		"https://manhwa18.net/manga/my-lewd-college-friends/chapter-60#top": "https://manhwa18.net/manga/my-lewd-college-friends",
		"https://www.manhwa18.net/manga/my-lewd-college-friends/chapter-2":  "https://www.manhwa18.net/manga/my-lewd-college-friends",
		"https://manhwa18.net/": "https://manhwa18.net/",
	}
	for input, want := range cases {
		m := &Manhwa18{Grabber: &Grabber{URL: input}}
		if err := m.resolveSeriesURL(); err != nil {
			t.Fatalf("resolveSeriesURL(%q) error = %v", input, err)
		}
		if m.URL != want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", input, m.URL, want)
		}
	}
}

func TestDecodeManhwa18Page(t *testing.T) {
	series := `{"component":"Manga","props":{"manga":{"name":"My Lewd College Friends & Co","slug":"my-lewd-college-friends"},` +
		`"chapters":[{"name":"Chapter 2","slug":"chapter-2"},{"name":"Chapter 1","slug":"chapter-1"}]}}`
	reader := `{"component":"Chapter","props":{"chapterImages":[{"src":"https://min.manhwa18.net/1.jpg"},{"src":"https://min.manhwa18.net/2.jpg"}]}}`

	page, err := decodeManhwa18Page(strings.NewReader(`<div id="app" data-page="`+html.EscapeString(series)+`"></div>`), "series")
	if err != nil {
		t.Fatal(err)
	}
	if page.Props.Manga.Name != "My Lewd College Friends & Co" || page.Props.Manga.Slug != "my-lewd-college-friends" {
		t.Errorf("manga = %+v", page.Props.Manga)
	}
	if len(page.Props.Chapters) != 2 || page.Props.Chapters[0].Slug != "chapter-2" || page.Props.Chapters[1].Name != "Chapter 1" {
		t.Errorf("chapters = %+v", page.Props.Chapters)
	}

	page, err = decodeManhwa18Page(strings.NewReader(`<div id="app" data-page="`+html.EscapeString(reader)+`"></div>`), "reader")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Props.ChapterImages) != 2 || page.Props.ChapterImages[1].Src != "https://min.manhwa18.net/2.jpg" {
		t.Errorf("chapterImages = %+v", page.Props.ChapterImages)
	}

	if _, err := decodeManhwa18Page(strings.NewReader(`<div></div>`), "missing"); err == nil {
		t.Error("a page without data-page decoded without an error")
	}
}
