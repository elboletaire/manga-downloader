// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoffeemangaResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://coffeemanga.net/manga/under-the-oak-tree/chapter-74/", "https://coffeemanga.net/manga/under-the-oak-tree"},
		{"https://www.coffeemanga.net/manga/under-the-oak-tree/chapter-74/?x=1", "https://www.coffeemanga.net/manga/under-the-oak-tree"},
		{"https://coffeemanga.net/manga/under-the-oak-tree/", "https://coffeemanga.net/manga/under-the-oak-tree"},
		{"https://COFFEEMANGA.net/Manga/Under-The-Oak-Tree#top", "https://COFFEEMANGA.net/Manga/Under-The-Oak-Tree"},
	}

	for _, c := range cases {
		cm := &Coffeemanga{Grabber: &Grabber{URL: c.input}}
		cm.resolveSeriesURL()
		if cm.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, cm.URL, c.want)
		}
	}
}

func TestCoffeemangaTest(t *testing.T) {
	base, hits := newFixtureServer(t, "https://coffeemanga.net", map[string]string{
		"/manga/under-the-oak-tree/":            "coffeemanga_series.html",
		"/manga/under-the-oak-tree/chapter-74/": "coffeemanga_reader.html",
		"/manga/other/":                         "coffeemanga_other_inertia.html",
		"/manga/plain/":                         "unrelated_page.html",
	})

	cases := []struct {
		path string
		want bool
	}{
		{"/manga/under-the-oak-tree/", true},
		{"/manga/under-the-oak-tree/chapter-74/", true},
		{"/manga/other/", false},
		{"/manga/plain/", false},
		// outside /manga/ the page is never fetched
		{"/read/some-series/", false},
	}

	for _, c := range cases {
		cm := NewCoffeemanga(&Grabber{URL: base + c.path})
		got, err := cm.Test()
		if err != nil {
			t.Fatalf("Test(%s): %v", c.path, err)
		}
		if got != c.want {
			t.Errorf("Test(%s) = %v, want %v", c.path, got, c.want)
		}
	}

	if n := hits.count("/read/some-series/"); n != 0 {
		t.Errorf("a non-/manga/ URL was fetched %d times", n)
	}
}

func TestCoffeemangaFetchTitle(t *testing.T) {
	base, _ := newFixtureServer(t, "https://coffeemanga.net", map[string]string{
		"/manga/under-the-oak-tree/": "coffeemanga_series.html",
	})

	cm := NewCoffeemanga(&Grabber{URL: base + "/manga/under-the-oak-tree/chapter-74/"})
	title, err := cm.FetchTitle()
	if err != nil {
		t.Fatal(err)
	}
	if title != "Under the Oak Tree" {
		t.Errorf("title = %q", title)
	}
}

// TestCoffeemangaFetchChaptersPaginated walks the two listing pages, chapters/1.json
// and chapters/2.json
func TestCoffeemangaFetchChaptersPaginated(t *testing.T) {
	base, _ := newFixtureServer(t, "https://coffeemanga.net", map[string]string{
		"/manga/under-the-oak-tree/chapters/1.json": "coffeemanga_chapters_1.json",
		"/manga/under-the-oak-tree/chapters/2.json": "coffeemanga_chapters_2.json",
	})

	cm := NewCoffeemanga(&Grabber{URL: base + "/manga/under-the-oak-tree/chapter-74/"})
	filterables, errs := cm.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(filterables) != 3 {
		t.Fatalf("got %d chapters, want 3", len(filterables))
	}

	wantNumbers := []float64{171, 170, 74}
	wantLocked := []bool{false, true, false}
	for i, f := range filterables {
		c := f.(*CoffeemangaChapter)
		if c.Number != wantNumbers[i] || c.Locked != wantLocked[i] {
			t.Errorf("chapter %d = number %v locked %v, want %v %v", i, c.Number, c.Locked, wantNumbers[i], wantLocked[i])
		}
		if c.URL != base+"/manga/under-the-oak-tree/chapter-"+formatChapterNumber(wantNumbers[i])+"/" {
			t.Errorf("chapter %d URL = %q", i, c.URL)
		}
	}
	if title := filterables[1].(*CoffeemangaChapter).Title; title != "The Oak" {
		t.Errorf("title = %q", title)
	}
}

func TestCoffeemangaPageURLs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(identifyFixtureDir, "coffeemanga_reader.html"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	urls, err := coffeemangaPageURLs(body)
	if err != nil {
		t.Fatal(err)
	}
	// the third page is still a placeholder, so it has no URL yet
	want := []string{
		"https://coffeemanga.net/img/AAA/raw:1/MDAx.jpg",
		"https://coffeemanga.net/img/BBB/raw:1/MDAy.jpg",
	}
	if len(urls) != len(want) || urls[0] != want[0] || urls[1] != want[1] {
		t.Errorf("urls = %v, want %v", urls, want)
	}
}
