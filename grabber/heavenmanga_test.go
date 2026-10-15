// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeavenMangaTest(t *testing.T) {
	cases := map[string]bool{
		"https://heavenmanga.com/manga/black-clover":     true,
		"https://www.heavenmanga.com/manga/black-clover": true,
		"https://heavenmanga.com/manga/leer/633695":      true,
		"https://example.com/manga/black-clover":         false,
	}
	for url, want := range cases {
		got, err := NewHeavenManga(&Grabber{URL: url}).Test()
		if err != nil {
			t.Fatalf("Test(%q) error = %v", url, err)
		}
		if got != want {
			t.Errorf("Test(%q) = %v, want %v", url, got, want)
		}
	}
}

func TestHeavenMangaSlug(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://heavenmanga.com/manga/black-clover", "black-clover"},
		{"https://heavenmanga.com/manga/black-clover/", "black-clover"},
		{"https://heavenmanga.com/manga/cambiarias-tu-novia/34", "cambiarias-tu-novia"},
		{"https://heavenmanga.com/manga/black-clover?x=1", "black-clover"},
		// reader pages are not series
		{"https://heavenmanga.com/manga/leer/633695", ""},
		{"https://heavenmanga.com/adulto", ""},
	}
	for _, c := range cases {
		if got := heavenMangaSlug(c.url); got != c.want {
			t.Errorf("heavenMangaSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

// heavenMangaTestServer answers the series table with rows, and reader pages
// with a reader script listing their images
func heavenMangaTestServer(t *testing.T, rows []heavenMangaRow, readers map[string]string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manga/black-clover" && r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
			if r.URL.Query().Get("length") != "-1" {
				http.Error(w, "expected every row in one request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"draw":            1,
				"recordsTotal":    len(rows),
				"recordsFiltered": len(rows),
				"data":            rows,
			})
			return
		}
		if body, ok := readers[r.URL.Path]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv
}

// heavenMangaRowHTML is the panel DataTables gets for a release: the
// chapter's number, the reader link and the uploader's label
func heavenMangaRowHTML(number, label string) string {
	return `<div class="panel panel-default c_panel"><h3 class="h-inherit"> ` + number + ` </h3>` +
		`<a href="https://heavenmanga.com/manga/leer/1"><i>Leer</i></a>` +
		`<span class="pull-right greenSpan">` + label + `</span></div>`
}

func TestHeavenMangaFetchChapters(t *testing.T) {
	rows := []heavenMangaRow{
		{ID: 10, Number: heavenMangaRowHTML("Capítulo 2", "Ronin Fansub")},
		{ID: 11, Number: heavenMangaRowHTML("Capítulo 2", "La Quinta Hoja Scanlation")},
		{ID: 12, Number: heavenMangaRowHTML("Capítulo 1", "Solo Subida")},
		// a row with no chapter number is skipped
		{ID: 13, Number: heavenMangaRowHTML("Extra", "Ronin Fansub")},
	}
	srv := heavenMangaTestServer(t, rows, nil)

	m := NewHeavenManga(&Grabber{URL: srv.URL + "/manga/black-clover"})
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d chapters, want 3", len(chapters))
	}

	want := []struct {
		id     string
		number float64
		group  string
	}{
		{"10", 2, "Ronin Fansub"},
		{"11", 2, "La Quinta Hoja Scanlation"},
		{"12", 1, ""},
	}
	for i, w := range want {
		c := chapters[i].(*HeavenMangaChapter)
		if c.ID != w.id || c.GetNumber() != w.number || c.Group != w.group {
			t.Errorf("chapter %d = {ID %q, number %v, group %q}, want {%q, %v, %q}",
				i, c.ID, c.GetNumber(), c.Group, w.id, w.number, w.group)
		}
		if c.URL != srv.URL+"/manga/leer/"+w.id {
			t.Errorf("chapter %d URL = %q", i, c.URL)
		}
	}
}

func TestHeavenMangaFetchChapter(t *testing.T) {
	// the reader's script lists the pages with escaped slashes
	reader := `<script>$('#lector').konekomangareader('setup',{'pages': 2,
		"pages":[{"npage": 1, "imgURL": "https:\/\/blogger.example\/img\/a\/s16000\/0001.webp"},` +
		`{"npage": 2, "imgURL": "https:\/\/blogger.example\/img\/b\/s16000\/0002.webp"}]});</script>`
	srv := heavenMangaTestServer(t, nil, map[string]string{"/manga/leer/10": reader})

	listed := &HeavenMangaChapter{
		Chapter: Chapter{Number: 2, Title: "Chapter 2", ID: "10"},
		URL:     srv.URL + "/manga/leer/10",
	}
	listed.SetGroups("Ronin Fansub")

	chapter, err := NewHeavenManga(&Grabber{URL: srv.URL + "/manga/black-clover"}).FetchChapter(listed)
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if chapter.PagesCount != 2 || len(chapter.Pages) != 2 {
		t.Fatalf("FetchChapter() pages = %d, want 2", len(chapter.Pages))
	}
	if chapter.Pages[0].URL != "https://blogger.example/img/a/s16000/0001.webp" {
		t.Errorf("page 1 URL = %q", chapter.Pages[0].URL)
	}
	if chapter.ID != "10" || chapter.Group != "Ronin Fansub" {
		t.Errorf("chapter kept ID %q and group %q, want 10 and Ronin Fansub", chapter.ID, chapter.Group)
	}
}
