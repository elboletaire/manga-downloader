// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const (
	sacachispaTestManga   = "3ba3f7c1-4655-488f-87c5-bf1d797007c1"
	sacachispaTestRelease = "011b647a-fc27-5696-b823-986a3f5a98d4"
)

// sacachispaTestServer mimics the api.sacachispa.site slice the grabber uses, with
// two-page chapter and release lists. c1 has two releases (es by Alpha, en by Gamma),
// c2 one, c3 a Patreon one whose pages 403, and c4 has none.
func sacachispaTestServer(t *testing.T) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/manga/"+sacachispaTestManga, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"title":"Some \"Quoted\" Series"}}`))
	})
	mux.HandleFunc("/chapters", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mangaId") != sacachispaTestManga {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		body := `{"data":[{"id":"c1","chapter":"1","title":null,"patreonOnly":false},{"id":"c2","chapter":"22.5","title":"Named","patreonOnly":false}],"pagination":{"pages":2}}`
		if r.URL.Query().Get("page") == "2" {
			body = `{"data":[{"id":"c3","chapter":"23","title":null,"patreonOnly":true},{"id":"c4","chapter":"24","title":null,"patreonOnly":true},{"id":"cx","chapter":"oneshot","title":null,"patreonOnly":false}],"pagination":{"pages":2}}`
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mangaId") != sacachispaTestManga {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		body := `{"data":[` +
			`{"id":"r-es","chapterId":"c1","language":"es","group":{"id":"g1","name":"Alpha"}},` +
			`{"id":"r-c2","chapterId":"c2","language":"en","group":{"id":"g2","name":"Beta"}}` +
			`],"pagination":{"pages":2}}`
		if r.URL.Query().Get("page") == "2" {
			body = `{"data":[` +
				`{"id":"r-en","chapterId":"c1","language":"en","group":{"id":"g3","name":"Gamma"}},` +
				`{"id":"r-pat","chapterId":"c3","language":"en","group":{"id":"g2","name":"Beta"}}` +
				`],"pagination":{"pages":2}}`
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/releases/r-en/pages", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"items":[{"page":1,"url":"https://cdn.example/a.jpg"},{"page":2,"url":"https://cdn.example/b.jpg"}]}}`))
	})
	mux.HandleFunc("/releases/r-pat/pages", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"success":false}`, http.StatusForbidden)
	})
	mux.HandleFunc("/releases/"+sacachispaTestRelease, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"manga":{"id":"` + sacachispaTestManga + `"}}}`))
	})

	srv := httptest.NewServer(mux)
	old := sacachispaAPI
	sacachispaAPI = srv.URL
	t.Cleanup(func() {
		srv.Close()
		sacachispaAPI = old
	})
}

func newTestSacachispa(url string) *Sacachispa {
	return NewSacachispa(&Grabber{URL: url, Settings: &Settings{}})
}

func TestSacachispaTitleAndChapters(t *testing.T) {
	sacachispaTestServer(t)
	s := newTestSacachispa("https://sacachispa.site/manga/" + sacachispaTestManga + "/some-series")

	title, err := s.FetchTitle()
	if err != nil || title != `Some "Quoted" Series` {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := s.FetchChapters()
	if len(errs) != 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	// both API pages are read, one chapter per release (c1 has two, the
	// Patreon c4 none, so it's listed once anyway); the non-numeric one is dropped
	type release struct{ id, group string }
	var got []release
	for _, c := range chapters {
		got = append(got, release{c.(*SacachispaChapter).ID, c.(*SacachispaChapter).Group})
	}
	want := []release{{"r-es", "Alpha"}, {"r-en", "Gamma"}, {"r-c2", "Beta"}, {"r-pat", "Beta"}, {"", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FetchChapters() releases = %v, want %v", got, want)
	}
	if chapters[2].GetNumber() != 22.5 || chapters[2].GetTitle() != "Named" {
		t.Errorf("chapters[2] = %v %q, unexpected", chapters[2].GetNumber(), chapters[2].GetTitle())
	}
	if ChapterLabel(chapters[0]) != "Chapter 1" {
		t.Errorf("chapters[0] title = %q, want the \"Chapter N\" fallback", ChapterLabel(chapters[0]))
	}
}

func TestSacachispaFetchChapter(t *testing.T) {
	sacachispaTestServer(t)
	s := newTestSacachispa("https://sacachispa.site/manga/" + sacachispaTestManga + "/some-series")
	s.Settings.Language = "en"

	// the English releases only: c1 is Gamma's, the Spanish one isn't listed
	chapters, _ := s.FetchChapters()
	if len(chapters) != 4 {
		t.Fatalf("FetchChapters() got %d chapters, want 4", len(chapters))
	}

	ch, err := s.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if ch.PagesCount != 2 || len(ch.Pages) != 2 || ch.Pages[1].URL != "https://cdn.example/b.jpg" || ch.Pages[1].Number != 2 {
		t.Errorf("FetchChapter() = %+v, unexpected", ch)
	}
	if ch.ID != "r-en" || ch.Group != "Gamma" {
		t.Errorf("FetchChapter() ID %q, Group %q, want r-en, Gamma", ch.ID, ch.Group)
	}

	// a Patreon-only release 403s and must say why
	_, err = s.FetchChapter(chapters[2])
	if err == nil || !strings.Contains(err.Error(), "Patreon") {
		t.Errorf("FetchChapter() on a Patreon release error = %v, want a Patreon hint", err)
	}

	// a Patreon-only chapter the listing has no release of says so too
	_, err = s.FetchChapter(chapters[3])
	if err == nil || !strings.Contains(err.Error(), "Patreon") {
		t.Errorf("FetchChapter() on a release-less Patreon chapter error = %v, want a Patreon hint", err)
	}
}

func TestSacachispaFetchChaptersPerLanguage(t *testing.T) {
	sacachispaTestServer(t)

	// no language asked for: every release of c1 is listed
	s := newTestSacachispa("https://sacachispa.site/manga/" + sacachispaTestManga + "/some-series")
	chapters, _ := s.FetchChapters()
	if len(chapters) != 5 {
		t.Fatalf("FetchChapters() with no language got %d chapters, want 5", len(chapters))
	}

	// Spanish asked for: c1 keeps its Spanish release, and c2, which has no
	// Spanish one, keeps its only (English) release
	s = newTestSacachispa("https://sacachispa.site/manga/" + sacachispaTestManga + "/some-series")
	s.Settings.Language = "es"
	chapters, _ = s.FetchChapters()
	var ids []string
	for _, c := range chapters {
		ids = append(ids, c.(*SacachispaChapter).ID)
	}
	if want := []string{"r-es", "r-c2", "r-pat", ""}; !reflect.DeepEqual(ids, want) {
		t.Errorf("FetchChapters() in es = %v, want %v", ids, want)
	}
}

func TestSacachispaResolveMangaID(t *testing.T) {
	sacachispaTestServer(t)

	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://sacachispa.site/manga/" + sacachispaTestManga + "/some-series", sacachispaTestManga, false},
		// a reader URL is resolved through its release
		{"https://sacachispa.site/read/" + sacachispaTestRelease, sacachispaTestManga, false},
		// the old site's URLs no longer exist
		{"https://sacachispa.site/series/some-series", "", true},
	}

	for _, c := range cases {
		got, err := newTestSacachispa(c.url).resolveMangaID()
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("resolveMangaID(%q) = %q, %v; want %q (error: %v)", c.url, got, err, c.want, c.wantErr)
		}
	}
}

func TestSacachispaTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://sacachispa.site/manga/" + sacachispaTestManga + "/some-series", true},
		{"https://sacachispa.site/read/" + sacachispaTestRelease, true},
		{"https://example.com/manga/" + sacachispaTestManga + "/some-series", false},
	}

	for _, c := range cases {
		got, err := newTestSacachispa(c.url).Test()
		if err != nil || got != c.want {
			t.Errorf("Test(%q) = %v, %v; want %v", c.url, got, err, c.want)
		}
	}
}
