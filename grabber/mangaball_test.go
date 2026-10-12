// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
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
