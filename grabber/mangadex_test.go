// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMangadexChapterURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "chapter URL",
			url:  "https://mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79",
			want: true,
		},
		{
			name: "chapter URL with query",
			url:  "https://mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79?page=5",
			want: true,
		},
		{
			name: "www. prefix (308 to apex)",
			url:  "https://www.mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79",
			want: true,
		},
		{
			name: "uppercase UUID",
			url:  "https://mangadex.org/chapter/C780A907-8CF7-4D64-97FD-750519F8FF79",
			want: true,
		},
		{
			name: "series URL",
			url:  "https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece",
			want: false,
		},
		{
			name: "non-mangadex URL",
			url:  "https://example.com/chapter/123",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mangadexChapterURL(tt.url)
			if got != tt.want {
				t.Errorf("mangadexChapterURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// The site caps the configured concurrency: lower settings are kept, higher
// ones (the defaults included) are clamped to what the image servers take
func TestMangadexGetMaxConcurrency(t *testing.T) {
	cases := []struct {
		set, want MaxConcurrency
	}{
		{MaxConcurrency{Chapters: 5, Pages: 10}, mangadexMaxConcurrency},
		{MaxConcurrency{Chapters: 1, Pages: 2}, MaxConcurrency{Chapters: 1, Pages: 2}},
		{MaxConcurrency{Chapters: 1, Pages: 10}, MaxConcurrency{Chapters: 1, Pages: mangadexMaxConcurrency.Pages}},
	}
	for _, c := range cases {
		m := NewMangadex(&Grabber{Settings: &Settings{MaxConcurrency: c.set}})
		if got := m.GetMaxConcurrency(); got != c.want {
			t.Errorf("GetMaxConcurrency() with %+v = %+v, want %+v", c.set, got, c.want)
		}
		// the settings themselves are left alone
		if m.Settings.MaxConcurrency != c.set {
			t.Errorf("settings changed to %+v", m.Settings.MaxConcurrency)
		}
	}
}

// Mangadex paces its pages, through the interface the downloader looks for
func TestMangadexIsPagePacer(t *testing.T) {
	var site Site = NewMangadex(&Grabber{Settings: &Settings{}})
	if _, ok := site.(PagePacer); !ok {
		t.Fatal("*Mangadex doesn't implement PagePacer")
	}
}

// The pacer spaces its slots one interval apart, even for concurrent callers
func TestPacerSpacesSlots(t *testing.T) {
	p := &pacer{interval: 20 * time.Millisecond}
	const n = 6
	var mu sync.Mutex
	var at []time.Time
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.wait()
			mu.Lock()
			at = append(at, time.Now())
			mu.Unlock()
		}()
	}
	wg.Wait()
	slices.SortFunc(at, func(a, b time.Time) int { return a.Compare(b) })
	// n slots take (n-1) intervals; allow a little scheduling slack
	if span := at[n-1].Sub(at[0]); span < (n-1)*p.interval-5*time.Millisecond {
		t.Errorf("%d slots spanned %v, want at least %v", n, span, (n-1)*p.interval)
	}
}

// Each group's release of a chapter keeps its own id, from the listing to the
// chapter FetchChapter returns
func TestMangadexChapterIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/feed"):
			if r.URL.Query().Get("offset") != "0" {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[
				{"id":"rel-a","attributes":{"chapter":"1","title":"Same","translatedLanguage":"en","pages":2}},
				{"id":"rel-b","attributes":{"chapter":"1","title":"Same","translatedLanguage":"en","pages":2}}]}`))
		case strings.HasPrefix(r.URL.Path, "/at-home/server/"):
			_, _ = w.Write([]byte(`{"baseUrl":"https://cdn.test","chapter":{"hash":"h","data":["1.png","2.png"]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := mangadexAPI
	mangadexAPI = srv.URL
	defer func() { mangadexAPI = old }()

	m := NewMangadex(&Grabber{URL: "https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f", Settings: &Settings{Language: "en"}})
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters = %d chapters, errs %v", len(chapters), errs)
	}
	for i, want := range []string{"rel-a", "rel-b"} {
		got, err := m.FetchChapter(chapters[i])
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != want {
			t.Errorf("chapter %d ID = %q, want %q", i, got.ID, want)
		}
	}
}

// A release names its scanlation groups from the feed's relationships, several
// joined with ", "; an upload by a user alone names none. FetchChapter keeps
// the group with the release it downloads.
func TestMangadexChapterGroups(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/feed"):
			if r.URL.Query().Get("offset") != "0" {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			if !slices.Contains(r.URL.Query()["includes[]"], "scanlation_group") {
				http.Error(w, "groups not included", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"data":[
				{"id":"rel-a","attributes":{"chapter":"1","title":"","translatedLanguage":"en","pages":2},
					"relationships":[{"id":"g1","type":"scanlation_group","attributes":{"name":"A"}},
						{"id":"g2","type":"scanlation_group","attributes":{"name":"B"}},
						{"id":"m","type":"manga"},{"id":"u","type":"user","attributes":{"username":"up"}}]},
				{"id":"rel-b","attributes":{"chapter":"2","title":"","translatedLanguage":"en","pages":2},
					"relationships":[{"id":"u","type":"user","attributes":{"username":"up"}},{"id":"m","type":"manga"}]},
				{"id":"rel-c","attributes":{"chapter":"3","title":"","translatedLanguage":"en","pages":2},
					"relationships":[{"id":"g1","type":"scanlation_group","attributes":{"name":"A"}}]}]}`))
		case strings.HasPrefix(r.URL.Path, "/at-home/server/"):
			_, _ = w.Write([]byte(`{"baseUrl":"https://cdn.test","chapter":{"hash":"h","data":["1.png","2.png"]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := mangadexAPI
	mangadexAPI = srv.URL
	defer func() { mangadexAPI = old }()

	m := NewMangadex(&Grabber{URL: "https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f", Settings: &Settings{Language: "en"}})
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 3 {
		t.Fatalf("FetchChapters = %d chapters, errs %v", len(chapters), errs)
	}
	wantGroups := []string{"A, B", "", "A"}
	for i, want := range wantGroups {
		c := chapters[i].(*MangadexChapter)
		if c.Group != want {
			t.Errorf("chapter %d Group = %q, want %q", i, c.Group, want)
		}
	}
	if c := chapters[0].(*MangadexChapter); c.ID != "rel-a" {
		t.Errorf("chapter 0 ID = %q, want rel-a", c.ID)
	}

	got, err := m.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "rel-a" || got.Group != "A, B" {
		t.Errorf("FetchChapter ID, Group = %q, %q, want rel-a, \"A, B\"", got.ID, got.Group)
	}
}

// mangadexExternalServer serves a manga offering en (external only), es
// (mixed) and fr (plain), honouring the feed's language filter.
func mangadexExternalServer(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/feed"):
			if r.URL.Query().Get("offset") != "0" {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			all := []string{
				`{"id":"e1","attributes":{"chapter":"1","translatedLanguage":"en","pages":0,"externalUrl":"https://mangaplus.shueisha.co.jp/viewer/1"}}`,
				`{"id":"s1","attributes":{"chapter":"1","translatedLanguage":"es","pages":0,"externalUrl":"https://example.test/x"}}`,
				`{"id":"s2","attributes":{"chapter":"2","translatedLanguage":"es","pages":3}}`,
				`{"id":"f1","attributes":{"chapter":"1","translatedLanguage":"fr","pages":3}}`,
			}
			var out []string
			langs := r.URL.Query()["translatedLanguage[]"]
			for _, c := range all {
				if len(langs) == 0 || slices.ContainsFunc(langs, func(l string) bool { return strings.Contains(c, `"`+l+`"`) }) {
					out = append(out, c)
				}
			}
			_, _ = w.Write([]byte(`{"data":[` + strings.Join(out, ",") + `]}`))
		case strings.Contains(r.URL.Path, "/manga/"):
			_, _ = w.Write([]byte(`{"data":{"attributes":{"title":{"en":"T"},"availableTranslatedLanguages":["en","es","fr"]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := mangadexAPI
	mangadexAPI = srv.URL
	t.Cleanup(func() { mangadexAPI = old })
}

func newMangadexExternal(lang string) *Mangadex {
	return NewMangadex(&Grabber{URL: "https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f", Settings: &Settings{Language: lang}})
}

// A language whose chapters are all external gets a clear error, not the
// generic "no chapters" one
func TestMangadexExternalOnlyLanguage(t *testing.T) {
	mangadexExternalServer(t)
	m := newMangadexExternal("en")
	chapters, errs := m.FetchChapters()
	if len(chapters) != 0 || len(errs) != 1 {
		t.Fatalf("FetchChapters = %d chapters, errs %v", len(chapters), errs)
	}
	for _, want := range []string{"hosted on MANGA Plus", "try https://mangaplus.shueisha.co.jp/viewer/1"} {
		if !strings.Contains(errs[0].Error(), want) {
			t.Errorf("error %q lacks %q", errs[0], want)
		}
	}
	// the filtered listing only rules out its own language
	if got := m.LanguageOptions().Values; !slices.Equal(got, []string{"es", "fr"}) {
		t.Errorf("filtered LanguageOptions = %v, want [es fr]", got)
	}
}

// A mixed language still lists its readable chapters and is offered
func TestMangadexExternalMixedLanguage(t *testing.T) {
	mangadexExternalServer(t)
	m := newMangadexExternal("es")
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 1 {
		t.Fatalf("FetchChapters = %d chapters, errs %v", len(chapters), errs)
	}
	// a filtered listing can't vouch for the other languages
	if got := m.LanguageOptions().Values; !slices.Equal(got, []string{"en", "es", "fr"}) {
		t.Errorf("LanguageOptions = %v, want [en es fr]", got)
	}
}

// An all-languages listing drops the external-only languages from the options
func TestMangadexExternalUnfilteredOptions(t *testing.T) {
	mangadexExternalServer(t)
	m := newMangadexExternal("")
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 || len(chapters) != 2 {
		t.Fatalf("FetchChapters = %d chapters, errs %v", len(chapters), errs)
	}
	if got := m.LanguageOptions().Values; !slices.Equal(got, []string{"es", "fr"}) {
		t.Errorf("LanguageOptions = %v, want [es fr]", got)
	}
}

// Before any listing, the manga's own languages are offered
func TestMangadexLanguageOptionsBeforeListing(t *testing.T) {
	mangadexExternalServer(t)
	m := newMangadexExternal("")
	if got := m.LanguageOptions().Values; !slices.Equal(got, []string{"en", "es", "fr"}) {
		t.Errorf("LanguageOptions = %v, want [en es fr]", got)
	}
}

// Every chapter external with no language filter: the error drops the language
func TestMangadexExternalAllLanguages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"e","attributes":{"chapter":"1","translatedLanguage":"en","pages":0,"externalUrl":"https://example.test/x"}}]}`))
	}))
	defer srv.Close()
	old := mangadexAPI
	mangadexAPI = srv.URL
	defer func() { mangadexAPI = old }()

	_, errs := newMangadexExternal("all").FetchChapters()
	want := "this title's chapters are hosted elsewhere: https://example.test/x"
	if len(errs) != 1 || errs[0].Error() != want {
		t.Errorf("errs = %v, want %q", errs, want)
	}
}

// A listing that failed on a later page prunes nothing
func TestMangadexPartialListingKeepsLanguages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/feed"):
			if r.URL.Query().Get("offset") != "0" {
				http.Error(w, "boom", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"f1","attributes":{"chapter":"1","translatedLanguage":"fr","pages":3}}]}`))
		case strings.Contains(r.URL.Path, "/manga/"):
			_, _ = w.Write([]byte(`{"data":{"attributes":{"title":{"en":"T"},"availableTranslatedLanguages":["en","fr"]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := mangadexAPI
	mangadexAPI = srv.URL
	defer func() { mangadexAPI = old }()

	m := newMangadexExternal("")
	if _, errs := m.FetchChapters(); len(errs) == 0 {
		t.Fatal("want an error from the failing page")
	}
	if got := m.LanguageOptions().Values; !slices.Equal(got, []string{"en", "fr"}) {
		t.Errorf("LanguageOptions = %v, want [en fr]", got)
	}
}
