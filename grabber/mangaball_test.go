// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"reflect"
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
	const series = "/title-detail/baki-6a5ffe5d90273b5b995225d2/"

	mux := http.NewServeMux()
	respond(mux, series, `<html><body><script>self.__next_f.push([1,"1a:[[\"$\",\"$L24\",null,{\"initialTitleData\":{\"id\":\"6a5ffe5d90273b5b995225d2\",\"name\":\"Baki Gaiden: Shin Chiharu\",\"slug\":\"baki-gaiden-shin-chiharu\"}}]]\n"])</script></body></html>`)
	srv := serve(t, mux)

	title, err := NewMangaball(&Grabber{URL: srv.URL + series}).FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "Baki Gaiden: Shin Chiharu" {
		t.Errorf("FetchTitle() = %q, want %q", title, "Baki Gaiden: Shin Chiharu")
	}
}

func TestMangaballSeriesURL(t *testing.T) {
	// Actual JSON-LD from the chapter page: name comes before url
	html := `<script type="application/ld+json">{"@type":"ComicStory","isPartOf":{"@type":"ComicSeries","name":"Baki Gaiden: Shin Chiharu","url":"https://mangaball.com/title-detail/6a5ffe5d90273b5b995225d2"}}</script>`

	got, ok := mangaballSeriesURL(html)
	if !ok {
		t.Fatal("mangaballSeriesURL: no match")
	}
	if want := "https://mangaball.com/title-detail/6a5ffe5d90273b5b995225d2"; got != want {
		t.Errorf("mangaballSeriesURL() = %q, want %q", got, want)
	}
}

func TestMangaballChapterImages(t *testing.T) {
	page := `<html><body><script>self.__next_f.push([1,"1a:[[\"$\",\"$L24\",null,{\"initialChapter\":{\"id\":\"x\",\"pages\":[\"https://cdn.example/x-001.png\",\"https://cdn.example/x-002.png\"]}}]]\n"])</script></body></html>`

	images, err := mangaballChapterImages(page)
	if err != nil {
		t.Fatalf("mangaballChapterImages() error = %v", err)
	}
	want := []string{"https://cdn.example/x-001.png", "https://cdn.example/x-002.png"}
	if !reflect.DeepEqual(images, want) {
		t.Errorf("mangaballChapterImages() = %v, want %v", images, want)
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
