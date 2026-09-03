// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/browser"
)

const mangafireBase = "https://mangafire.to"

// mangafireMaxChapterPages caps how many chapter-list pages the pager is
// clicked through. The list loads 20 chapters per page, so this covers series
// with a few thousand chapters (e.g. One Piece needs ~90) while still bounding
// a runaway loop if the end-of-list detection ever fails.
const mangafireMaxChapterPages = 400

// mangafireNextPageSelector advances the chapter-list pager one page. The
// "Next page" arrow only renders when there are more than ~5 pages, so short
// pagers (numbered buttons only) need the numbered button right after the
// active one instead; on long pagers the arrow covers the case where the
// active number is the last of its window. querySelector picks whichever
// matches first in document order — both advance exactly one page. On the
// last page neither matches (no numbered sibling, arrow gone/disabled), which
// ends the pagination loop.
const mangafireNextPageSelector = `.npager__num.is-active + .npager__num, .npager__nav[aria-label="Next page"]`

// Mangafire is a grabber for mangafire.to. The site is a react SPA whose JSON
// API signs every request with a per-session, Cloudflare-challenge-gated `vrf`
// token generated client-side, so the endpoints can no longer be called with
// plain HTTP (they answer 403 "Missing token"). Instead we drive a real
// (headless) browser: it renders the series/reader pages, makes the signed API
// calls itself, and we intercept the JSON responses. Page images then download
// over plain HTTP (a Referer header is enough, no cookies) reusing the harvested
// browser session.
type Mangafire struct {
	*Grabber
	// cached by load() after the first series render
	loaded     bool
	loadErr    error
	title      string
	readerBase string // canonical reader-URL base, e.g. https://mangafire.to/title/dkw-one-piece
	chapters   Filterables
}

func NewMangafire(g *Grabber) *Mangafire {
	return &Mangafire{Grabber: g}
}

// MangafireChapter represents a Mangafire chapter
type MangafireChapter struct {
	Chapter
	// URL is the chapter reader page URL, rendered in a browser to capture the
	// signed pages-API response
	URL string
	// Id is the numeric chapter id
	Id int64
}

// Test returns true if the URL is a mangafire.to series URL
func (m *Mangafire) Test() (bool, error) {
	re := regexp.MustCompile(`mangafire\.to/(title|manga)/`)
	return re.MatchString(m.URL), nil
}

// FetchTitle returns the manga title
func (m *Mangafire) FetchTitle() (string, error) {
	if err := m.load(); err != nil {
		return "", err
	}
	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangafire) FetchChapters() (Filterables, []error) {
	if err := m.load(); err != nil {
		return nil, []error{err}
	}
	return m.chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (m *Mangafire) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangafireChapter)

	// rendering the reader page makes the SPA call /api/chapters/{id} with a
	// valid vrf; we intercept that single response (which carries the full
	// pages list) instead of trying to sign the call ourselves
	responses, err := browser.GetAPIResponses(
		mchap.URL,
		".reader-img",
		fmt.Sprintf("/api/chapters/%d", mchap.Id),
		"",
		0,
		0,
	)
	if err != nil {
		return nil, err
	}

	var pages []mangafirePage
	for _, r := range responses {
		feed := struct {
			Data struct {
				Pages []mangafirePage `json:"pages"`
			} `json:"data"`
		}{}
		if err := json.Unmarshal([]byte(r.Body), &feed); err != nil {
			continue
		}
		if len(feed.Data.Pages) > 0 {
			pages = feed.Data.Pages
			break
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for chapter %s (it may be premium/locked)", f.GetTitle())
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   mchap.Language,
		PagesCount: int64(len(pages)),
	}
	for i, p := range pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    p.Url,
		})
	}

	return chapter, nil
}

// load renders the series page once, paginating through the whole chapter list,
// and caches the title, the canonical reader-URL base and the chapters. Both
// FetchTitle and FetchChapters go through it, so the (slow) browser render only
// happens once per run.
func (m *Mangafire) load() error {
	if m.loaded {
		return m.loadErr
	}
	m.loaded = true

	hid, err := m.hid()
	if err != nil {
		m.loadErr = err
		return err
	}

	// urlSubstr matches the title (/api/titles/{hid}?...), the volumes list and
	// every chapters page (/api/titles/{hid}/chapters?...); they're told apart
	// below by their exact URL
	responses, err := browser.GetAPIResponses(
		m.URL,
		".title-detail__row-link",
		"/api/titles/"+hid,
		mangafireNextPageSelector,
		mangafireMaxChapterPages,
		0,
	)
	if err != nil {
		m.loadErr = fmt.Errorf("error fetching chapters: %w", err)
		return m.loadErr
	}

	m.readerBase = mangafireBase + "/title/" + hid // fallback if the title call is missed
	titlePrefix := "/api/titles/" + hid + "?"
	chaptersPrefix := "/api/titles/" + hid + "/chapters"

	// first pass: the title-info response gives the clean title and the
	// canonical reader-URL base
	for _, r := range responses {
		if !strings.Contains(r.URL, titlePrefix) {
			continue
		}
		info := struct {
			Data struct {
				Title string `json:"title"`
				URL   string `json:"url"`
			} `json:"data"`
		}{}
		if err := json.Unmarshal([]byte(r.Body), &info); err != nil {
			continue
		}
		if info.Data.Title != "" {
			m.title = info.Data.Title
		}
		if info.Data.URL != "" {
			m.readerBase = mangafireBase + info.Data.URL
		}
		break
	}

	// second pass: collect chapters from every captured feed page
	for _, c := range parseMangafireChapters(responses, chaptersPrefix) {
		title := c.Name
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(c.Number, 'f', -1, 64)
		}
		m.chapters = append(m.chapters, &MangafireChapter{
			Chapter: Chapter{
				Number:   c.Number,
				Title:    title,
				Language: c.Language,
			},
			URL: fmt.Sprintf("%s/chapter/%d", m.readerBase, c.Id),
			Id:  c.Id,
		})
	}

	if len(m.chapters) == 0 {
		m.loadErr = fmt.Errorf("no chapters found for %s", m.URL)
	}

	return m.loadErr
}

// hid returns the title id from the URL, e.g. "dkw" for both the current
// https://mangafire.to/title/dkw-one-piece format and the legacy
// https://mangafire.to/manga/one-piecee.dkw one
func (m *Mangafire) hid() (string, error) {
	re := regexp.MustCompile(`/title/([^/-]+)-`)
	if matches := re.FindStringSubmatch(m.URL); len(matches) == 2 {
		return matches[1], nil
	}
	re = regexp.MustCompile(`/manga/[^/]+\.([^/.]+)`)
	if matches := re.FindStringSubmatch(m.URL); len(matches) == 2 {
		return matches[1], nil
	}
	return "", fmt.Errorf("could not find title id in url %s", m.URL)
}

// parseMangafireChapters collects the chapter items out of the captured API
// responses whose URL matches chaptersPrefix (one response per chapter-list
// page), deduping by chapter number and preferring the official scanlation
// (the api returns both official and unofficial uploads for the same number).
// Items keep their first-seen order, which is the site's own list order.
func parseMangafireChapters(responses []browser.APIResponse, chaptersPrefix string) []mangafireChapterItem {
	best := map[float64]mangafireChapterItem{}
	var order []float64
	for _, r := range responses {
		if !strings.Contains(r.URL, chaptersPrefix) {
			continue
		}
		feed := struct {
			Items []mangafireChapterItem `json:"items"`
		}{}
		if err := json.Unmarshal([]byte(r.Body), &feed); err != nil {
			continue
		}
		for _, c := range feed.Items {
			cur, ok := best[c.Number]
			if !ok {
				best[c.Number] = c
				order = append(order, c.Number)
				continue
			}
			if cur.Type != "official" && c.Type == "official" {
				best[c.Number] = c
			}
		}
	}

	out := make([]mangafireChapterItem, 0, len(order))
	for _, num := range order {
		out = append(out, best[num])
	}
	return out
}

// mangafireChapterItem is one entry of the chapters-list JSON feed
type mangafireChapterItem struct {
	Id       int64   `json:"id"`
	Number   float64 `json:"number"`
	Name     string  `json:"name"`
	Language string  `json:"language"`
	Type     string  `json:"type"` // "official" or "unofficial"
}

// mangafirePage is one entry of the reader's pages list
type mangafirePage struct {
	Url string `json:"url"`
}
