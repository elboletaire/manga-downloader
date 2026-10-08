// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	sacachispaTestManga   = "3ba3f7c1-4655-488f-87c5-bf1d797007c1"
	sacachispaTestRelease = "011b647a-fc27-5696-b823-986a3f5a98d4"
)

// sacachispaTestServer mimics the slice of api.sacachispa.site the grabber uses:
// a 2-page chapter list (so pagination gets exercised), a free chapter with two
// releases, a Patreon one that 403s, and the reader's release lookup
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
			body = `{"data":[{"id":"c3","chapter":"23","title":null,"patreonOnly":true},{"id":"cx","chapter":"oneshot","title":null,"patreonOnly":false}],"pagination":{"pages":2}}`
		}
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/chapters/c1/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"r-es","language":"es"},{"id":"r-en","language":"en"}]}`))
	})
	mux.HandleFunc("/chapters/c3/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"r-pat","language":"en"}]}`))
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
	// both API pages are read; the non-numeric chapter is dropped
	if len(chapters) != 3 {
		t.Fatalf("FetchChapters() got %d chapters, want 3", len(chapters))
	}
	if chapters[1].GetNumber() != 22.5 || chapters[1].GetTitle() != "Named" {
		t.Errorf("chapters[1] = %v %q, unexpected", chapters[1].GetNumber(), chapters[1].GetTitle())
	}
	if chapters[0].GetTitle() != "Chapter 1" {
		t.Errorf("chapters[0] title = %q, want the \"Chapter N\" fallback", chapters[0].GetTitle())
	}
}

func TestSacachispaFetchChapter(t *testing.T) {
	sacachispaTestServer(t)
	s := newTestSacachispa("https://sacachispa.site/manga/" + sacachispaTestManga + "/some-series")
	s.Settings.Language = "en"

	chapters, _ := s.FetchChapters()

	// the English release is picked over the first-listed Spanish one
	ch, err := s.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if ch.PagesCount != 2 || len(ch.Pages) != 2 || ch.Pages[1].URL != "https://cdn.example/b.jpg" || ch.Pages[1].Number != 2 {
		t.Errorf("FetchChapter() = %+v, unexpected", ch)
	}

	// a Patreon-only chapter 403s and must say why
	_, err = s.FetchChapter(chapters[2])
	if err == nil || !strings.Contains(err.Error(), "Patreon") {
		t.Errorf("FetchChapter() on a Patreon chapter error = %v, want a Patreon hint", err)
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
