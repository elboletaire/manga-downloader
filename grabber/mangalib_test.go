// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMangalibSeriesSlug(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://mangalib.me/ru/manga/206--one-piece", "206--one-piece", false},
		{"https://mangalib.me/manga/206--one-piece", "206--one-piece", false},
		{"https://mangalib.me/manga/one-piece", "one-piece", false},
		{"https://mangalib.me/ru/manga/206--one-piece?query=1", "206--one-piece", false},
		// chapter reader URL: extra path segments after the slug are ignored
		{"https://mangalib.me/ru/manga/206--one-piece/v1/c1188", "206--one-piece", false},
		{"https://mangalib.me/ru/manga/", "", true},
		{"https://mangalib.me/", "", true},
	}

	for _, c := range cases {
		m := Mangalib{Grabber: &Grabber{URL: c.url}}
		got, err := m.seriesSlug()
		if (err != nil) != c.wantErr {
			t.Errorf("seriesSlug(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("seriesSlug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestMangalibTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://mangalib.me/ru/manga/206--one-piece", true},
		{"https://mangalib.me/manga/one-piece", true},
		{"https://mangadex.org/title/xyz", false},
	}

	for _, c := range cases {
		m := &Mangalib{Grabber: &Grabber{URL: c.url}}
		got, err := m.Test()
		if err != nil {
			t.Errorf("Test(%q) unexpected error: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

// mangalibServer serves the chapters list and the chapter endpoint of a fake
// api. Each branch's pages are named after it, so the test can tell them apart;
// the requested query strings are recorded
func mangalibServer(t *testing.T) (*httptest.Server, *[]url.Values) {
	t.Helper()

	var chapterQueries []url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("/api/manga/gintama/chapters", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[`+
			// two branches, one of them with no team
			`{"volume":"1","number":"1","name":"One","branches":[`+
			`{"id":100,"branch_id":200,"teams":[{"id":15840,"name":"Antitype"}]},`+
			`{"id":101,"branch_id":201,"teams":[]}]},`+
			// a branch without a branch_id, and two teams
			`{"volume":"1","number":"2","name":"Two","branches":[`+
			`{"id":102,"branch_id":null,"teams":[{"id":7,"name":"Alpha"},{"id":8,"name":"Beta"}]}]},`+
			// no branch listed at all
			`{"volume":"1","number":"3","name":"Three","branches":[]}`+
			`]}`)
	})
	mux.HandleFunc("/api/manga/gintama/chapter", func(w http.ResponseWriter, r *http.Request) {
		chapterQueries = append(chapterQueries, r.URL.Query())
		fmt.Fprintf(w, `{"data":{"pages":[{"url":"//manga/gintama/%s.jpg"}]}}`, r.URL.Query().Get("branch_id"))
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, &chapterQueries
}

func TestMangalibFetchChaptersPerBranch(t *testing.T) {
	server, _ := mangalibServer(t)
	defer setMangalibApi(server.URL + "/api")()
	m := NewMangalib(&Grabber{URL: "https://mangalib.me/ru/manga/gintama", Settings: &Settings{}})

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}

	want := []struct {
		id, group string
		branch    int
	}{
		{"100", "Antitype", 200},
		{"101", "", 201},
		{"102", "Alpha, Beta", 0},
		{"", "", 0},
	}
	if len(chapters) != len(want) {
		t.Fatalf("FetchChapters() got %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i].(*MangalibChapter)
		if c.ID != w.id || c.Group != w.group || c.BranchID != w.branch {
			t.Errorf("chapter %d = (ID %q, Group %q, BranchID %d), want (%q, %q, %d)",
				i, c.ID, c.Group, c.BranchID, w.id, w.group, w.branch)
		}
	}
}

func TestMangalibFetchChapterRequestsBranch(t *testing.T) {
	server, queries := mangalibServer(t)
	defer setMangalibApi(server.URL + "/api")()
	m := NewMangalib(&Grabber{URL: "https://mangalib.me/ru/manga/gintama", Settings: &Settings{}})

	chapters, _ := m.FetchChapters()
	chapter, err := m.FetchChapter(chapters[0])
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if got := (*queries)[0].Get("branch_id"); got != "200" {
		t.Errorf("branch_id = %q, want 200", got)
	}
	if chapter.ID != "100" || chapter.Group != "Antitype" {
		t.Errorf("FetchChapter() ID %q, Group %q, want 100, Antitype", chapter.ID, chapter.Group)
	}
	if len(chapter.Pages) != 1 || !strings.Contains(chapter.Pages[0].URL, "/200.jpg") {
		t.Errorf("FetchChapter() pages = %+v, want the branch's page", chapter.Pages)
	}

	// a chapter without a branch_id asks for none
	if _, err := m.FetchChapter(chapters[2]); err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if _, ok := (*queries)[1]["branch_id"]; ok {
		t.Errorf("query = %v, want no branch_id", (*queries)[1])
	}
}

// setMangalibApi points the api at base for a test, returning the func that
// restores it
func setMangalibApi(base string) func() {
	old := mangalibApi
	mangalibApi = base
	return func() { mangalibApi = old }
}
