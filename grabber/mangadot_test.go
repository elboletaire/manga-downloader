// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"reflect"
	"testing"
)

// mangadotListing is the chapter list of series 229: a scraper row, a group
// upload with a group, one without a title, and a Spanish and a French row
const mangadotListing = `[` +
	`{"id":2043556,"chapter_number":1,"volume_number":null,"chapter_title":"Chapter 1","language":"fr","group_id":18291,"group_name":"Anime-Sama","group_slug":"anime-sama","group_is_scanlator":false,"groups":[{"id":18291,"name":"Anime-Sama","slug":"anime-sama","is_scanlator":false}],"page_count":53,"source":"user","scanlator_name":null},` +
	`{"id":39688,"chapter_number":1,"volume_number":null,"chapter_title":"Chapter 1","language":"en","group_id":0,"group_name":null,"group_slug":null,"groups":[],"page_count":52,"source":"scraper","scanlator_name":null},` +
	`{"id":1807106,"chapter_number":112,"volume_number":null,"chapter_title":null,"language":"en","group_id":18210,"group_name":"K Manga","groups":[{"id":18210,"name":"K Manga","slug":"k-manga","is_scanlator":false}],"page_count":34,"source":"user","scanlator_name":null},` +
	`{"id":3182177,"chapter_number":1,"chapter_title":"Chapter 1","language":"en","group_name":"Kayn Scan","groups":[{"id":9181,"name":"Kayn Scan"}],"scanlator_name":"Kayn Scan","source":"user"},` +
	`{"id":25796,"chapter_number":1,"chapter_title":"Chapter 1","language":"es","group_id":0,"group_name":null,"groups":[],"scanlator_name":"","source":"user"}` +
	`]`

// mangadotServer serves series 229 with the listing above and the pages of
// the two releases the tests fetch
func mangadotServer(t *testing.T) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	respond(mux, "/api/manga/229", `{"manga":{"id":229,"title":"Grand Blue Dreaming"},"total_chapters":143}`)
	respond(mux, "/api/manga/229/chapters/list", mangadotListing)
	respond(mux, "/api/uploads/1807106/images", `{"chapter":{"id":1807106,"manga_id":229,"chapter_number":"112.00"},"images":[`+
		`{"url":"/chapters/manga_229/user_1604_ch112_en/001.webp","w":960,"h":1378,"filename":"001.webp"},`+
		`{"url":"/chapters/manga_229/user_1604_ch112_en/002.jpg","w":960,"h":1378,"filename":"002.jpg"}]}`)
	respond(mux, "/api/chapters/39688/images", `{"chapter":{"id":39688,"manga_id":229,"chapter_number":"1.0"},"images":[`+
		`{"url":"/chapters/manga_229/chapter_1/001.webp","w":0,"h":0}]}`)
	respond(mux, "/api/uploads/1807006/images", `{"chapter":{"id":1807006,"manga_id":229},"images":[]}`)

	return mux
}

func TestMangadotTest(t *testing.T) {
	cases := map[string]bool{
		"https://mangadot.net/manga/229":                   true,
		"https://mangadot.net/chapter/1807106?source=user": true,
		"https://www.mangadot.net/chapter/39688":           true,
		"https://mangadot.net/volume/1807006":              true,
		"https://mangadot.net/":                            false,
		"https://example.com/manga/229":                    false,
		"https://notmangadot.net/manga/229":                false,
	}
	for url, want := range cases {
		got, err := NewMangadot(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil {
			t.Fatalf("Test(%s) error = %v", url, err)
		}
		if got != want {
			t.Errorf("Test(%s) = %v, want %v", url, got, want)
		}
	}
}

func TestMangadotResolveSeriesURL(t *testing.T) {
	srv := serve(t, mangadotServer(t))
	overrideBase(t, &mangadotBase, srv.URL)

	cases := []struct {
		url  string
		want string
		err  bool
	}{
		{"https://mangadot.net/chapter/1807106?source=user", srv.URL + "/manga/229", false},
		{"https://mangadot.net/chapter/39688", srv.URL + "/manga/229", false},
		{"https://mangadot.net/volume/1807006", srv.URL + "/manga/229", false},
		{"https://mangadot.net/manga/229", "https://mangadot.net/manga/229", false},
		{"https://mangadot.net/chapter/999999?source=user", "https://mangadot.net/chapter/999999?source=user", true},
	}
	for _, c := range cases {
		m := NewMangadot(&Grabber{URL: c.url, Settings: &Settings{}})
		err := m.resolveSeriesURL()
		if (err != nil) != c.err {
			t.Errorf("resolveSeriesURL(%s) error = %v, want error %v", c.url, err, c.err)
		}
		if err == nil && m.URL != c.want {
			t.Errorf("resolveSeriesURL(%s) = %s, want %s", c.url, m.URL, c.want)
		}
	}
}

func TestMangadotFetchTitle(t *testing.T) {
	srv := serve(t, mangadotServer(t))
	overrideBase(t, &mangadotBase, srv.URL)

	m := NewMangadot(&Grabber{URL: "https://mangadot.net/manga/229", Settings: &Settings{}})
	title, err := m.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "Grand Blue Dreaming" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Grand Blue Dreaming")
	}
}

func TestMangadotFetchChapters(t *testing.T) {
	srv := serve(t, mangadotServer(t))
	overrideBase(t, &mangadotBase, srv.URL)

	type want struct {
		ids    []string
		groups []string
	}
	cases := []struct {
		lang  string
		want  want
		langs []string
	}{
		// the default is English, and keeps the scraper's row and the uploads
		{"", want{
			ids:    []string{"scraper:39688", "user:1807106", "user:3182177"},
			groups: []string{"", "K Manga", "Kayn Scan"},
		}, []string{"fr", "en", "es"}},
		{FilterAll, want{
			ids:    []string{"user:2043556", "scraper:39688", "user:1807106", "user:3182177", "user:25796"},
			groups: []string{"Anime-Sama", "", "K Manga", "Kayn Scan", ""},
		}, []string{"fr", "en", "es"}},
		{"fr", want{
			ids:    []string{"user:2043556"},
			groups: []string{"Anime-Sama"},
		}, []string{"fr", "en", "es"}},
	}
	for _, c := range cases {
		m := NewMangadot(&Grabber{URL: "https://mangadot.net/manga/229", Settings: &Settings{Language: c.lang}})
		chapters, errs := m.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("FetchChapters(%q) errors = %v", c.lang, errs)
		}
		var ids, groups []string
		for _, f := range chapters {
			ids = append(ids, ChapterOf(f).ID)
			groups = append(groups, ChapterOf(f).Group)
		}
		if !reflect.DeepEqual(ids, c.want.ids) {
			t.Errorf("FetchChapters(%q) IDs = %v, want %v", c.lang, ids, c.want.ids)
		}
		if !reflect.DeepEqual(groups, c.want.groups) {
			t.Errorf("FetchChapters(%q) groups = %v, want %v", c.lang, groups, c.want.groups)
		}
		assertFilterOptions(t, "mangadot "+c.lang, m.LanguageOptions(), c.langs, true, "")
	}
}

func TestMangadotFetchChaptersWithoutEnglish(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/api/manga/229/chapters/list", `[`+
		`{"id":1,"chapter_number":1,"language":"pl","source":"user","groups":[]},`+
		`{"id":2,"chapter_number":2,"language":"pl","source":"user","groups":[]}]`)
	srv := serve(t, mux)
	overrideBase(t, &mangadotBase, srv.URL)

	m := NewMangadot(&Grabber{URL: "https://mangadot.net/manga/229", Settings: &Settings{}})
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 2 {
		t.Errorf("FetchChapters() returned %d chapters, want 2", len(chapters))
	}
}

func TestMangadotScanlatorFromScanlatorName(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/api/manga/229/chapters/list", `[`+
		`{"id":7,"chapter_number":3,"language":"en","source":"user","scanlator_name":"Official?","groups":[]}]`)
	srv := serve(t, mux)
	overrideBase(t, &mangadotBase, srv.URL)

	m := NewMangadot(&Grabber{URL: "https://mangadot.net/manga/229", Settings: &Settings{Language: "en"}})
	chapters, _ := m.FetchChapters()
	if len(chapters) != 1 || ChapterOf(chapters[0]).Group != "Official?" {
		t.Fatalf("FetchChapters() = %v, want one chapter grouped as %q", chapters, "Official?")
	}
}

func TestMangadotChapterLabel(t *testing.T) {
	srv := serve(t, mangadotServer(t))
	overrideBase(t, &mangadotBase, srv.URL)

	m := NewMangadot(&Grabber{URL: "https://mangadot.net/manga/229", Settings: &Settings{Language: "en"}})
	chapters, _ := m.FetchChapters()
	if got := ChapterLabel(chapters[1]); got != "Chapter 112" {
		t.Errorf("ChapterLabel() = %q, want %q", got, "Chapter 112")
	}
}

func TestMangadotFetchChapter(t *testing.T) {
	srv := serve(t, mangadotServer(t))
	overrideBase(t, &mangadotBase, srv.URL)

	m := NewMangadot(&Grabber{URL: "https://mangadot.net/manga/229", Settings: &Settings{Language: FilterAll}})
	chapters, _ := m.FetchChapters()

	// a group upload is read from the uploads table
	chapter, err := m.FetchChapter(chapters[2])
	if err != nil {
		t.Fatalf("FetchChapter(upload) error = %v", err)
	}
	wantPages := []Page{
		{Number: 1, URL: srv.URL + "/chapters/manga_229/user_1604_ch112_en/001.webp"},
		{Number: 2, URL: srv.URL + "/chapters/manga_229/user_1604_ch112_en/002.jpg"},
	}
	if !reflect.DeepEqual(chapter.Pages, wantPages) {
		t.Errorf("FetchChapter(upload) pages = %v, want %v", chapter.Pages, wantPages)
	}
	if chapter.ID != "user:1807106" || chapter.GetGroup() != "K Manga" {
		t.Errorf("FetchChapter(upload) ID %q group %q, want user:1807106 and K Manga", chapter.ID, chapter.GetGroup())
	}
	if ChapterLabel(chapter) != "Chapter 112" {
		t.Errorf("FetchChapter(upload) label = %q, want %q", ChapterLabel(chapter), "Chapter 112")
	}

	// a scraper row is read from the chapters table
	chapter, err = m.FetchChapter(chapters[1])
	if err != nil {
		t.Fatalf("FetchChapter(scraper) error = %v", err)
	}
	wantPages = []Page{{Number: 1, URL: srv.URL + "/chapters/manga_229/chapter_1/001.webp"}}
	if !reflect.DeepEqual(chapter.Pages, wantPages) {
		t.Errorf("FetchChapter(scraper) pages = %v, want %v", chapter.Pages, wantPages)
	}
	if chapter.ID != "scraper:39688" || chapter.GetGroup() != "" {
		t.Errorf("FetchChapter(scraper) ID %q group %q, want scraper:39688 and no group", chapter.ID, chapter.GetGroup())
	}
}
