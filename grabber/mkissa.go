// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
)

// Mkissa is a grabber for mkissa.to. It's a SvelteKit SPA backed by a GraphQL
// API (Apollo persisted queries) at api.mkissa.net: the manga info + full
// chapter-number list are wide open to plain HTTP, no cookies/browser needed.
// The reader route, however, answers with a Cloudflare challenge on *direct*
// navigation (even after warming up cookies from the series page in the same
// browser context) but loads fine through the app's own client-side routing
// (clicking a chapter link from the already-loaded series page never issues
// a real top-level navigation to the blocked route). The reader also decodes
// its page-image list from an encrypted API blob client-side (their own
// "tobeparsed" anti-scrape layer, same idea as sakuramangas' AetherCipher),
// but each page's <img> resolves to a plain, directly-downloadable JPEG URL
// once scrolled into view, so no cipher reverse-engineering is needed either.
type Mkissa struct {
	*Grabber
	id   string
	info *mkissaMangaInfo
}

func NewMkissa(g *Grabber) *Mkissa {
	return &Mkissa{Grabber: g}
}

// MkissaChapter represents a Mkissa chapter
type MkissaChapter struct {
	Chapter
	MangaId   string
	NumberStr string
}

// mkissaIdRe matches mkissa.to series URLs, e.g.
// https://mkissa.to/manga/eaYBWW65WLabNiLEi
var mkissaIdRe = regexp.MustCompile(`mkissa\.to/manga/([A-Za-z0-9]+)`)

// Test returns true if the URL is a mkissa.to series URL
func (m *Mkissa) Test() (bool, error) {
	return mkissaIdRe.MatchString(m.URL), nil
}

// mangaId returns the manga id parsed from the series URL
func (m *Mkissa) mangaId() (string, error) {
	if m.id != "" {
		return m.id, nil
	}
	match := mkissaIdRe.FindStringSubmatch(m.URL)
	if len(match) != 2 {
		return "", fmt.Errorf("could not find manga id in url %s", m.URL)
	}
	m.id = match[1]
	return m.id, nil
}

// mkissaAPI is the GraphQL endpoint (Apollo persisted queries: only a query
// hash is sent, no query text, reusing whatever the site's own frontend
// already registered server-side)
const mkissaAPI = "https://api.mkissa.net/api"

// mkissaInfoQueryHash is the persisted query hash for the manga info query
// (series title + full per-translation chapter number list)
const mkissaInfoQueryHash = "f2678aedf3d265af9ba482e9a20285aa2cfecfd55233fd2643971c2f658784bd"

// mkissaMangaInfo is the relevant subset of the manga info API response
type mkissaMangaInfo struct {
	Name                    string `json:"name"`
	EnglishName             string `json:"englishName"`
	AvailableChaptersDetail struct {
		Sub []string `json:"sub"`
	} `json:"availableChaptersDetail"`
}

// fetchInfo fetches and caches the manga info (title + chapter list)
func (m *Mkissa) fetchInfo() (*mkissaMangaInfo, error) {
	if m.info != nil {
		return m.info, nil
	}

	id, err := m.mangaId()
	if err != nil {
		return nil, err
	}

	variables := fmt.Sprintf(`{"_id":%q,"search":{"allowAdult":false,"allowUnknown":false}}`, id)
	extensions := fmt.Sprintf(`{"persistedQuery":{"version":1,"sha256Hash":%q}}`, mkissaInfoQueryHash)

	q := url.Values{}
	q.Set("variables", variables)
	q.Set("extensions", extensions)

	body, err := http.GetText(http.RequestParams{
		URL:     mkissaAPI + "?" + q.Encode(),
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	feed := struct {
		Data struct {
			Manga mkissaMangaInfo `json:"manga"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	m.info = &feed.Data.Manga

	return m.info, nil
}

// FetchTitle fetches and returns the manga title
func (m *Mkissa) FetchTitle() (string, error) {
	info, err := m.fetchInfo()
	if err != nil {
		return "", err
	}

	title := info.EnglishName
	if title == "" {
		title = info.Name
	}

	return sanitizeTitle(title), nil
}

// FetchChapters returns the chapters of the manga (translated/"sub" only)
func (m *Mkissa) FetchChapters() (chapters Filterables, errs []error) {
	info, err := m.fetchInfo()
	if err != nil {
		return nil, []error{err}
	}
	id, err := m.mangaId()
	if err != nil {
		return nil, []error{err}
	}

	for _, numStr := range info.AvailableChaptersDetail.Sub {
		number, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			continue
		}
		chapters = append(chapters, &MkissaChapter{
			Chapter{
				Number:   number,
				Title:    "Chapter " + numStr,
				Language: "en",
			},
			id,
			numStr,
		})
	}

	return
}

// mkissa reader selectors (see browser.GetReaderHTML for how these are used)
const (
	mkissaTabSelector        = `button[aria-label*="Chapters"]`
	mkissaPaginationSelector = `.media-ep-list__pagination button`
	mkissaImgSelector        = `img.reader-page__img`
)

// FetchChapter fetches a chapter and its pages
func (m *Mkissa) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MkissaChapter)

	href := fmt.Sprintf("/manga/%s/chapter-%s-sub", mchap.MangaId, mchap.NumberStr)
	linkSelector := fmt.Sprintf(`div[data-href=%q]`, href)
	// unique to this chapter's page image URLs, e.g. "/eaYBWW65WLabNiLEi/59/";
	// used both to know when scrolling has resolved every page of *this*
	// chapter (some mkissa readers auto-continue into the next one) and to
	// filter out any stray next-chapter images from the final result
	urlSubstr := fmt.Sprintf("/%s/%s/", mchap.MangaId, mchap.NumberStr)

	html, err := browser.GetReaderHTML(
		m.URL, mkissaTabSelector, mkissaPaginationSelector, linkSelector,
		mkissaImgSelector, urlSubstr, 0,
	)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
	}

	doc.Find(mkissaImgSelector).Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" || !strings.Contains(src, urlSubstr) {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
