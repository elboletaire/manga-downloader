// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
)

// Coffeemanga is a grabber for coffeemanga.net, an Inertia.js app. The title
// and listing are plain HTTP, but the reader's images come from its own script
// after an encrypted manifest, so FetchChapter renders the reader in a browser.
type Coffeemanga struct {
	*Grabber
}

// NewCoffeemanga returns a new Coffeemanga grabber
func NewCoffeemanga(g *Grabber) *Coffeemanga {
	return &Coffeemanga{Grabber: g}
}

// CoffeemangaChapter is a chapter of coffeemanga.net, from its listing
type CoffeemangaChapter struct {
	Chapter
	URL string
}

// coffeemangaSeriesRe matches a series or reader URL and captures the series URL
var coffeemangaSeriesRe = regexp.MustCompile(`(?i)^(https?://[^/]+/manga/[^/?#]+)`)

// coffeemangaPayloadRe captures the Inertia payload every page carries
var coffeemangaPayloadRe = regexp.MustCompile(`(?s)<script data-page="app" type="application/json">(.*?)</script>`)

// coffeemangaPayload is the Inertia payload of a page: the component it renders and its props
type coffeemangaPayload struct {
	Component string `json:"component"`
	Props     struct {
		Series *struct {
			Title string `json:"title"`
		} `json:"series"`
		// Total is the reader's page count (reader pages only)
		Total int `json:"total"`
	} `json:"props"`
}

// coffeemangaPayloadOf decodes the Inertia payload of a page
func coffeemangaPayloadOf(body string) (coffeemangaPayload, error) {
	var payload coffeemangaPayload
	m := coffeemangaPayloadRe.FindStringSubmatch(body)
	if m == nil {
		return payload, errors.New("no Inertia payload on the page")
	}
	err := json.Unmarshal([]byte(m[1]), &payload)
	return payload, err
}

// Test returns true if the URL is a series or reader page of coffeemanga.net,
// matched by the page's Inertia payload. It fetches the page once (shared with
// the other markup testers), and only for a /manga/ URL.
func (c *Coffeemanga) Test() (bool, error) {
	if !coffeemangaSeriesRe.MatchString(c.URL) {
		return false, nil
	}
	body, err := c.identifyPage(c.URL)
	if err != nil {
		return false, nil
	}
	payload, err := coffeemangaPayloadOf(body)
	if err != nil {
		return false, nil
	}
	isSite := payload.Component == "Catalog/Series" || strings.HasPrefix(payload.Component, "Reader/")
	return isSite && payload.Props.Series != nil, nil
}

// resolveSeriesURL rewrites a reader URL to its series URL. Series URLs are unchanged.
func (c *Coffeemanga) resolveSeriesURL() {
	if m := coffeemangaSeriesRe.FindStringSubmatch(c.URL); m != nil {
		c.URL = m[1]
	}
}

// FetchTitle returns the manga title from the series page's payload
func (c *Coffeemanga) FetchTitle() (string, error) {
	c.resolveSeriesURL()
	body, err := http.GetText(http.RequestParams{URL: c.URL + "/", Referer: c.BaseUrl()})
	if err != nil {
		return "", fmt.Errorf("coffeemanga title %s: %w", c.URL, err)
	}
	payload, err := coffeemangaPayloadOf(body)
	if err != nil {
		return "", fmt.Errorf("coffeemanga title %s: %w", c.URL, err)
	}
	if payload.Props.Series == nil {
		return "", fmt.Errorf("coffeemanga title %s: no series on the page", c.URL)
	}

	return sanitizeTitle(payload.Props.Series.Title), nil
}

// coffeemangaListing is one page of the chapter listing
type coffeemangaListing struct {
	Items []struct {
		URL    string `json:"url"`
		Number string `json:"number"`
		Title  string `json:"title"`
		Locked bool   `json:"locked"`
	} `json:"items"`
	// Last is the number of the last listing page
	Last int `json:"last"`
}

// fetchListing fetches one page of the chapter listing. chapters.json (page 1
// alone) lacks each chapter's number and URL, so every page is chapters/{page}.json
func (c *Coffeemanga) fetchListing(page int) (coffeemangaListing, error) {
	var listing coffeemangaListing
	url := fmt.Sprintf("%s/chapters/%d.json", c.URL, page)
	body, err := http.GetText(http.RequestParams{URL: url, Referer: c.URL + "/"})
	if err != nil {
		return listing, fmt.Errorf("coffeemanga listing %s: %w", url, err)
	}

	err = json.Unmarshal([]byte(body), &listing)
	if err != nil {
		return listing, fmt.Errorf("coffeemanga listing %s: %w", url, err)
	}
	return listing, nil
}

// FetchChapters returns the chapters of the manga, walking every listing page
func (c *Coffeemanga) FetchChapters() (Filterables, []error) {
	c.resolveSeriesURL()

	var chapters Filterables
	for page := 1; ; page++ {
		listing, err := c.fetchListing(page)
		if err != nil {
			return nil, []error{err}
		}

		for _, item := range listing.Items {
			number, err := strconv.ParseFloat(item.Number, 64)
			if err != nil {
				continue
			}
			chapters = append(chapters, &CoffeemangaChapter{
				Chapter: Chapter{
					Number:   number,
					Title:    sanitizeTitle(item.Title),
					Language: "en",
					Label:    Label{Style: LabelNumbered},
					Locked:   item.Locked,
				},
				URL: c.BaseUrl() + item.URL,
			})
		}

		if page >= listing.Last {
			return chapters, nil
		}
	}
}

// coffeemangaPageSelector matches the reader's page images, whose real URL the
// script fills in after a placeholder src
const coffeemangaPageSelector = `img[alt^="Page "]`

// coffeemangaReadyCountJS counts the reader's pages that have their real URL.
// It's the scroll's Progress, and coffeemangaReadyJS compares it to the total
const coffeemangaReadyCountJS = `(function(){
	var ready = 0;
	document.querySelectorAll('img[alt^="Page "]').forEach(function(img){
		if (/^https?:/.test(img.getAttribute('srcset') || img.getAttribute('src') || '')) ready++;
	});
	return ready;
})()`

// coffeemangaReadyJS holds once every page of the reader has its real URL,
// counted against the reader's own total
var coffeemangaReadyJS = `(function(){
	var page = document.querySelector('script[data-page="app"]');
	var total = page ? JSON.parse(page.textContent).props.total : 0;
	return total > 0 && ` + coffeemangaReadyCountJS + ` >= total;
})()`

// FetchChapter renders the reader and returns its pages
func (c *Coffeemanga) FetchChapter(f Filterable) (*Chapter, error) {
	cchap, ok := f.(*CoffeemangaChapter)
	if !ok {
		return nil, errors.New("invalid chapter type")
	}

	body, err := browser.GetHTMLWithScroll(cchap.URL, coffeemangaPageSelector, browser.Scroll{
		Done:     coffeemangaReadyJS,
		Progress: coffeemangaReadyCountJS,
		Pause:    500 * time.Millisecond,
	}, 0)
	if err != nil {
		return nil, fmt.Errorf("chapter %s: %w", cchap.URL, err)
	}

	payload, err := coffeemangaPayloadOf(body)
	if err != nil {
		return nil, fmt.Errorf("chapter %s: %w", cchap.URL, err)
	}
	urls, err := coffeemangaPageURLs(body)
	if err != nil {
		return nil, fmt.Errorf("chapter %s: %w", cchap.URL, err)
	}
	if len(urls) == 0 || len(urls) < payload.Props.Total {
		return nil, fmt.Errorf("chapter %s: only %d of %d pages loaded in the reader", cchap.URL, len(urls), payload.Props.Total)
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "en",
	}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// coffeemangaPageURLs returns the real image URLs of a rendered reader, in
// order. A page's URL is in its srcset (src keeps the placeholder), and a
// page still on the placeholder has no URL yet, so it's skipped.
func coffeemangaPageURLs(body string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	var urls []string
	doc.Find(coffeemangaPageSelector).Each(func(_ int, s *goquery.Selection) {
		candidates := s.AttrOr("srcset", "")
		if candidates == "" {
			candidates = s.AttrOr("src", "")
		}
		first, _, _ := strings.Cut(strings.TrimSpace(candidates), " ")
		if strings.HasPrefix(first, "http") {
			urls = append(urls, first)
		}
	})

	return urls, nil
}
