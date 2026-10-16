// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

const heavenlyDelusionDomain = "heavenly-delusion.com"

// HeavenlyDelusion is a grabber for heavenly-delusion.com, a single-manga
// WordPress blog running the "Toivo Lite" theme. Chapter enumeration uses the
// XML sitemap (via robots.txt, with /sitemap.xml and /wp-sitemap.xml as
// fallbacks) rather than the homepage widget, which only lists 10 chapters.
type HeavenlyDelusion struct {
	*Grabber
	chapters Filterables
	title    string
}

// NewHeavenlyDelusion returns a new HeavenlyDelusion grabber.
func NewHeavenlyDelusion(g *Grabber) *HeavenlyDelusion {
	return &HeavenlyDelusion{Grabber: g}
}

// HeavenlyDelusionChapter is a chapter for heavenly-delusion.com.
type HeavenlyDelusionChapter struct {
	Chapter
	URL string
}

// heavenlyDelusionChapterPath filters out tag archives and static pages: a
// tag like /tag/heavenly-delusion-chapter-32/ matches the chapter slug regex
// but must not become a phantom chapter.
const heavenlyDelusionChapterPath = "/manga/"

// Test returns true if the URL is a heavenly-delusion.com URL (host check, no fetch).
func (h *HeavenlyDelusion) Test() (bool, error) {
	return hostMatchesDomain(h.URL, heavenlyDelusionDomain), nil
}

// fetchChapterList returns all chapters from the site's XML sitemap.
// Candidates (robots.txt first, then well-known paths) are tried in order;
// errors are skipped.
func (h *HeavenlyDelusion) fetchChapterList() (Filterables, error) {
	candidates := h.sitemapCandidates()

	var errs []string
	for _, sm := range candidates {
		locs, err := collectSitemapURLs(sm, 0)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", sm, err))
			continue
		}
		if chapters := h.chaptersFromLocs(locs); len(chapters) > 0 {
			return chapters, nil
		}
		errs = append(errs, fmt.Sprintf("%s: no chapter URLs", sm))
	}

	return nil, fmt.Errorf("no chapters found in any sitemap (tried %d: %s)",
		len(candidates), strings.Join(errs, "; "))
}

// sitemapCandidates lists sitemap URLs to probe, most authoritative first.
func (h *HeavenlyDelusion) sitemapCandidates() []string {
	base := strings.TrimSuffix(h.BaseUrl(), "/")

	var out []string
	seen := map[string]bool{}
	add := func(u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}

	for _, u := range robotsSitemaps(base) {
		add(u)
	}
	add(base + "/sitemap.xml")
	add(base + "/wp-sitemap.xml")

	return out
}

// chaptersFromLocs extracts chapters from sitemap URLs, filtering out
// tag archives and other non-chapter pages.
func (h *HeavenlyDelusion) chaptersFromLocs(locs []string) Filterables {
	var chapters Filterables
	seen := map[string]bool{}

	for _, loc := range locs {
		if !strings.Contains(loc, heavenlyDelusionChapterPath) {
			continue
		}
		num, numStr, ok := parseChapterSlugNum(loc)
		if !ok || seen[loc] {
			continue
		}
		seen[loc] = true

		chapters = append(chapters, &HeavenlyDelusionChapter{
			Chapter{
				Number: num,
				ID:     chapterURLID(loc),
				Label:  Label{Style: LabelNumbered, Number: numStr},
			},
			loc,
		})
	}

	return chapters
}

// FetchTitle returns the manga title.
func (h *HeavenlyDelusion) FetchTitle() (string, error) {
	if h.title != "" {
		return h.title, nil
	}

	rbody, err := http.Get(http.RequestParams{URL: h.BaseUrl()})
	if err != nil {
		return "", err
	}
	doc, err := goquery.NewDocumentFromReader(rbody)
	rbody.Close()
	if err != nil {
		return "", err
	}
	h.title = strings.TrimSpace(doc.Find("h1.entry-title").Text())
	return h.title, nil
}

// FetchChapters returns the chapter list from the site's XML sitemap.
func (h *HeavenlyDelusion) FetchChapters() (chapters Filterables, errs []error) {
	if _, err := h.FetchTitle(); err != nil {
		return nil, []error{err}
	}
	if h.chapters != nil {
		return h.chapters, nil
	}
	var err error
	h.chapters, err = h.fetchChapterList()
	if err != nil {
		return nil, []error{err}
	}
	return h.chapters, nil
}

// FetchChapter fetches a chapter and its pages.
func (h *HeavenlyDelusion) FetchChapter(f Filterable) (*Chapter, error) {
	hchap := f.(*HeavenlyDelusionChapter)

	rbody, err := http.Get(http.RequestParams{
		URL:     hchap.URL,
		Referer: h.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()

	doc, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return nil, err
	}

	pages, err := toivoLiteEntryContentPages(doc, ChapterLabel(f))
	if err != nil {
		return nil, err
	}

	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		Pages:      pages,
		ID:         chapterURLID(hchap.URL),
	}, nil
}
