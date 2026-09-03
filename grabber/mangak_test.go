// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseMangakChaptersList(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{
			name: "full list",
			raw: `{"success":true,"data":{"chapters":[` +
				`{"id":"2ZWkoJV8","url":"/nano-machine/chapter-328","name":"Chapter 328","slug":"chapter-328","number":397,"cv":1788422605659},` +
				`{"id":"YPVRLaj2","url":"/nano-machine/chapter-327","name":"Chapter 327","slug":"chapter-327","number":396,"cv":1788422605659},` +
				`{"id":"aBcDeFgH","url":"/nano-machine/chapter-0","name":"Chapter 0","slug":"chapter-0","number":1,"cv":1788422605659}]}}`,
			want: 3,
		},
		{name: "unsuccessful response", raw: `{"success":false,"message":"nope"}`, wantErr: true},
		{name: "missing data", raw: `{"success":true}`, wantErr: true},
		{name: "empty chapter list", raw: `{"success":true,"data":{"chapters":[]}}`, wantErr: true},
		{name: "not json", raw: `<!DOCTYPE html><html>an error page</html>`, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chapters, err := parseMangakChaptersList([]byte(c.raw))
			if c.wantErr {
				if err == nil {
					t.Fatal("parseMangakChaptersList() expected an error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMangakChaptersList() error = %v", err)
			}
			if len(chapters) != c.want {
				t.Fatalf("parseMangakChaptersList() got %d chapters, want %d", len(chapters), c.want)
			}
		})
	}
}

func TestParseMangakChaptersListFields(t *testing.T) {
	raw := `{"success":true,"data":{"chapters":[{"id":"x","url":"/some-series/chapter-12.5","name":"Chapter 12.5","slug":"chapter-12.5","number":13}]}}`

	chapters, err := parseMangakChaptersList([]byte(raw))
	if err != nil {
		t.Fatalf("parseMangakChaptersList() error = %v", err)
	}
	if chapters[0].Name != "Chapter 12.5" || chapters[0].URL != "/some-series/chapter-12.5" {
		t.Errorf("parseMangakChaptersList()[0] = %+v, unexpected", chapters[0])
	}
}

func TestMangakNextDataSeriesPage(t *testing.T) {
	// a trimmed-down series page __NEXT_DATA__: the embedded chapters list
	// only carries the newest chapters, while stats.chaptersCount declares
	// the real total, and siteConfig carries the API base for the full list
	raw := `{"props":{"pageProps":{"siteConfig":{"apiUrl":"https://api.mangak.io"},` +
		`"initialManga":{"id":"jD6jV0DM","name":"Nano Machine","cv":1788422605659,` +
		`"chapters":[{"name":"Chapter 328","url":"/nano-machine/chapter-328"}],` +
		`"stats":{"chaptersCount":397}}}}}`

	data := &mangakNextData{}
	if err := json.Unmarshal([]byte(raw), data); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	manga := data.Props.PageProps.InitialManga
	if manga == nil {
		t.Fatal("initialManga is nil")
	}
	if manga.ID != "jD6jV0DM" || manga.CV != 1788422605659 {
		t.Errorf("manga id/cv = %q/%d, unexpected", manga.ID, manga.CV)
	}
	if manga.Stats.ChaptersCount != 397 {
		t.Errorf("chaptersCount = %d, want 397", manga.Stats.ChaptersCount)
	}
	if len(manga.Chapters) != 1 || manga.Chapters[0].URL != "/nano-machine/chapter-328" {
		t.Errorf("chapters = %+v, unexpected", manga.Chapters)
	}
	if data.Props.PageProps.SiteConfig.APIURL != "https://api.mangak.io" {
		t.Errorf("apiUrl = %q, unexpected", data.Props.PageProps.SiteConfig.APIURL)
	}
}

func TestMangakMissingChapters(t *testing.T) {
	cases := []struct {
		name  string
		count int
		list  int
		want  int
	}{
		{name: "complete list", count: 397, list: 397},
		{name: "truncated to the embedded 50", count: 397, list: 50, want: 347},
		{name: "short series embeds everything", count: 38, list: 38},
		{name: "unknown total", count: 0, list: 50},
		{name: "more chapters than declared", count: 10, list: 12},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			manga := &mangakManga{Chapters: make([]mangakChapterEntry, c.list)}
			manga.Stats.ChaptersCount = c.count
			if got := mangakMissingChapters(manga); got != c.want {
				t.Errorf("mangakMissingChapters() = %d, want %d", got, c.want)
			}
		})
	}
}

// mangakServer serves a fake mangak.io: a series page whose __NEXT_DATA__
// embeds only `embedded` of `total` chapters, plus the chapters list API
// (which only answers when `apiOK`, so the truncation fallback can be tested)
func mangakServer(t *testing.T, total, embedded int, apiOK bool) (*httptest.Server, *[]string) {
	t.Helper()

	var requested []string
	mux := http.NewServeMux()
	mux.HandleFunc("/titles/jD6jV0DM/chapters", func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		if !apiOK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		entries := make([]string, 0, total)
		// newest first, as the real API returns them
		for i := total; i > 0; i-- {
			entries = append(entries, fmt.Sprintf(`{"name":"Chapter %d","url":"/nano-machine/chapter-%d"}`, i, i))
		}
		fmt.Fprintf(w, `{"success":true,"data":{"chapters":[%s]}}`, strings.Join(entries, ","))
	})
	mux.HandleFunc("/nano-machine", func(w http.ResponseWriter, r *http.Request) {
		entries := make([]string, 0, embedded)
		for i := total; i > total-embedded; i-- {
			entries = append(entries, fmt.Sprintf(`{"name":"Chapter %d","url":"/nano-machine/chapter-%d"}`, i, i))
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><script id="__NEXT_DATA__" type="application/json">`+
			`{"props":{"pageProps":{"siteConfig":{"apiUrl":"%s"},"initialManga":{"id":"jD6jV0DM",`+
			`"name":"Nano Machine","cv":1788422605659,"chapters":[%s],"stats":{"chaptersCount":%d}}}}}`+
			`</script></body></html>`, "http://"+r.Host, strings.Join(entries, ","), total)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, &requested
}

func TestMangakFetchChaptersUsesAPI(t *testing.T) {
	server, requested := mangakServer(t, 397, 50, true)
	m := NewMangak(&Grabber{URL: server.URL + "/nano-machine", Settings: &Settings{}})

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	// without the API fetch only the 50 embedded chapters would show up
	if len(chapters) != 397 {
		t.Fatalf("FetchChapters() got %d chapters, want 397", len(chapters))
	}
	if len(*requested) != 1 {
		t.Fatalf("api requests = %v, want exactly one", *requested)
	}
	// the cv cache-version token is load-bearing: without it the cdn serves a
	// stale list missing the newest chapters
	if !strings.Contains((*requested)[0], "cv=1788422605659") {
		t.Errorf("api request = %q, expected the cv query param", (*requested)[0])
	}
}

// TestMangakFetchChaptersAPIFailure is the one that matters: a failed API call
// must not quietly degrade to the truncated list embedded in the series page
func TestMangakFetchChaptersAPIFailure(t *testing.T) {
	server, _ := mangakServer(t, 397, 50, false)
	m := NewMangak(&Grabber{URL: server.URL + "/nano-machine", Settings: &Settings{}})

	chapters, errs := m.FetchChapters()
	if len(errs) == 0 {
		t.Fatalf("FetchChapters() returned %d chapters and no error, expected it to fail loudly", len(chapters))
	}
}

// TestMangakFetchChaptersAPIFailureShortSeries checks the other side of it: a
// series the page embeds in full stays downloadable when the API is down
func TestMangakFetchChaptersAPIFailureShortSeries(t *testing.T) {
	server, _ := mangakServer(t, 38, 38, false)
	m := NewMangak(&Grabber{URL: server.URL + "/nano-machine", Settings: &Settings{}})

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 38 {
		t.Fatalf("FetchChapters() got %d chapters, want 38", len(chapters))
	}
}
