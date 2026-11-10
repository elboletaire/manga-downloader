// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"testing"
)

func TestUtoonTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://www.utoon.us/en/manga/gachiakuta.html", true},
		{"https://utoon.us/en/manga/gachiakuta.html", true},
		{"https://www.utoon.net/manga/gachiakuta", false}, // hijacked/offline domain, not supported
		{"https://www.resetscans.net/en/manga/the-ultimate-of-all-ages.html", true},
		{"https://www.resetscans.net/en/the-ultimate-of-all-ages/the-ultimate-of-all-ages-chapter-267.html", true},
		{"https://mangak.io/some-manga", false},
	}

	for _, c := range cases {
		u := Utoon{Grabber: &Grabber{URL: c.url}}
		got, err := u.Test()
		if err != nil {
			t.Errorf("Test(%q) returned unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestUtoonResolveSeriesURL(t *testing.T) {
	const chapterPath = "/en/gachiakuta/gachiakuta-chapter-167.html"

	mux := http.NewServeMux()
	// Minimal chapter page with manga-series link (goquery reverses attr order on output
	// so the resolver uses Find("a.manga-series") rather than regex on raw HTML)
	respond(mux, chapterPath, `<html><body><a class="manga-series" href="https://www.utoon.us/en/manga/gachiakuta.html">Gachiakuta</a></body></html>`)
	srv := serve(t, mux)

	seriesURL, err := utoonResolveSeriesURL(srv.URL + chapterPath)
	if err != nil {
		t.Fatalf("utoonResolveSeriesURL() error = %v", err)
	}
	if want := "https://www.utoon.us/en/manga/gachiakuta.html"; seriesURL != want {
		t.Errorf("utoonResolveSeriesURL() = %q, want %q", seriesURL, want)
	}
}

func TestUtoonFetchChaptersTitles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/en/manga/the-ultimate-of-all-ages.html", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		fmt.Fprintf(w, `<html><body>
<h1 class="series-title">The Ultimate of All Ages</h1>
<script>var mangaverse_ajax = {"ajax_url":"%[1]s/wp-admin/admin-ajax.php","nonce":"abc","current_lang":"en"};</script>
<div class="chapters-list" data-category="386">
<article class="chapter-item"><a class="chapter-link" href="%[1]s/en/the-ultimate-of-all-ages/the-ultimate-of-all-ages-chapter-267.html"><h3 class="chapter-title">The Ultimate of All Ages, Chapter 267</h3></a></article>
<article class="chapter-item"><a class="chapter-link" href="%[1]s/en/the-ultimate-of-all-ages/the-ultimate-of-all-ages-chapter-266.html"><h3 class="chapter-title">the ultimate of all ages - Chapter 266</h3></a></article>
</div></body></html>`, base)
	})
	respond(mux, "/wp-admin/admin-ajax.php", `{"success":true,"data":{"html":"","has_more":false}}`)
	srv := serve(t, mux)

	u := Utoon{Grabber: &Grabber{URL: srv.URL + "/en/manga/the-ultimate-of-all-ages.html"}}
	chapters, errs := u.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	want := []string{"Chapter 267", "Chapter 266"}
	if len(chapters) != len(want) {
		t.Fatalf("FetchChapters() returned %d chapters, want %d", len(chapters), len(want))
	}
	for i, c := range chapters {
		if got := c.(*UtoonChapter).Title; got != want[i] {
			t.Errorf("chapter %d title = %q, want %q", i, got, want[i])
		}
	}
}

func TestUtoonAjaxConfigRe(t *testing.T) {
	html := `<script id="mangaverse-load-more-js-extra">
var mangaverse_ajax = {"ajax_url":"https://www.utoon.us/wp-admin/admin-ajax.php","nonce":"223204e60f","current_lang":"en"};
//# sourceURL=mangaverse-load-more-js-extra
</script>`

	m := utoonAjaxConfigRe.FindStringSubmatch(html)
	if len(m) < 3 {
		t.Fatalf("expected ajax config to be found, got %v", m)
	}
	if want := "https://www.utoon.us/wp-admin/admin-ajax.php"; m[1] != want {
		t.Errorf("ajax_url = %q, want %q", m[1], want)
	}
	if want := "223204e60f"; m[2] != want {
		t.Errorf("nonce = %q, want %q", m[2], want)
	}
}
