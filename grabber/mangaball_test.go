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
