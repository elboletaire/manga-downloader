// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

const (
	globalcomixFreeKey = "6d7935c9-a32e-49e3-944b-dd0813719b32"
	globalcomixPaidKey = "59cd868b-094b-47fb-9c04-76fff5306dda"
	// globalcomixTestKey is the api_key in testdata/globalcomix/page.html, not
	// the fallback constant
	globalcomixTestKey = "gck_fromthepage123"
)

// newTestGlobalcomix serves the trimmed fixtures as the API, and returns a
// grabber for the given URL
func newTestGlobalcomix(t *testing.T, pageURL string, settings *Settings) *Globalcomix {
	t.Helper()

	files := map[string]string{
		"/v1/read/acts-one-nine":           "comic.json",
		"/v1/read/dceased-2019-":           "comic.json",
		"/v1/comics/7312/releases":         "releases-free.json",
		"/v1/readV3/" + globalcomixFreeKey: "read-free.json",
		"/v1/readV3/" + globalcomixPaidKey: "read-paid.json",
		"/v1/comics/18240/releases":        "releases-paid.json",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/c/") || strings.HasPrefix(r.URL.Path, "/read/") {
			b, _ := os.ReadFile("testdata/globalcomix/page.html")
			w.Write(b)
			return
		}
		if r.Header.Get("X-Gc-Client") != globalcomixTestKey {
			http.Error(w, `{"error":"CLIENT_HEADER_MISSING"}`, http.StatusBadRequest)
			return
		}
		name, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile("testdata/globalcomix/" + name)
		if err != nil {
			t.Error(err)
		}
		s := string(b)
		if r.URL.Path == "/v1/read/dceased-2019-" {
			s = strings.NewReplacer(`"id": 7312`, `"id": 18240`, `"slug": "acts-one-nine"`, `"slug": "dceased-2019-"`).Replace(s)
		}
		http.SetCookie(w, &http.Cookie{Name: globalcomixAuthCookie, Value: "jwt-" + r.URL.Path})
		w.Write([]byte(s))
	}))
	t.Cleanup(srv.Close)

	old := globalcomixAPI
	globalcomixAPI = srv.URL
	t.Cleanup(func() { globalcomixAPI = old })

	if settings == nil {
		settings = &Settings{}
	}

	// the page is read off the same server, so the path is all that matters
	u, _ := url.Parse(pageURL)
	pageURL = srv.URL + u.EscapedPath() + "?" + u.RawQuery

	return NewGlobalcomix(&Grabber{URL: pageURL, Settings: settings})
}

func TestGlobalcomixTest(t *testing.T) {
	for url, want := range map[string]bool{
		"https://globalcomix.com/c/acts-one-nine":            true,
		"https://www.globalcomix.com/read/abc":               true,
		"https://globalcomix.com":                            true,
		"https://notglobalcomix.com/c/x":                     false,
		"https://example.com/?u=https://globalcomix.com/c/x": false,
	} {
		got, _ := (&Globalcomix{Grabber: &Grabber{URL: url}}).Test()
		if got != want {
			t.Errorf("Test(%q) = %v, want %v", url, got, want)
		}
	}
}

// series, issue, reader and ?lang_id links all load the same series
func TestGlobalcomixResolvesSeries(t *testing.T) {
	for _, url := range []string{
		"https://globalcomix.com/c/acts-one-nine",
		"https://globalcomix.com/c/acts-one-nine?lang_id=en",
		"https://globalcomix.com/c/acts-one-nine/r/" + globalcomixFreeKey,
		"https://globalcomix.com/read/" + globalcomixFreeKey + "/1",
	} {
		g := newTestGlobalcomix(t, url, nil)
		title, err := g.FetchTitle()
		if err != nil || title != "ACTS ONE NINE" {
			t.Errorf("%s: title = %q, err = %v", url, title, err)
		}
	}
}

func TestGlobalcomixFetchChaptersFree(t *testing.T) {
	g := newTestGlobalcomix(t, "https://globalcomix.com/c/acts-one-nine", nil)
	chapters, errs := g.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(chapters))
	}
	c := chapters[1].(*GlobalcomixChapter)
	if c.Number != 2 || c.Title != "TWO: Cooking with Ricin" || c.Language != "en" || LockedOf(c) {
		t.Errorf("unexpected chapter %+v", c.Chapter)
	}
}

// a paid series lists its issues as locked, without its collected editions
func TestGlobalcomixFetchChaptersPaidAreLocked(t *testing.T) {
	g := newTestGlobalcomix(t, "https://globalcomix.com/c/dceased-2019-", nil)
	chapters, errs := g.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want the 2 issues", len(chapters))
	}
	for _, c := range chapters {
		if !LockedOf(c) {
			t.Errorf("%s should be locked", c.GetTitle())
		}
	}
}

func TestGlobalcomixFetchChapterPages(t *testing.T) {
	g := newTestGlobalcomix(t, "https://globalcomix.com/c/acts-one-nine", nil)
	chapter, err := g.FetchChapter(&GlobalcomixChapter{Chapter: Chapter{Number: 1}, Key: globalcomixFreeKey})
	if err != nil {
		t.Fatal(err)
	}
	if chapter.PagesCount != 53 || len(chapter.Pages) != 53 {
		t.Fatalf("pages = %d/%d, want 53", chapter.PagesCount, len(chapter.Pages))
	}
	p := chapter.Pages[1]
	if want := "https://reader-cdn.globalcomix.com/r/" + globalcomixFreeKey + "/p/2/desktop.webp"; p.URL != want {
		t.Errorf("page URL = %q, want %q", p.URL, want)
	}
	if want := "gc_reader_auth=jwt-/v1/readV3/" + globalcomixFreeKey; p.Headers["Cookie"] != want {
		t.Errorf("cookie = %q, want %q", p.Headers["Cookie"], want)
	}
}

// a paid issue's preview pages are never returned as the issue
func TestGlobalcomixFetchChapterPaidIsRefused(t *testing.T) {
	g := newTestGlobalcomix(t, "https://globalcomix.com/c/dceased-2019-", nil)
	chapter, err := g.FetchChapter(&GlobalcomixChapter{Chapter: Chapter{Number: 1}, Key: globalcomixPaidKey})
	if err == nil || chapter != nil || !strings.Contains(err.Error(), "paid") {
		t.Errorf("chapter = %v, err = %v, want a paid error", chapter, err)
	}
}

func TestGlobalcomixLanguages(t *testing.T) {
	g := newTestGlobalcomix(t, "https://globalcomix.com/c/acts-one-nine?lang_id=es", nil)
	comic, _ := g.fetchComic()
	if got := g.languages(comic); len(got) != 1 || got[0] != "es" {
		t.Errorf("url language = %v, want [es]", got)
	}
	g.Settings.Language = FilterAll
	if got := g.languages(comic); len(got) != 2 {
		t.Errorf("all = %v, want en and es", got)
	}
	if o := g.LanguageOptions(); o.Current != "" || len(o.Values) != 2 {
		t.Errorf("options = %+v", o)
	}
}

// the language that isn't offered is the same error the other language listers return
func TestGlobalcomixUnknownLanguage(t *testing.T) {
	g := newTestGlobalcomix(t, "https://globalcomix.com/c/acts-one-nine", &Settings{Language: "fr"})
	_, errs := g.FetchChapters()
	var unavailable *UnavailableFilterError
	if len(errs) != 1 || !errors.As(errs[0], &unavailable) || unavailable.Filter != "language" || len(unavailable.Available) != 2 {
		t.Errorf("errs = %v, want an UnavailableFilterError listing en and es", errs)
	}
}

func TestGlobalcomixCurrentLanguage(t *testing.T) {
	for _, tc := range []struct{ url, language, want string }{
		{"https://globalcomix.com/c/acts-one-nine", "", "en"},
		{"https://globalcomix.com/c/acts-one-nine?lang_id=es", "", "es"},
		{"https://globalcomix.com/c/acts-one-nine?lang_id=es", "en", "en"},
		{"https://globalcomix.com/c/acts-one-nine", FilterAll, ""},
	} {
		g := newTestGlobalcomix(t, tc.url, &Settings{Language: tc.language})
		if got := g.LanguageOptions().Current; got != tc.want {
			t.Errorf("%s %q: current = %q, want %q", tc.url, tc.language, got, tc.want)
		}
	}
}

// numbered collected editions are dropped, unnumbered ones kept without
// taking an issue's number
func TestGlobalcomixChaptersUnnumbered(t *testing.T) {
	releases := []globalcomixRelease{
		{Key: "k3", ChapterType: "c", Chapter: "2", Order: 3, IsFree: true},
		{Key: "k1", ChapterType: "c", Chapter: "1", Order: 1, IsFree: true},
		{Key: "k4", ChapterType: "v", Chapter: "1", Order: 4, IsFree: true},
		{Key: "k2", ChapterType: "s", Chapter: "", Order: 2, Title: "Bonus", IsFree: true},
		{Key: "k5", ChapterType: "o", Chapter: "", Order: 5, Title: "One-shot", IsFree: true},
	}
	var got []string
	for _, c := range globalcomixChapters(releases, "en") {
		got = append(got, c.(*GlobalcomixChapter).Key+"="+strconv.FormatFloat(c.GetNumber(), 'f', -1, 64))
	}
	if want := "k1=1,k2=1.01,k3=2,k5=2.01"; strings.Join(got, ",") != want {
		t.Errorf("chapters = %v, want %s", got, want)
	}
}
