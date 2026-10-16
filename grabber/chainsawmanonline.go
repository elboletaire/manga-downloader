// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

const chainsawManOnlineDomain = "chainsawman-chapter.online"

// ChainsawManOnline is a grabber for chainsawman-chapter.online and its
// rotating wN. mirror subdomains. It is a single-manga WordPress site on a
// custom "NEWKOFI" theme, so domain matching is appropriate here.
// The chapter list lives on the site root (/); FetchTitle and FetchChapters
// always fetch BaseUrl() so chapter URLs also work as entry points.
type ChainsawManOnline struct {
	*Grabber
	chapters    Filterables
	title       string
	rootFetched bool
}

// NewChainsawManOnline returns a new ChainsawManOnline grabber.
func NewChainsawManOnline(g *Grabber) *ChainsawManOnline {
	return &ChainsawManOnline{Grabber: g}
}

// ChainsawManOnlineChapter is a chapter for chainsawman-chapter.online.
type ChainsawManOnlineChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a chainsawman-chapter.online URL (host check, no fetch).
func (c *ChainsawManOnline) Test() (bool, error) {
	return hostMatchesDomain(c.URL, chainsawManOnlineDomain), nil
}

// fetchRoot fetches the series root page, parses the title and chapter list,
// and caches them. FetchTitle and FetchChapters share this single fetch.
func (c *ChainsawManOnline) fetchRoot() error {
	if c.rootFetched {
		return nil
	}

	rbody, err := http.Get(http.RequestParams{URL: c.BaseUrl()})
	if err != nil {
		return err
	}
	defer rbody.Close()

	doc, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return err
	}

	c.title, c.chapters = chainsawManOnlineParseRoot(doc)
	c.rootFetched = true
	return nil
}

// chainsawManOnlineParseRoot scrapes the chapter list and series title from
// the site root. Chapter numbers come from each link's aria-label
// ("Chainsaw Man, Chapter 232").
func chainsawManOnlineParseRoot(doc *goquery.Document) (string, Filterables) {
	var title string
	var chapters Filterables

	doc.Find("a.chapter-list-item").Each(func(_ int, s *goquery.Selection) {
		href := strings.TrimSpace(s.AttrOr("href", ""))
		label := strings.TrimSpace(s.AttrOr("aria-label", ""))
		if href == "" || label == "" {
			return
		}

		// aria-label format: "Chainsaw Man, Chapter 232"
		const chapterSep = ", Chapter "
		idx := strings.LastIndex(label, chapterSep)
		if idx < 0 {
			return
		}

		seriesName := label[:idx]
		numStr := label[idx+len(chapterSep):]

		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return
		}

		if title == "" {
			title = seriesName
		}

		chapters = append(chapters, &ChainsawManOnlineChapter{
			Chapter{
				Number: num,
				ID:     chapterURLID(href),
				Label:  Label{Style: LabelNumbered, Number: numStr},
			},
			href,
		})
	})

	return title, chapters
}

// FetchTitle returns the manga title.
func (c *ChainsawManOnline) FetchTitle() (string, error) {
	if err := c.fetchRoot(); err != nil {
		return "", err
	}
	return c.title, nil
}

// FetchChapters returns the full chapter list.
func (c *ChainsawManOnline) FetchChapters() (Filterables, []error) {
	if err := c.fetchRoot(); err != nil {
		return nil, []error{err}
	}
	return c.chapters, nil
}

// FetchChapter fetches a chapter's pages. Images are scoped to
// div.images-container to exclude Ko-fi widget images in the same post body.
// Each image is embedded twice in the container; the seen map deduplicates.
func (c *ChainsawManOnline) FetchChapter(f Filterable) (*Chapter, error) {
	cchap := f.(*ChainsawManOnlineChapter)

	rbody, err := http.Get(http.RequestParams{
		URL:     cchap.URL,
		Referer: c.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()

	doc, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return nil, err
	}

	var pages []Page
	seen := map[string]bool{}
	doc.Find("div.images-container img").Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" || strings.HasPrefix(src, "data:") || seen[src] {
			return
		}
		seen[src] = true
		pages = append(pages, Page{
			Number: int64(len(pages) + 1),
			URL:    src,
		})
	})

	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}

	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "en",
		Pages:      pages,
		ID:         chapterURLID(cchap.URL),
	}, nil
}
