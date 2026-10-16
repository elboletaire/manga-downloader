// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// toivoLiteSites lists the single-manga "Toivo Lite" WordPress sites. Add a
// domain here plus makefile, smoke.yaml and README to support a new site.
// heavenly-delusion.com uses HeavenlyDelusion (sitemap-based, not the widget).
var toivoLiteSites = []string{
	"jojolandsmanga.com",
	"the-apothecary-diaries.com",
}

// ToivoLite is a grabber for single-manga WordPress blogs running the "Toivo
// Lite" theme. The "#Chapters_List" sidebar widget lists every released
// chapter; pages are plain <img> tags in div.entry-content.
type ToivoLite struct {
	*Grabber
	chapters    Filterables
	errs        []error
	title       string
	rootFetched bool
}

// NewToivoLite creates a new ToivoLite grabber.
func NewToivoLite(g *Grabber) *ToivoLite {
	return &ToivoLite{Grabber: g}
}

// ToivoLiteChapter is a chapter fetched by the ToivoLite grabber.
type ToivoLiteChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a Toivo Lite site URL (host check, no fetch).
func (t *ToivoLite) Test() (bool, error) {
	for _, d := range toivoLiteSites {
		if hostMatchesDomain(t.URL, d) {
			return true, nil
		}
	}
	return false, nil
}

// fetchRoot fetches the site root, parses the title and chapter list, and
// caches them. FetchTitle and FetchChapters share this single fetch.
func (t *ToivoLite) fetchRoot() error {
	if t.rootFetched {
		return nil
	}

	body, err := http.Get(http.RequestParams{
		URL: t.BaseUrl() + "/",
	})
	if err != nil {
		return err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}

	// h1.entry-title to avoid matching chapter page <h1> titles
	t.title = strings.TrimSpace(doc.Find("h1.entry-title").Text())

	entries, errs := toivoLiteChapterList(doc, t.BaseUrl())
	t.errs = errs

	t.chapters = make(Filterables, 0, len(entries))
	for _, e := range entries {
		t.chapters = append(t.chapters, &ToivoLiteChapter{
			Chapter{
				Title:  e.Title,
				Number: e.Number,
				ID:     chapterURLID(e.URL),
				Label:  e.Label,
			},
			e.URL,
		})
	}

	t.rootFetched = true
	return nil
}

// FetchTitle returns the manga title.
func (t *ToivoLite) FetchTitle() (string, error) {
	if err := t.fetchRoot(); err != nil {
		return "", err
	}
	return t.title, nil
}

// FetchChapters returns the full chapter list.
func (t *ToivoLite) FetchChapters() (Filterables, []error) {
	if err := t.fetchRoot(); err != nil {
		return nil, []error{err}
	}
	return t.chapters, t.errs
}

// FetchChapter fetches a chapter and returns its pages.
func (t *ToivoLite) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*ToivoLiteChapter)

	body, err := http.Get(http.RequestParams{
		URL:     tchap.URL,
		Referer: t.BaseUrl() + "/",
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
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
		ID:         chapterURLID(tchap.URL),
	}, nil
}

// toivoLiteChapterEntry is one chapter as listed in the sidebar widget.
type toivoLiteChapterEntry struct {
	Title  string
	Label  Label
	Number float64
	URL    string
}

// toivoLiteChapterNumRe pulls the chapter number from widget link text
// ("The Apothecary Diaries, Chapter 63.2" → "63.2").
var toivoLiteChapterNumRe = regexp.MustCompile(`(?i)chapter\s*(\d+(?:\.\d+)?)`)

// toivoLiteChapterList parses the "All Chapters" sidebar widget shared by
// every Toivo Lite page. Numbers come from the link text ("Chapter 63.2"),
// not the slug (which uses hyphens indistinguishable from WP duplicate-slug
// suffixes). The selector scopes to the inner <ul> to avoid matching the
// outer #Chapters_List <li> scaffold.
func toivoLiteChapterList(doc *goquery.Document, baseURL string) (entries []toivoLiteChapterEntry, errs []error) {
	doc.Find("#Chapters_List .ceo_latest_comics_widget ul li").Each(func(i int, s *goquery.Selection) {
		link := s.Find("a")
		rawTitle := strings.TrimSpace(link.Text())

		u := link.AttrOr("href", "")
		if u == "" {
			return
		}
		if !strings.HasPrefix(u, "http") {
			u = baseURL + u
		}

		num, ok := toivoLiteChapterNum(rawTitle, u)
		if !ok {
			errs = append(errs, fmt.Errorf("could not parse chapter number from %q (%s)", rawTitle, u))
			return
		}

		title, label := toivoLiteChapterTitle(rawTitle)

		entries = append(entries, toivoLiteChapterEntry{
			Title:  title,
			Label:  label,
			Number: num,
			URL:    u,
		})
	})

	return
}

// toivoLiteChapterTitle strips the series-name prefix, keeping from the
// chapter word onwards ("JOJOLands, Chapter 41: X" → "Chapter 41: X"), or no
// title when only the number follows, which the label gives as written.
func toivoLiteChapterTitle(raw string) (string, Label) {
	idx := strings.Index(strings.ToLower(raw), "chapter")
	if idx < 0 {
		return raw, Label{}
	}
	if m := toivoLiteNumberOnlyRe.FindStringSubmatch(raw[idx+len("chapter"):]); m != nil {
		return "", Label{Style: LabelNumbered, Number: m[1]}
	}
	return raw[idx:], Label{}
}

// toivoLiteNumberOnlyRe matches what follows the chapter word when it's just the number
var toivoLiteNumberOnlyRe = regexp.MustCompile(`^ (\d+(?:\.\d+)?)$`)

// toivoLiteChapterNum resolves a chapter number from widget link text,
// falling back to the URL slug.
func toivoLiteChapterNum(title, url string) (float64, bool) {
	if m := toivoLiteChapterNumRe.FindStringSubmatch(title); m != nil {
		if num, err := strconv.ParseFloat(m[1], 64); err == nil {
			return num, true
		}
	}
	if num, _, ok := parseChapterSlugNum(url); ok {
		return num, true
	}
	return 0, false
}

// toivoLiteEntryContentPages extracts page images from a Toivo Lite chapter
// post (plain <img src> in div.entry-content). Returns an error if the post
// contains "not available yet" text to avoid silently accepting a placeholder.
func toivoLiteEntryContentPages(doc *goquery.Document, chapterTitle string) ([]Page, error) {
	content := doc.Find("div.entry-content")

	if strings.Contains(strings.ToLower(content.Text()), "not available yet") {
		return nil, fmt.Errorf("chapter %s is not available yet", chapterTitle)
	}

	var pages []Page
	content.Find("img").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		pages = append(pages, Page{
			Number: int64(len(pages) + 1),
			URL:    src,
		})
	})

	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", chapterTitle)
	}

	return pages, nil
}
