package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// heavenlyDelusionHomepageHTML is what the site returns when an old sitemap
// path redirects to the homepage — HTML that fails XML parsing.
const heavenlyDelusionHomepageHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Heavenly Delusion</title></head>
<body><div class=wrapper><a href=/manga/ >Read</a></div></body>
</html>`

// heavenlyDelusionFlatSitemap mirrors the Companion Sitemap Generator output:
// one urlset with chapters, tag archives and pages mixed together.
func heavenlyDelusionFlatSitemap(base string) string {
	return fmt.Sprintf(`<?xml version='1.0' encoding='UTF-8' ?>
<urlset xmlns='http://www.sitemaps.org/schemas/sitemap/0.9'>
<url><loc>%[1]s/</loc></url>
<url><loc>%[1]s/manga/heavenly-delusion-chapter-1/</loc></url>
<url><loc>%[1]s/manga/heavenly-delusion-chapter-2/</loc></url>
<url><loc>%[1]s/manga/heavenly-delusion-chapter-13-5/</loc></url>
<url><loc>%[1]s/manga/heavenly-delusion-chapter-84/</loc></url>
<url><loc>%[1]s/manga/heavenly-delusion-chapter-84/</loc></url>
<url><loc>%[1]s/tag/heavenly-delusion-chapter-32/</loc></url>
<url><loc>%[1]s/tag/chapters/</loc></url>
<url><loc>%[1]s/about/</loc></url>
</urlset>`, base)
}

func newHeavenlyDelusionSiteFor(url string) *HeavenlyDelusion {
	h := NewHeavenlyDelusion(&Grabber{URL: url, Settings: &Settings{}})
	h.title = "Heavenly Delusion"
	return h
}

func heavenlyDelusionChapterNums(f Filterables) []float64 {
	out := make([]float64, 0, len(f))
	for _, c := range f {
		out = append(out, c.GetNumber())
	}
	sort.Float64s(out)
	return out
}

// TestHeavenlyDelusionFetchChapterListFlatSitemap verifies sitemap fallback
// when the well-known path redirects to HTML.
func TestHeavenlyDelusionFetchChapterListFlatSitemap(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml", base)
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, heavenlyDelusionFlatSitemap(base))
	})
	mux.HandleFunc("/wp-sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, heavenlyDelusionHomepageHTML)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	chapters, err := newHeavenlyDelusionSiteFor(srv.URL).fetchChapterList()
	if err != nil {
		t.Fatalf("fetchChapterList: %v", err)
	}

	got := heavenlyDelusionChapterNums(chapters)
	want := []float64{1, 2, 13.5, 84}
	if len(got) != len(want) {
		t.Fatalf("got %v chapters (%v), want %v", len(got), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chapter[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestHeavenlyDelusionFetchChapterListExcludesTagArchives ensures tag archive
// URLs are not treated as chapters.
func TestHeavenlyDelusionFetchChapterListExcludesTagArchives(t *testing.T) {
	h := newHeavenlyDelusionSiteFor("https://w7.heavenly-delusion.com/")
	chapters := h.chaptersFromLocs([]string{
		"https://w7.heavenly-delusion.com/manga/heavenly-delusion-chapter-31/",
		"https://w7.heavenly-delusion.com/tag/heavenly-delusion-chapter-32/",
	})

	if len(chapters) != 1 {
		t.Fatalf("got %d chapters, want 1 (tag archive must be excluded)", len(chapters))
	}
	if n := chapters[0].GetNumber(); n != 31 {
		t.Errorf("kept chapter %v, want 31", n)
	}
}

// TestHeavenlyDelusionFetchChapterListSitemapIndex verifies the sitemap-index
// shape (child sitemaps) is followed correctly.
func TestHeavenlyDelusionFetchChapterListSitemapIndex(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<sitemap><loc>%[1]s/wp-sitemap-posts-comic-1.xml</loc></sitemap>
<sitemap><loc>%[1]s/wp-sitemap-taxonomies-post_tag-1.xml</loc></sitemap>
</sitemapindex>`, base)
	})
	mux.HandleFunc("/wp-sitemap-posts-comic-1.xml", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>%[1]s/manga/heavenly-delusion-chapter-7/</loc></url>
<url><loc>%[1]s/manga/heavenly-delusion-chapter-58-5/</loc></url>
</urlset>`, base)
	})
	// A child sitemap that fails must not cost chapters in its sibling.
	mux.HandleFunc("/wp-sitemap-taxonomies-post_tag-1.xml", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	base = srv.URL

	chapters, err := newHeavenlyDelusionSiteFor(srv.URL).fetchChapterList()
	if err != nil {
		t.Fatalf("fetchChapterList: %v", err)
	}

	got := heavenlyDelusionChapterNums(chapters)
	want := []float64{7, 58.5}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestHeavenlyDelusionFetchChapterListNoSitemap checks the error reports what was tried.
func TestHeavenlyDelusionFetchChapterListNoSitemap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := newHeavenlyDelusionSiteFor(srv.URL).fetchChapterList()
	if err == nil {
		t.Fatal("expected an error when no sitemap is reachable")
	}
	if !strings.Contains(err.Error(), "sitemap.xml") || !strings.Contains(err.Error(), "tried") {
		t.Errorf("error should name what was tried, got: %v", err)
	}
}

// TestHeavenlyDelusionTest smoke-tests Test() against the real domain, subdomain and lookalike.
func TestHeavenlyDelusionTest(t *testing.T) {
	yes := func(url string) {
		t.Helper()
		got, err := NewHeavenlyDelusion(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || !got {
			t.Errorf("Test(%q) = %v, %v; want true, nil", url, got, err)
		}
	}
	no := func(url string) {
		t.Helper()
		got, err := NewHeavenlyDelusion(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || got {
			t.Errorf("Test(%q) = %v, %v; want false, nil", url, got, err)
		}
	}

	yes("https://heavenly-delusion.com/")
	yes("https://w7.heavenly-delusion.com/manga/chapter-1/")
	yes("https://HEAVENLY-DELUSION.COM/")
	no("https://myheavenly-delusion.com/")
	no("https://example.com/?redirect=heavenly-delusion.com")
	no("https://mangadex.org/")
}
