// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
)

// japscanDoc reads a japscan fixture
func japscanDoc(t *testing.T, name string) *goquery.Document {
	t.Helper()
	body, err := os.ReadFile("testdata/japscan/" + name)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestJapscanTest(t *testing.T) {
	for url, want := range map[string]bool{
		"https://japscan.foo/manga/blue-lock/":            true,
		"https://www.japscan.foo/manga/blue-lock/186/":    true,
		"https://www.japscan.lol/manga/blue-lock/":        true,
		"https://japscan.com/manga/blue-lock/":            true,
		"https://www.japscan.vip/manga/blue-lock/":        true,
		"https://japscan.example.com/manga/blue-lock/":    false,
		"https://notjapscan.foo/manga/blue-lock/":         false,
		"https://example.org/japscan.foo/manga/blue-lock": false,
	} {
		j := &Japscan{Grabber: &Grabber{URL: url}}
		if got, err := j.Test(); err != nil || got != want {
			t.Errorf("Test(%q) = %v, %v; want %v", url, got, err, want)
		}
	}
}

func TestJapscanResolveSeriesURL(t *testing.T) {
	cases := map[string]string{
		"https://www.japscan.foo/manga/blue-lock/186/":       "https://www.japscan.foo/manga/blue-lock/",
		"https://www.japscan.foo/manga/blue-lock/186":        "https://www.japscan.foo/manga/blue-lock/",
		"https://www.japscan.foo/manga/blue-lock/186/?x=1":   "https://www.japscan.foo/manga/blue-lock/",
		"https://www.japscan.foo/manga/blue-lock/364.2/#top": "https://www.japscan.foo/manga/blue-lock/",
		"https://www.japscan.foo/manga/blue-lock/":           "https://www.japscan.foo/manga/blue-lock/",
		"https://www.japscan.foo/manga/blue-lock":            "https://www.japscan.foo/manga/blue-lock",
		"https://www.japscan.lol/manga/blue-lock/186/":       "https://www.japscan.foo/manga/blue-lock/",
		"https://japscan.vip/manga/blue-lock/":               "https://www.japscan.foo/manga/blue-lock/",
		"https://WWW.JAPSCAN.FOO/Manga/Blue-Lock/186/":       "https://WWW.JAPSCAN.FOO/Manga/Blue-Lock/",
	}
	for input, want := range cases {
		j := &Japscan{Grabber: &Grabber{URL: input}}
		j.resolveSeriesURL()
		if j.URL != want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", input, j.URL, want)
		}
	}
}

func TestJapscanChapters(t *testing.T) {
	chapters, err := japscanChapters(japscanDoc(t, "series.html"), "https://www.japscan.foo")
	if err != nil {
		t.Fatal(err)
	}

	type listed struct {
		number float64
		title  string
		volume int
		path   string
	}
	want := []listed{
		{370, "Les textes sacrés du tyran", 41, "/manga/blue-lock/370/"},
		{364.2, "Pari à un point (2)", 41, "/manga/blue-lock/364.2/"},
		{1, "Ce que tu veux", 40, "/manga/blue-lock/1/"},
	}
	var got []listed
	for _, c := range chapters {
		got = append(got, listed{c.Number, c.Title, c.Volume, c.ID})
		if c.URL != "https://www.japscan.foo"+c.ID {
			t.Errorf("chapter %s URL = %q", c.ID, c.URL)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("chapters = %+v, want %+v", got, want)
	}
}

func TestJapscanChaptersNoneFound(t *testing.T) {
	doc := japscanDoc(t, "reader.html")
	if _, err := japscanChapters(doc, "https://www.japscan.foo"); err == nil {
		t.Error("japscanChapters on a page without a list = no error")
	}
}

func TestJapscanPageCount(t *testing.T) {
	if n := japscanPageCount(japscanDoc(t, "reader.html")); n != 3 {
		t.Errorf("pages of the reader = %d, want 3", n)
	}
	if n := japscanPageCount(japscanDoc(t, "reader-puzzle.html")); n != 0 {
		t.Errorf("pages of the puzzle = %d, want 0", n)
	}
}

func TestJapscanPages(t *testing.T) {
	const u1 = "https://c4.japscan.foo/a/b/c/1.png?yu=9042"
	const u2 = "https://c4.japscan.foo/a/b/c/2.png?yu=9042"
	const u3 = "https://c4.japscan.foo/a/b/c/3.png?yu=9042"
	const u4 = "https://c4.japscan.foo/a/b/c/4.png?yu=9042"
	reqs := []browser.Request{
		// the reader's token request, never a page
		{URL: "https://c4.japscan.foo/a/b/token", Type: "Fetch", Loaded: true},
		{URL: u1, Type: "Fetch", Loaded: true},
		{URL: u1, Type: "Image", Loaded: true},
		{URL: u1, Type: "Image", Loaded: true},
		{URL: u2, Type: "Fetch", Loaded: true},
		{URL: u2, Type: "Image", Loaded: true},
		// sent but not loaded yet still counts: only the URL is needed
		{URL: u3, Type: "Image"},
		{URL: u4, Type: "Image", Failed: true},
	}
	got, err := japscanPages(reqs, 4)
	if err != nil || !reflect.DeepEqual(got, []string{u1, u2, u3, u4}) {
		t.Errorf("japscanPages = %v, %v; want [%s %s %s %s]", got, err, u1, u2, u3, u4)
	}

	// a shortfall says how many of the images failed or never finished
	_, err = japscanPages(reqs, 5)
	if err == nil || !strings.Contains(err.Error(), "found 4 page images for the 5 pages (1 failed to load, 1 never finished)") {
		t.Errorf("japscanPages short = %v, want the counts of the shortfall", err)
	}
}

func TestJapscanFetchedChapterIsKeptBriefly(t *testing.T) {
	j := &Japscan{Grabber: &Grabber{URL: "https://www.japscan.foo/manga/blue-lock/"}}
	chapter := &Chapter{Title: "x", Pages: []Page{{Number: 1, URL: "https://c1.japscan.foo/1.png"}}}
	j.rememberChapter("https://www.japscan.foo/manga/blue-lock/186/", chapter)

	got := j.recentChapter("https://www.japscan.foo/manga/blue-lock/186/")
	if got == nil || len(got.Pages) != 1 {
		t.Fatalf("recentChapter = %+v, want the remembered chapter", got)
	}
	// a copy: the caller may skip pages of it
	got.Pages = nil
	if again := j.recentChapter("https://www.japscan.foo/manga/blue-lock/186/"); again == nil || len(again.Pages) != 1 {
		t.Errorf("the remembered chapter changed with its copy: %+v", again)
	}

	j.fetchedMu.Lock()
	entry := j.fetched["https://www.japscan.foo/manga/blue-lock/186/"]
	entry.at = time.Now().Add(-2 * japscanFetchedFor)
	j.fetched["https://www.japscan.foo/manga/blue-lock/186/"] = entry
	j.fetchedMu.Unlock()
	if j.recentChapter("https://www.japscan.foo/manga/blue-lock/186/") != nil {
		t.Error("a chapter older than japscanFetchedFor is still returned")
	}
}
