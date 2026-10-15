// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// paginatedListing renders a chapter list page with the given chapter numbers
func paginatedListing(numbers ...int) string {
	var b strings.Builder
	b.WriteString(`<html><body><h2 class="text-2xl">One Piece</h2><div id="chapters-list">`)
	for _, n := range numbers {
		fmt.Fprintf(&b, `<a href="/manga/one-piece/%d"><div class="flex gap-2"><span>Chapitre %d</span></div></a>`, n, n)
	}
	b.WriteString(`</div></body></html>`)
	return b.String()
}

// TestPlainHTMLPaginated checks that a Paginated selector walks ?page=N until a
// page has no rows, and that without the flag only the first page is read
func TestPlainHTMLPaginated(t *testing.T) {
	pages := map[string]string{
		"":  paginatedListing(3, 2),
		"2": paginatedListing(1),
		"3": paginatedListing(),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, ok := pages[r.URL.Query().Get("page")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)

	site := SiteSelector{
		Title:     "h2.text-2xl",
		Rows:      "#chapters-list a",
		Chapter:   "div.flex.gap-2 span",
		Paginated: true,
	}
	m := &PlainHTML{Grabber: &Grabber{URL: srv.URL + "/manga/one-piece"}, site: site}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pages[""]))
	if err != nil {
		t.Fatal(err)
	}
	m.doc = doc
	m.rows = doc.Find(site.Rows)

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("paginated FetchChapters() = %d chapters, want 3", len(chapters))
	}

	m.site.Paginated = false
	chapters, _ = m.FetchChapters()
	if len(chapters) != 2 {
		t.Errorf("unpaginated FetchChapters() = %d chapters, want 2", len(chapters))
	}
}

// TestPlainHTMLTitleNoise checks that TitleNoise leaves a badge nested in the
// title out, as lelscanfr's "[Manga]"
func TestPlainHTMLTitleNoise(t *testing.T) {
	html := `<h2 class="text-2xl">One Piece <span class="text-sm"> [Manga] </span></h2>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	for noise, want := range map[string]string{"span": "One Piece", "": "One Piece [Manga]"} {
		m := PlainHTML{site: SiteSelector{Title: "h2.text-2xl", TitleNoise: noise}, doc: doc}
		if got, _ := m.FetchTitle(); got != want {
			t.Errorf("FetchTitle() with TitleNoise %q = %q, want %q", noise, got, want)
		}
	}
}
