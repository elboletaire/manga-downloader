// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMangaballTest(t *testing.T) {
	for _, u := range []string{
		"https://mangaball.com/title-detail/baki-6a5ffe5d90273b5b995225d2/",
		"https://mangaball.net/title-detail/baki-6a5ffe5d90273b5b995225d2/",
	} {
		ok, _ := NewMangaball(&Grabber{URL: u}).Test()
		if !ok {
			t.Errorf("Test(%q) = false, want true", u)
		}
	}
}

func TestMangaballChapterURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "chapter URL (com)",
			url:  "https://mangaball.com/chapter-detail/6a5ffe5d90273b5b995225d2",
			want: true,
		},
		{
			name: "chapter URL (net)",
			url:  "https://mangaball.net/chapter-detail/6a5ffe5d90273b5b995225d2",
			want: true,
		},
		{
			name: "www. prefix",
			url:  "https://www.mangaball.com/chapter-detail/6a5ffe5d90273b5b995225d2",
			want: true,
		},
		{
			name: "uppercase UUID",
			url:  "https://mangaball.com/chapter-detail/6A5FFE5D90273B5B995225D2",
			want: true,
		},
		{
			name: "series URL",
			url:  "https://mangaball.com/title-detail/baki-6a5ffe5d90273b5b995225d2/",
			want: false,
		},
		{
			name: "non-mangaball URL",
			url:  "https://example.com/chapter-detail/abc",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mangaballChapterURL(tt.url)
			if got != tt.want {
				t.Errorf("mangaballChapterURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestMangaballFetchTitle(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/title/6a5ffe5d90273b5b995225d2", `{"code":200,"status":"success","data":{"id":"6a5ffe5d90273b5b995225d2","name":"Baki Gaiden: Shin Chiharu","slug":"baki-gaiden-shin-chiharu"}}`)
	srv := serve(t, mux)

	old := mangaballAPI
	mangaballAPI = srv.URL
	defer func() { mangaballAPI = old }()

	title, err := NewMangaball(&Grabber{URL: "https://mangaball.com/title-detail/baki-gaiden-shin-chiharu-6a5ffe5d90273b5b995225d2/"}).FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "Baki Gaiden: Shin Chiharu" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Baki Gaiden: Shin Chiharu")
	}
}

func TestMangaballChapterDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/chapter/detail" || !strings.Contains(string(b), `"chapter_id":"6a1dcd629a5e1d32a8460eaa"`) {
			http.Error(w, "bad request", 400)
			return
		}
		w.Write([]byte(`{"status":"success","code":200,"data":{"chapter":{"id":"6a1dcd629a5e1d32a8460eaa","title_id":"685155e3702284f834178602","pages":["https://cdn.example/a-001.webp","https://cdn.example/a-002.webp"]}}}`))
	}))
	defer srv.Close()

	old := mangaballAPI
	mangaballAPI = srv.URL
	defer func() { mangaballAPI = old }()

	// a chapter URL resolves to its series through title_id
	m := NewMangaball(&Grabber{URL: "https://mangaball.net/chapter-detail/6a1dcd629a5e1d32a8460eaa"})
	if err := m.resolveSeriesURL(); err != nil {
		t.Fatalf("resolveSeriesURL() error = %v", err)
	}
	if want := "https://mangaball.com/title-detail/685155e3702284f834178602"; m.URL != want {
		t.Errorf("series URL = %q, want %q", m.URL, want)
	}

	ch, err := m.FetchChapter(&MangaballChapter{Chapter: Chapter{ID: "6a1dcd629a5e1d32a8460eaa", Number: 8}})
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if len(ch.Pages) != 2 || ch.Pages[1].URL != "https://cdn.example/a-002.webp" || ch.Pages[1].Number != 2 {
		t.Errorf("pages = %+v", ch.Pages)
	}
}

func TestMangaballResolveNetSeries(t *testing.T) {
	m := NewMangaball(&Grabber{URL: "https://www.mangaball.net/title-detail/x-685155e3702284f834178602/"})
	if err := m.resolveSeriesURL(); err != nil {
		t.Fatal(err)
	}
	if want := "https://mangaball.com/title-detail/x-685155e3702284f834178602/"; m.URL != want {
		t.Errorf("URL = %q, want %q", m.URL, want)
	}
}

// one release per id: two English releases of chapter 1 by different groups
// (the same number can come from several groups), and a Russian one
func TestMangaballFetchChaptersListsEveryRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"success","data":[
			{"id":"6a600241febf35d143d23185","lang":"en","name":"","number":1,"group_name":"Raikou"},
			{"id":"6a601952dc0a87f39f960c90","lang":"en","name":"","number":1,"group_name":"Articuno"},
			{"id":"6abb5fc522b5c96c42102004","lang":"ru","name":"","number":3,"group_name":"Raikou"}
		]}`))
	}))
	defer srv.Close()

	old := mangaballAPI
	mangaballAPI = srv.URL
	defer func() { mangaballAPI = old }()

	m := NewMangaball(&Grabber{
		URL:      "https://mangaball.com/title-detail/baki-6a5ffe5d90273b5b995225d2/",
		Settings: &Settings{Language: "en"},
	})
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want the 2 English releases", len(chapters))
	}
	wants := []struct{ id, group string }{
		{"6a600241febf35d143d23185", "Raikou"},
		{"6a601952dc0a87f39f960c90", "Articuno"},
	}
	for i, w := range wants {
		c := chapters[i].(*MangaballChapter)
		if c.ID != w.id || c.Group != w.group {
			t.Errorf("chapter[%d] ID = %q, Group = %q, want %q, %q", i, c.ID, c.Group, w.id, w.group)
		}
	}
}

func TestMangaballTitleID(t *testing.T) {
	for _, c := range []struct{ url, want string }{
		{"https://mangaball.com/title-detail/6a5ffe5d90273b5b995225d2", "6a5ffe5d90273b5b995225d2"},
		{"https://mangaball.com/title-detail/baki-6a5ffe5d90273b5b995225d2/", "6a5ffe5d90273b5b995225d2"},
	} {
		got, err := NewMangaball(&Grabber{URL: c.url}).titleID()
		if err != nil || got != c.want {
			t.Errorf("titleID(%q) = %q, %v, want %q", c.url, got, err, c.want)
		}
	}
	if _, err := NewMangaball(&Grabber{URL: "https://mangaball.com/"}).titleID(); err == nil {
		t.Error("titleID without an id: want an error")
	}
}

func TestMangaballBareTitleURLFetchTitle(t *testing.T) {
	mux := http.NewServeMux()
	respond(mux, "/title/6a5ffe5d90273b5b995225d2", `{"status":"success","data":{"name":"Baki"}}`)
	srv := serve(t, mux)

	old := mangaballAPI
	mangaballAPI = srv.URL
	defer func() { mangaballAPI = old }()

	title, err := NewMangaball(&Grabber{URL: "https://mangaball.com/title-detail/6a5ffe5d90273b5b995225d2"}).FetchTitle()
	if err != nil || title != "Baki" {
		t.Errorf("FetchTitle() = %q, %v, want Baki", title, err)
	}
}

func TestMangaballChapterDetailErrors(t *testing.T) {
	for name, body := range map[string]string{
		"status not success": `{"status":"error","code":500}`,
		"empty title_id":     `{"status":"success","data":{"chapter":{"id":"6a1dcd629a5e1d32a8460eaa","pages":[]}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(body))
			}))
			defer srv.Close()
			old := mangaballAPI
			mangaballAPI = srv.URL
			defer func() { mangaballAPI = old }()

			m := NewMangaball(&Grabber{URL: "https://mangaball.com/chapter-detail/6a1dcd629a5e1d32a8460eaa"})
			if err := m.resolveSeriesURL(); err == nil {
				t.Errorf("resolveSeriesURL() = nil error, URL %q", m.URL)
			}
		})
	}
}

func TestMangaballGetMaxConcurrency(t *testing.T) {
	for _, c := range []struct{ set, want MaxConcurrency }{
		{MaxConcurrency{Chapters: 5, Pages: 10}, mangaballMaxConcurrency},
		{MaxConcurrency{Chapters: 1, Pages: 1}, MaxConcurrency{Chapters: 1, Pages: 1}},
	} {
		m := NewMangaball(&Grabber{Settings: &Settings{MaxConcurrency: c.set}})
		if got := m.GetMaxConcurrency(); got != c.want {
			t.Errorf("GetMaxConcurrency() with %+v = %+v, want %+v", c.set, got, c.want)
		}
		if m.Settings.MaxConcurrency != c.set {
			t.Errorf("settings changed to %+v", m.Settings.MaxConcurrency)
		}
	}
}

func TestMangaballIsPagePacer(t *testing.T) {
	var site Site = NewMangaball(&Grabber{})
	if _, ok := site.(PagePacer); !ok {
		t.Fatal("*Mangaball doesn't implement PagePacer")
	}
}
