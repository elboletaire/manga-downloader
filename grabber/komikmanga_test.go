// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKomikmangaResolveSeriesURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/chapters/28347", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"chapter":{"id":28347,"number":"1","manga":{"slug":"Unrivaled-Spear-Demon"}}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cases := []struct {
		input string
		want  string
	}{
		{srv.URL + "/chapter/28347", srv.URL + "/manga/Unrivaled-Spear-Demon"},
		{srv.URL + "/manga/Unrivaled-Spear-Demon", srv.URL + "/manga/Unrivaled-Spear-Demon"},
	}
	for _, c := range cases {
		k := &Komikmanga{Grabber: &Grabber{URL: c.input, Settings: &Settings{}}}
		if err := k.resolveSeriesURL(); err != nil {
			t.Fatalf("resolveSeriesURL(%q): %v", c.input, err)
		}
		if k.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, k.URL, c.want)
		}
	}
}

func TestKomikmangaChapters(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/manga/Unrivaled-Spear-Demon/chapters", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"items":[{"id":63192,"number":"57"},{"id":28348,"number":"52.1"}],"next_cursor":"abc","has_more":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"id":6940,"number":"40"}],"next_cursor":"","has_more":false}`))
	})
	mux.HandleFunc("/api/v1/manga/Unrivaled-Spear-Demon", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"slug":"Unrivaled-Spear-Demon","title":"Unrivaled Spear Demon"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	k := &Komikmanga{Grabber: &Grabber{URL: srv.URL + "/manga/Unrivaled-Spear-Demon", Settings: &Settings{}}}
	title, err := k.FetchTitle()
	if err != nil || title != "Unrivaled Spear Demon" {
		t.Fatalf("FetchTitle() = %q, %v", title, err)
	}

	chapters, errs := k.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors: %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3 across both pages of the cursor", len(chapters))
	}
	if c := chapters[1].(*KomikmangaChapter); c.ID != "28348" || c.Number != 52.1 || ChapterLabel(c) != "Chapter 52.1" {
		t.Errorf("chapter 52.1 = {ID %q, Number %v, Title %q}", c.ID, c.Number, ChapterLabel(c))
	}
}
