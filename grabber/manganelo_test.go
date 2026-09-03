// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// manganeloChaptersAPI serves a fake /api/manga/{slug}/chapters endpoint over
// total chapters, capping every response at pageSize regardless of the
// requested limit (which is what forces the pagination walk). It records how
// many requests it served and the offsets it was asked for.
func manganeloChaptersAPI(total, pageSize int, requests *[]int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		*requests = append(*requests, offset)

		type chapter struct {
			Name   string  `json:"chapter_name"`
			Slug   string  `json:"chapter_slug"`
			Number float64 `json:"chapter_num"`
		}
		chapters := []chapter{}
		// newest first, like the real API: chapter total down to 1
		for i := 0; i < pageSize && offset+i < total; i++ {
			n := float64(total - (offset + i))
			chapters = append(chapters, chapter{
				Name:   fmt.Sprintf("Chapter %d", int(n)),
				Slug:   fmt.Sprintf("chapter-%d", int(n)),
				Number: n,
			})
		}

		resp := map[string]any{
			"success": true,
			"data": map[string]any{
				"chapters": chapters,
				"pagination": map[string]any{
					"total":    total,
					"limit":    pageSize,
					"offset":   offset,
					"has_more": offset+len(chapters) < total,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestManganeloFetchChaptersWalksAllPages is the regression test for the bug
// this grabber exists to fix: a server-side page cap must not silently
// truncate the chapter list. It fails if FetchChapters ever goes back to
// issuing a single request.
func TestManganeloFetchChaptersWalksAllPages(t *testing.T) {
	const total, pageSize = 237, 50

	var requests []int
	srv := manganeloChaptersAPI(total, pageSize, &requests)
	defer srv.Close()

	m := NewManganelo(&Grabber{URL: srv.URL + "/manga/test-series"})
	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters returned errors: %v", errs)
	}

	if len(chapters) != total {
		t.Fatalf("got %d chapters, want %d (the list was truncated to the first page)", len(chapters), total)
	}

	wantRequests := (total + pageSize - 1) / pageSize
	if len(requests) != wantRequests {
		t.Errorf("served %d requests, want %d", len(requests), wantRequests)
	}
	for i, offset := range requests {
		if want := i * pageSize; offset != want {
			t.Errorf("request %d asked for offset %d, want %d", i, offset, want)
		}
	}

	// the oldest chapter only exists on the last page, so its presence is
	// what proves the whole walk completed
	last := chapters[len(chapters)-1].(*ManganeloChapter)
	if last.Number != 1 || last.Slug != "chapter-1" {
		t.Errorf("last chapter = %v (%q), want 1 (chapter-1)", last.Number, last.Slug)
	}
}

// TestManganeloFetchChaptersMidWalkFailure makes sure a failure partway
// through the walk surfaces as an error instead of yielding a short list —
// silently returning the pages fetched so far would reintroduce the exact
// truncation this grabber fixes.
func TestManganeloFetchChaptersMidWalkFailure(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"chapters":[
			{"chapter_name":"Chapter 2","chapter_slug":"chapter-2","chapter_num":2}
		],"pagination":{"total":2,"limit":1,"offset":0,"has_more":true}}}`))
	}))
	defer srv.Close()

	m := NewManganelo(&Grabber{URL: srv.URL + "/manga/test-series"})
	chapters, errs := m.FetchChapters()
	if len(errs) == 0 {
		t.Fatalf("expected an error when a page fails mid-walk, got %d chapters", len(chapters))
	}
	if chapters != nil {
		t.Errorf("expected no chapters on a mid-walk failure, got %d", len(chapters))
	}
}

// TestManganeloFetchChaptersBounded guards against an API that reports
// has_more forever while ignoring the offset: the walk must give up with an
// error rather than append the same page until it runs out of memory.
func TestManganeloFetchChaptersBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"chapters":[
			{"chapter_name":"Chapter 1","chapter_slug":"chapter-1","chapter_num":1}
		],"pagination":{"total":9999,"limit":1,"offset":0,"has_more":true}}}`))
	}))
	defer srv.Close()

	m := NewManganelo(&Grabber{URL: srv.URL + "/manga/test-series"})
	if _, errs := m.FetchChapters(); len(errs) == 0 {
		t.Fatal("expected the pagination walk to be bounded, but it terminated cleanly")
	}
}

// TestManganeloChapterTitleSanitized covers the repeat bug called out in
// AGENTS.md: chapter titles need sanitizeTitle() too, or padded API names
// leak into the cbz filename.
func TestManganeloChapterTitleSanitized(t *testing.T) {
	body := `{"success":true,"data":{"chapters":[
		{"chapter_name":"\n\tChapter  12\n","chapter_slug":"chapter-12","chapter_num":12}
	],"pagination":{"total":1,"limit":50,"offset":0,"has_more":false}}}`

	chapters, _, err := parseManganeloChaptersPage(body)
	if err != nil {
		t.Fatalf("parseManganeloChaptersPage returned error: %v", err)
	}
	if got := chapters[0].GetTitle(); got != "Chapter 12" {
		t.Errorf("title = %q, want %q", got, "Chapter 12")
	}
}

func TestManganeloTest(t *testing.T) {
	cases := []struct {
		url       string
		wantMatch bool
	}{
		{"https://www.mangabats.com/manga/one-piece", true},
		{"https://mangabats.com/manga/one-piece", true},
		{"https://www.mangakakalot.gg/manga/apotheosis", true},
		{"https://mangakakalot.gg/manga/apotheosis", true},
		{"https://www.natomanga.com/manga/rebirth-from-0-to-1", true},
		{"https://natomanga.com/manga/rebirth-from-0-to-1", true},
		{"https://example.com/manga/one-piece", false},
		// hostname anchored: no substring matches
		{"https://notmangabats.com/manga/one-piece", false},
		{"https://mangakakalot.gg.evil.com/manga/apotheosis", false},
		// the family domain must be in the host, not elsewhere in the URL
		{"https://example.com/manga/natomanga.com", false},
	}

	for _, c := range cases {
		m := NewManganelo(&Grabber{URL: c.url})
		got, err := m.Test()
		if err != nil {
			t.Errorf("Test(%q) returned error: %v", c.url, err)
			continue
		}
		if got != c.wantMatch {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.wantMatch)
		}
	}
}

func TestParseManganeloChaptersPage(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantCount   int
		wantHasMore bool
		wantErr     bool
		wantFirst   float64
		wantTitle   string
		wantSlug    string
	}{
		{
			name: "page with more remaining",
			body: `{"success":true,"data":{"chapters":[
				{"chapter_name":"Chapter 1301","chapter_slug":"chapter-1301","chapter_num":1301,"updated_at":"2025-07-21T09:16:14.000000Z","view":32629},
				{"chapter_name":"Chapter 1300.5","chapter_slug":"chapter-1300-5","chapter_num":1300.5,"view":13479}
			],"pagination":{"total":1398,"limit":2,"offset":0,"has_more":true}}}`,
			wantCount:   2,
			wantHasMore: true,
			wantFirst:   1301,
			wantTitle:   "Chapter 1301",
			wantSlug:    "chapter-1301",
		},
		{
			name: "final page",
			body: `{"success":true,"data":{"chapters":[
				{"chapter_name":"Chapter 1","chapter_slug":"chapter-1","chapter_num":1}
			],"pagination":{"total":1398,"limit":9999,"offset":0,"has_more":false}}}`,
			wantCount:   1,
			wantHasMore: false,
			wantFirst:   1,
			wantTitle:   "Chapter 1",
			wantSlug:    "chapter-1",
		},
		{
			name: "unnamed chapter falls back to its number",
			body: `{"success":true,"data":{"chapters":[
				{"chapter_name":"","chapter_slug":"chapter-2-5","chapter_num":2.5}
			],"pagination":{"total":1,"limit":50,"offset":0,"has_more":false}}}`,
			wantCount: 1,
			wantFirst: 2.5,
			wantTitle: "Chapter 2.5",
			wantSlug:  "chapter-2-5",
		},
		{
			name:      "empty chapter list",
			body:      `{"success":true,"data":{"chapters":[],"pagination":{"total":0,"limit":50,"offset":0,"has_more":false}}}`,
			wantCount: 0,
		},
		{
			name:    "api failure",
			body:    `{"success":false,"message":"Comic not found"}`,
			wantErr: true,
		},
		{
			name:    "malformed json",
			body:    `<!DOCTYPE html><html>Just a moment...</html>`,
			wantErr: true,
		},
	}

	for _, c := range cases {
		chapters, hasMore, err := parseManganeloChaptersPage(c.body)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", c.name, err, c.wantErr)
			continue
		}
		if c.wantErr {
			continue
		}
		if len(chapters) != c.wantCount {
			t.Errorf("%s: got %d chapters, want %d", c.name, len(chapters), c.wantCount)
			continue
		}
		if hasMore != c.wantHasMore {
			t.Errorf("%s: hasMore = %v, want %v", c.name, hasMore, c.wantHasMore)
		}
		if c.wantCount == 0 {
			continue
		}
		first := chapters[0].(*ManganeloChapter)
		if first.Number != c.wantFirst {
			t.Errorf("%s: first number = %v, want %v", c.name, first.Number, c.wantFirst)
		}
		if first.Title != c.wantTitle {
			t.Errorf("%s: first title = %q, want %q", c.name, first.Title, c.wantTitle)
		}
		if first.Slug != c.wantSlug {
			t.Errorf("%s: first slug = %q, want %q", c.name, first.Slug, c.wantSlug)
		}
	}
}

func TestManganeloSlug(t *testing.T) {
	cases := []struct {
		url      string
		wantSlug string
		wantErr  bool
	}{
		{"https://www.mangabats.com/manga/one-piece", "one-piece", false},
		{"https://www.mangakakalot.gg/manga/apotheosis/chapter-2", "apotheosis", false},
		{"https://www.natomanga.com/genre/action", "", true},
	}

	for _, c := range cases {
		m := NewManganelo(&Grabber{URL: c.url})
		slug, err := m.slug()
		if (err != nil) != c.wantErr {
			t.Errorf("slug(%q) err = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if slug != c.wantSlug {
			t.Errorf("slug(%q) = %q, want %q", c.url, slug, c.wantSlug)
		}
	}
}

func TestJsStringSlice(t *testing.T) {
	html := `<script>
		var cdns = ["https:\/\/imgs-2.2xstorage.com"];
		var chapterImages = ["apotheosis\/2\/0.webp","apotheosis\/2\/1.webp"];
	</script>`

	cdns, err := jsStringSlice(html, "cdns")
	if err != nil {
		t.Fatalf("jsStringSlice(cdns) returned error: %v", err)
	}
	if len(cdns) != 1 || cdns[0] != "https://imgs-2.2xstorage.com" {
		t.Errorf("jsStringSlice(cdns) = %v", cdns)
	}

	images, err := jsStringSlice(html, "chapterImages")
	if err != nil {
		t.Fatalf("jsStringSlice(chapterImages) returned error: %v", err)
	}
	if len(images) != 2 || images[1] != "apotheosis/2/1.webp" {
		t.Errorf("jsStringSlice(chapterImages) = %v", images)
	}

	if _, err = jsStringSlice(html, "missing"); err == nil {
		t.Error("jsStringSlice(missing) should have errored")
	}
}
