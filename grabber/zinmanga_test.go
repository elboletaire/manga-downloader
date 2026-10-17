// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

const zinmangaSeriesURL = "https://www.zinmanga.net/manga/academy-idolmaster-gold-rush"

func TestZinmangaTest(t *testing.T) {
	cases := map[string]bool{
		zinmangaSeriesURL:                  true,
		"https://zinmanga.net/manga/x":     true,
		zinmangaSeriesURL + "/chapter-1":   true,
		"https://example.com/zinmanga.net": false,
	}
	for input, want := range cases {
		z := &Zinmanga{Grabber: &Grabber{URL: input}}
		got, err := z.Test()
		if err != nil || got != want {
			t.Errorf("Test(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
}

func TestZinmangaResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// chapter URL → series URL
		{zinmangaSeriesURL + "/chapter-1", zinmangaSeriesURL},
		{zinmangaSeriesURL + "/chapter-1/", zinmangaSeriesURL},
		{zinmangaSeriesURL + "/chapter-1?foo=bar#top", zinmangaSeriesURL},
		{"https://ZINMANGA.net/Manga/academy-idolmaster-gold-rush/chapter-1", "https://ZINMANGA.net/Manga/academy-idolmaster-gold-rush"},
		// series URLs unchanged
		{zinmangaSeriesURL, zinmangaSeriesURL},
		{zinmangaSeriesURL + "/", zinmangaSeriesURL + "/"},
	}

	for _, c := range cases {
		z := &Zinmanga{Grabber: &Grabber{URL: c.input}}
		z.resolveSeriesURL()
		if z.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, z.URL, c.want)
		}
	}
}

func TestZinmangaChaptersFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/zinmanga/chapters-1.json")
	if err != nil {
		t.Fatal(err)
	}

	var feed zinmangaChaptersFeed
	if err := json.Unmarshal(body, &feed); err != nil {
		t.Fatalf("decoding feed: %v", err)
	}
	if !feed.Success || feed.Data.LastPage != 3 {
		t.Fatalf("feed = success %v, last_page %d; want success and 3 pages", feed.Success, feed.Data.LastPage)
	}

	got := zinmangaChapters(feed.Data.Chapters)
	wants := []struct {
		number float64
		slug   string
		label  string
	}{
		{24, "chapter-24", "Chapter 24"},
		{23, "chapter-23", "Chapter 23"},
	}
	if len(got) != len(wants) {
		t.Fatalf("got %d chapters, want %d", len(got), len(wants))
	}
	for i, w := range wants {
		c := got[i].(*ZinmangaChapter)
		if c.Number != w.number || c.Slug != w.slug || ChapterLabel(c) != w.label {
			t.Errorf("chapter[%d] = {Number %v, Slug %q, Label %q}, want {%v, %q, %q}",
				i, c.Number, c.Slug, ChapterLabel(c), w.number, w.slug, w.label)
		}
	}
}

func TestZinmangaPagesFixture(t *testing.T) {
	f, err := os.Open("testdata/zinmanga/chapter.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatal(err)
	}

	pages := zinmangaPages(doc)
	wants := []string{
		"https://img-r1.2xstorage.com/academy-idolmaster-gold-rush/1/0.webp",
		"https://img-r1.2xstorage.com/academy-idolmaster-gold-rush/1/1.webp",
	}
	if len(pages) != len(wants) {
		t.Fatalf("got %d pages, want %d (the logo image is not a reader page)", len(pages), len(wants))
	}
	for i, w := range wants {
		if pages[i].URL != w || pages[i].Number != int64(i+1) {
			t.Errorf("page[%d] = {Number %d, URL %q}, want {%d, %q}", i, pages[i].Number, pages[i].URL, i+1, w)
		}
	}
}

// newTestZinmanga serves the fixtures as the site, and returns a grabber for
// the given series or chapter URL on that server
func newTestZinmanga(t *testing.T, path string) *Zinmanga {
	t.Helper()

	files := map[string]string{
		"/manga/academy-idolmaster-gold-rush": "series.html",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/comics/academy-idolmaster-gold-rush/chapters" {
			name := "chapters-" + r.URL.Query().Get("page") + ".json"
			b, err := os.ReadFile("testdata/zinmanga/" + name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
			return
		}
		name, ok := files[r.URL.Path]
		if strings.HasPrefix(r.URL.Path, "/manga/academy-idolmaster-gold-rush/chapter-") {
			name, ok = "chapter.html", true
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile("testdata/zinmanga/" + name)
		if err != nil {
			t.Error(err)
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)

	return &Zinmanga{Grabber: &Grabber{URL: srv.URL + path, Settings: &Settings{}}}
}

func TestZinmangaFetch(t *testing.T) {
	z := newTestZinmanga(t, "/manga/academy-idolmaster-gold-rush/chapter-24")

	title, err := z.FetchTitle()
	if err != nil || title != "Academy Idolmaster: Gold Rush" {
		t.Fatalf("FetchTitle() = %q, %v; want the series title", title, err)
	}

	chapters, errs := z.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	// the three fixture pages hold two chapters each, walked in order
	var numbers []string
	for _, c := range chapters {
		numbers = append(numbers, ChapterLabel(c))
	}
	if got, want := strings.Join(numbers, ", "), "Chapter 24, Chapter 23, Chapter 14, Chapter 13, Chapter 4, Chapter 3"; got != want {
		t.Fatalf("chapters = %q, want %q", got, want)
	}

	chapter, err := z.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error: %v", err)
	}
	if chapter.PagesCount != 2 || len(chapter.Pages) != 2 {
		t.Errorf("FetchChapter() got %d pages, want 2", len(chapter.Pages))
	}
}
