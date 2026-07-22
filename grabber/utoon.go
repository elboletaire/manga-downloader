// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Utoon is a grabber for utoon.us: a WordPress "mangaverse" theme site (the
// successor of the now offline reset-scans / utoon.net). The series page
// only renders the ~10 most-recent chapters; the rest is loaded via a
// paginated wp-admin/admin-ajax.php "mangaverse_load_more" action, using a
// category id and a per-page nonce scraped from the series page itself.
type Utoon struct {
	*Grabber
}

func NewUtoon(g *Grabber) *Utoon {
	return &Utoon{Grabber: g}
}

// UtoonChapter is a chapter for Utoon
type UtoonChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a utoon.us URL
func (u *Utoon) Test() (bool, error) {
	re := regexp.MustCompile(`utoon\.us`)
	return re.MatchString(u.URL), nil
}

// FetchTitle fetches and returns the manga title
func (u Utoon) FetchTitle() (string, error) {
	doc, err := u.seriesDoc()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(doc.Find("h1.series-title").First().Text()), nil
}

// utoonAjaxConfigRe extracts the ajax url and per-page nonce the theme's own
// JS uses to call admin-ajax.php (var mangaverse_ajax = {"ajax_url":"...","nonce":"..."})
var utoonAjaxConfigRe = regexp.MustCompile(`var mangaverse_ajax = \{"ajax_url":"([^"]+)","nonce":"([^"]+)"`)

// FetchChapters returns the chapters of the manga, starting with the ones
// already rendered in the series page and paginating through the theme's
// "load more" ajax endpoint until it reports no more pages. Some duplicate
// entries (same chapter URL) turn up in the tail pages due to what looks
// like an off-by-one in the theme's own pagination query, so entries are
// deduplicated by URL as they're collected.
func (u Utoon) FetchChapters() (chapters Filterables, errs []error) {
	doc, err := u.seriesDoc()
	if err != nil {
		return nil, []error{err}
	}

	html, _ := doc.Html()
	m := utoonAjaxConfigRe.FindStringSubmatch(html)
	if len(m) < 3 {
		errs = append(errs, fmt.Errorf("could not find mangaverse ajax config in %s", u.URL))
		return
	}
	ajaxURL, nonce := m[1], m[2]

	list := doc.Find(".chapters-list")
	categoryID := list.AttrOr("data-category", "")
	if categoryID == "" {
		errs = append(errs, fmt.Errorf("could not find chapter category id in %s", u.URL))
		return
	}

	seen := map[string]bool{}
	collect := func(sel *goquery.Selection) {
		sel.Find(".chapter-item").Each(func(i int, s *goquery.Selection) {
			link := s.Find("a.chapter-link")
			href := link.AttrOr("href", "")
			if href == "" || seen[href] {
				return
			}
			title := sanitizeTitle(link.Find(".chapter-title").Text())
			number, ok := parseChapterNumber(title)
			if !ok {
				return
			}
			seen[href] = true
			chapters = append(chapters, &UtoonChapter{
				Chapter{Title: title, Number: number},
				href,
			})
		})
	}

	// the ~10 most recent chapters are already rendered server-side
	collect(list)

	for page := 2; ; page++ {
		body := fmt.Sprintf(
			"action=mangaverse_load_more&nonce=%s&page=%d&type=series&category_id=%s&order=desc&lang=en",
			url.QueryEscape(nonce), page, url.QueryEscape(categoryID),
		)
		resp, err := http.PostText(http.RequestParams{
			URL:     ajaxURL,
			Referer: u.URL,
			Body:    body,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		feed := utoonLoadMoreResponse{}
		if err = json.Unmarshal([]byte(resp), &feed); err != nil {
			errs = append(errs, err)
			return
		}
		if strings.TrimSpace(feed.Data.HTML) == "" {
			return
		}

		frag, err := goquery.NewDocumentFromReader(strings.NewReader(feed.Data.HTML))
		if err != nil {
			errs = append(errs, err)
			return
		}
		collect(frag.Selection)

		if !feed.Data.HasMore {
			return
		}
	}
}

// FetchChapter fetches a chapter and its pages
func (u Utoon) FetchChapter(f Filterable) (*Chapter, error) {
	uchap := f.(*UtoonChapter)

	body, err := http.Get(http.RequestParams{
		URL:     uchap.URL,
		Referer: u.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
	}

	pimages := getPlainHTMLImageURL(".entry-content img", doc)
	for i, img := range pimages {
		if img == "" {
			continue
		}
		if !strings.HasPrefix(img, "http") {
			img = u.BaseUrl() + img
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    img,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// seriesDoc fetches and parses the series page
func (u Utoon) seriesDoc() (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{
		URL: u.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// utoonLoadMoreResponse is the JSON response of the mangaverse_load_more ajax action
type utoonLoadMoreResponse struct {
	Success bool `json:"success"`
	Data    struct {
		HTML    string `json:"html"`
		HasMore bool   `json:"has_more"`
	} `json:"data"`
}
