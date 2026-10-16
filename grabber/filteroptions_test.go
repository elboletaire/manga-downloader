// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// serve starts a test server for the mux, closed when the test ends
func serve(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

// overrideBase points an API base var at value for the length of the test
func overrideBase(t *testing.T, base *string, value string) {
	t.Helper()

	old := *base
	*base = value
	t.Cleanup(func() { *base = old })
}

// respond answers every request for the path with the given JSON body
func respond(mux *http.ServeMux, path, body string) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	})
}

func assertFilterOptions(t *testing.T, name string, got FilterOptions, values []string, all bool, current string) {
	t.Helper()

	if !reflect.DeepEqual(got.Values, values) || got.All != all || got.Current != current {
		t.Errorf("%s: got %+v, want Values %v, All %v, Current %q", name, got, values, all, current)
	}
}

func TestMangadexLanguageOptions(t *testing.T) {
	const id = "a1c7c817-4e59-43b7-9365-09675a149a6f"
	var feedLanguages [][]string

	mux := http.NewServeMux()
	respond(mux, "/manga/"+id, `{"data":{"attributes":{"title":{"en":"One Piece"},"altTitles":[],"availableTranslatedLanguages":["en","es","ja","es"]}}}`)
	mux.HandleFunc("/manga/"+id+"/feed", func(w http.ResponseWriter, r *http.Request) {
		feedLanguages = append(feedLanguages, r.URL.Query()["translatedLanguage[]"])
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	srv := serve(t, mux)
	overrideBase(t, &mangadexAPI, srv.URL)
	url := "https://mangadex.org/title/" + id + "/one-piece"

	m := NewMangadex(&Grabber{URL: url, Settings: &Settings{}})
	assertFilterOptions(t, "mangadex", m.LanguageOptions(), []string{"en", "es", "ja"}, true, "")

	// the manga endpoint is read once for the title and the languages alike
	if _, err := m.FetchTitle(); err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}

	// an empty language and "all" both leave the feed unfiltered, a language filters it
	for _, lang := range []string{"", FilterAll, "es"} {
		m := NewMangadex(&Grabber{URL: url, Settings: &Settings{Language: lang}})
		if _, errs := m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters(%q) errors = %v", lang, errs)
		}
	}

	want := [][]string{nil, nil, {"es"}}
	if !reflect.DeepEqual(feedLanguages, want) {
		t.Errorf("feed translatedLanguage[] = %v, want %v", feedLanguages, want)
	}
}

func TestMangaballLanguageOptions(t *testing.T) {
	const series = "/title-detail/baki-6a5ffe5d90273b5b995225d2/"

	mux := http.NewServeMux()
	respond(mux, "/api/v1/chapter/chapter-listing-by-title-id", `{"status":"success","data":[`+
		`{"id":"a","lang":"en","name":"","number":1},`+
		`{"id":"b","lang":"es","name":"","number":1},`+
		`{"id":"c","lang":"en","name":"","number":2}]}`)
	srv := serve(t, mux)
	overrideBase(t, &mangaballAPI, srv.URL+"/api/v1")
	overrideBase(t, &mangaballSite, srv.URL)

	cases := []struct {
		lang   string
		chaps  int
		values []string
	}{
		{"", 2, []string{"en", "es"}}, // the default stays English
		{FilterAll, 3, []string{"en", "es"}},
		{"es", 1, []string{"en", "es"}},
	}
	for _, c := range cases {
		m := NewMangaball(&Grabber{URL: srv.URL + series, Settings: &Settings{Language: c.lang}})
		chapters, errs := m.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("FetchChapters(%q) errors = %v", c.lang, errs)
		}
		if len(chapters) != c.chaps {
			t.Errorf("FetchChapters(%q) returned %d chapters, want %d", c.lang, len(chapters), c.chaps)
		}
		assertFilterOptions(t, "mangaball "+c.lang, m.LanguageOptions(), c.values, true, "")
	}
}

func TestMangataroLanguageOptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/x", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body data-manga-id="m1"><h1>Title</h1></body></html>`))
	})
	respond(mux, "/auth/manga-chapters", `{"success":true,"has_more":false,"chapters":[`+
		`{"id":"c1","chapter":"1","title":"","language":"en"},`+
		`{"id":"c2","chapter":"1","title":"","language":"es"},`+
		`{"id":"c3","chapter":"2","title":"","language":"en"}]}`)
	srv := serve(t, mux)

	cases := []struct {
		lang  string
		chaps int
	}{
		{"", 2},
		{FilterAll, 3},
		{"es", 1},
	}
	for _, c := range cases {
		m := NewMangataro(&Grabber{URL: srv.URL + "/manga/x", Settings: &Settings{Language: c.lang}})
		chapters, errs := m.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("FetchChapters(%q) errors = %v", c.lang, errs)
		}
		if len(chapters) != c.chaps {
			t.Errorf("FetchChapters(%q) returned %d chapters, want %d", c.lang, len(chapters), c.chaps)
		}
		assertFilterOptions(t, "mangataro "+c.lang, m.LanguageOptions(), []string{"en", "es"}, true, "")
	}
}

func TestRoliascanLanguageOptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/series/x", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body data-manga-id="m1"><h1>Title</h1></body></html>`))
	})
	respond(mux, "/auth/manga-chapters", `{"success":true,"has_more":false,"chapters":[`+
		`{"id":"c1","chapter":"1","title":"","language":"en"},`+
		`{"id":"c2","chapter":"2","title":"","language":"es"},`+
		`{"id":"c3","chapter":"3","title":"","language":""}]}`)
	srv := serve(t, mux)

	cases := []struct {
		lang  string
		chaps int
	}{
		{"", 2}, // the default stays English; a chapter without a language still passes
		{FilterAll, 3},
		{"es", 2},
	}
	for _, c := range cases {
		m := NewRoliascan(&Grabber{URL: srv.URL + "/series/x", Settings: &Settings{Language: c.lang}})
		chapters, errs := m.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("FetchChapters(%q) errors = %v", c.lang, errs)
		}
		if len(chapters) != c.chaps {
			t.Errorf("FetchChapters(%q) returned %d chapters, want %d", c.lang, len(chapters), c.chaps)
		}
		assertFilterOptions(t, "roliascan "+c.lang, m.LanguageOptions(), []string{"en", "es"}, true, "")
	}
}

func TestBluesoloLanguageOptions(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/api/comics/frieren", `{"comic":{"title":"Frieren","chapters":[`+
		`{"chapter":1,"title":"","language":"fr","licensed":0,"url":"/read/frieren/fr/ch/1"},`+
		`{"chapter":1,"title":"","language":"en","licensed":0,"url":"/read/frieren/en/ch/1"},`+
		`{"chapter":2,"title":"","language":"de","licensed":1,"url":"/read/frieren/de/ch/2"}]}}`)
	srv := serve(t, mux)

	cases := []struct {
		lang  string
		chaps int
	}{
		{"", 2},
		{FilterAll, 2},
		{"en", 1},
	}
	for _, c := range cases {
		b := NewBluesolo(&Grabber{URL: srv.URL + "/comics/frieren", Settings: &Settings{Language: c.lang}})
		chapters, errs := b.FetchChapters()
		if len(errs) > 0 {
			t.Fatalf("FetchChapters(%q) errors = %v", c.lang, errs)
		}
		if len(chapters) != c.chaps {
			t.Errorf("FetchChapters(%q) returned %d chapters, want %d", c.lang, len(chapters), c.chaps)
		}
		// the licensed German chapter can't be read, so it isn't offered either
		assertFilterOptions(t, "bluesolo "+c.lang, b.LanguageOptions(), []string{"fr", "en"}, true, "")
	}
}

func TestFmteamLanguageOptions(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/api/comics/batuque", `{"comic":{"title":"Batuque","chapters":[`+
		`{"chapter":1,"title":"","language":"fr","url":"/read/batuque/fr/ch/1"},`+
		`{"chapter":1,"title":"","language":"en","url":"/read/batuque/en/ch/1"}]}}`)
	srv := serve(t, mux)

	f := NewFmteam(&Grabber{URL: srv.URL + "/comics/batuque", Settings: &Settings{Language: FilterAll}})
	if _, errs := f.FetchChapters(); len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	assertFilterOptions(t, "fmteam", f.LanguageOptions(), []string{"fr", "en"}, true, "")
}

func TestMangaplusLanguageOptions(t *testing.T) {
	const englishTitleID = 100020

	spanish := []mangaplusTestChapter{{id: 201, name: "#001", subTitle: "Chapter 1: Misión"}}
	english := []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}
	languages := []mangaplusTestLanguage{
		{titleID: englishTitleID, language: 0},
		{titleID: mangaplusTestTitleID, language: 1},
	}
	pages := map[uint32][]string{101: {"https://assets.test/101/1.webp?hash=a"}}

	// the URL points at the Spanish edition
	srv := newMangaplusTestServer(t, spanish, pages)
	srv.language, srv.languages = 1, languages
	srv.editions = map[uint32]mangaplusTestEdition{englishTitleID: {chapters: english}}

	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "")
	if _, errs := m.FetchChapters(); len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	assertFilterOptions(t, "mangaplus", m.LanguageOptions(), []string{"en", "es"}, false, "es")

	// "all" is the URL's edition as an empty value is: no edition lookup
	all := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, FilterAll)
	if _, errs := all.FetchChapters(); len(errs) > 0 {
		t.Fatalf("FetchChapters(all) errors = %v", errs)
	}
	// one title_detailV3 call per grabber, and none for an edition lookup
	if calls := srv.calls("/title_detailV3"); len(calls) != 2 {
		t.Errorf("title_detailV3 was called %d times, want 2", len(calls))
	}
	assertFilterOptions(t, "mangaplus all", all.LanguageOptions(), []string{"en", "es"}, false, "es")
}

func TestMangafireLanguageAllIsEmpty(t *testing.T) {
	for _, v := range []string{"", FilterAll, " ALL "} {
		got, err := mangafireLanguage(v)
		if err != nil || got != (mangafireLang{}) {
			t.Errorf("mangafireLanguage(%q) = %+v, %v; want the zero value and no error", v, got, err)
		}
	}
}

func TestSortedValuesIsCaseInsensitiveAndNatural(t *testing.T) {
	in := []string{"es-la", "Delta", "en", "alpha", "Group 10", "Group 9", "pt-br"}
	want := []string{"alpha", "Delta", "en", "es-la", "Group 9", "Group 10", "pt-br"}
	if got := SortedValues(in); !reflect.DeepEqual(got, want) {
		t.Errorf("SortedValues() = %v, want %v", got, want)
	}
	if in[0] != "es-la" {
		t.Errorf("SortedValues() changed its input: %v", in)
	}
}

func TestSortedValuesFoldsAccents(t *testing.T) {
	want := []string{"Dawn", "Écarlate", "Fable", "Zed"}
	if got := SortedValues([]string{"Zed", "Fable", "Écarlate", "Dawn"}); !reflect.DeepEqual(got, want) {
		t.Errorf("SortedValues() = %v, want %v", got, want)
	}
}

func TestUnavailableFilterErrorListsSorted(t *testing.T) {
	err := &UnavailableFilterError{Filter: "language", Value: "zz", Available: []string{"es", "en", "ca"}}
	want := `language "zz" isn't available for this title (available: ca, en, es)`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
