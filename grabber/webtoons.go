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
	"github.com/elboletaire/manga-downloader/http"
)

// Webtoons is a grabber for webtoons.com (WEBTOON, LINE's platform, Naver's
// own for the Korean originals). Plain HTTP only: both the episode list and
// the episode viewer are server-rendered, and the viewer carries every page
// of the episode in its HTML.
//
// The episode list comes from the mobile site's JSON API when it answers,
// which lists a whole series in one request. The HTML list is the fallback:
// it's paginated (9 episodes per page on the Tower of God list, 10 on canvas,
// the page size isn't fixed), and the pages past the last one repeat it
// instead of coming back empty, so pages are read until one adds no episode
// that wasn't seen already. Episode numbers come from the site's own
// episode number (the sequential episode_no the viewer URLs use), not from
// the "[Season N] Ep. X" titles, which restart with each season.
//
// Pages are served by the webtoon-phinf CDN, which refuses requests without a
// Referer of the site: the BaseUrl() referer the downloader sends covers it.
type Webtoons struct {
	*Grabber
	series *webtoonsSeries
}

func NewWebtoons(g *Grabber) *Webtoons {
	return &Webtoons{Grabber: g}
}

// WebtoonsChapter represents a Webtoons episode
type WebtoonsChapter struct {
	Chapter
	// URL is the absolute URL of the episode's viewer
	URL string
}

// webtoonsSeries is the whole episode list of a series, in the order its
// source lists it: oldest first from the API, newest first from the HTML list
type webtoonsSeries struct {
	title    string
	episodes []webtoonsEpisode
}

// webtoonsEpisode is one row of the episode list
type webtoonsEpisode struct {
	title  string
	number float64
	url    string
}

// webtoonsHostRe matches webtoons.com and its subdomains (www, m)
var webtoonsHostRe = regexp.MustCompile(`(^|\.)webtoons\.com$`)

// Test returns true if the URL is a webtoons.com series, list or episode URL.
// Any of them is rewritten to the series' episode list on www.webtoons.com,
// built from the /{lang}/{genre}/{slug} path and the title_no query, which
// is what every other URL the grabber builds is derived from.
func (w *Webtoons) Test() (bool, error) {
	uri, err := url.Parse(w.URL)
	if err != nil {
		return false, err
	}
	if !webtoonsHostRe.MatchString(uri.Hostname()) {
		return false, nil
	}

	titleNo := uri.Query().Get("title_no")
	segments := strings.Split(strings.Trim(uri.Path, "/"), "/")
	if titleNo == "" || len(segments) < 3 {
		return false, nil
	}

	w.URL = fmt.Sprintf("https://www.webtoons.com/%s/%s/%s/list?title_no=%s",
		segments[0], segments[1], segments[2], url.QueryEscape(titleNo))

	return true, nil
}

// FetchTitle fetches and returns the series title
func (w *Webtoons) FetchTitle() (string, error) {
	series, err := w.fetchSeries()
	if err != nil {
		return "", err
	}

	return series.title, nil
}

// webtoonsEpisodeID returns the ID of an episode from its viewer URL: the
// episode_no the site numbers it with. The API and HTML builds of the same
// episode's URL can differ in other params (readingLanguageCode), so the whole
// URL isn't the ID. Falls back to chapterURLID when there is no episode_no.
func webtoonsEpisodeID(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		if no := u.Query().Get("episode_no"); no != "" {
			return no
		}
	}

	return chapterURLID(rawURL)
}

// FetchChapters returns every episode of the series
func (w *Webtoons) FetchChapters() (chapters Filterables, errs []error) {
	series, err := w.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	for _, ep := range series.episodes {
		chapters = append(chapters, &WebtoonsChapter{
			Chapter{
				Title:  ep.title,
				Number: ep.number,
				ID:     webtoonsEpisodeID(ep.url),
			},
			ep.url,
		})
	}

	return
}

// FetchChapter fetches an episode and its pages
func (w *Webtoons) FetchChapter(f Filterable) (*Chapter, error) {
	wchap := f.(*WebtoonsChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     wchap.URL,
		Referer: w.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	pages := webtoonsPages(doc)
	// an episode the reader can't show (paid, or pulled) renders no pages
	// rather than an error, so an empty viewer must fail here instead of
	// packing an empty chapter
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for episode %v (locked or unavailable?): %s", f.GetNumber(), wchap.URL)
	}

	return &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Pages:      pages,
		ID:         webtoonsEpisodeID(wchap.URL),
	}, nil
}

// fetchSeries reads the series' title and its whole episode list, once
func (w *Webtoons) fetchSeries() (*webtoonsSeries, error) {
	if w.series != nil {
		return w.series, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:     w.URL,
		Referer: w.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	first, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	series := &webtoonsSeries{
		title: sanitizeTitle(first.Find("meta[property='og:title']").AttrOr("content", "")),
	}

	series.episodes, err = w.fetchEpisodesAPI()
	if err != nil || len(series.episodes) == 0 {
		// the API is the fast path; the HTML list is what's left when it
		// refuses the series (canvas ids, for one, aren't served by the
		// webtoon endpoint)
		series.episodes, err = w.walkEpisodeList(first)
		if err != nil {
			return nil, err
		}
	}

	if series.title == "" || len(series.episodes) == 0 {
		return nil, fmt.Errorf("no episodes found for %s", w.URL)
	}

	w.series = series

	return series, nil
}

// fetchEpisodesAPI lists the whole series from the mobile site's JSON API,
// following the cursor until the API reports no next one. The API only exists
// on the mobile host, and it answers with success: false for ids it doesn't
// serve, which the caller takes as a reason to walk the HTML list instead.
func (w *Webtoons) fetchEpisodesAPI() ([]webtoonsEpisode, error) {
	uri, err := url.Parse(w.URL)
	if err != nil {
		return nil, err
	}
	segments := strings.Split(strings.Trim(uri.Path, "/"), "/")
	if len(segments) < 3 {
		return nil, fmt.Errorf("unexpected webtoons URL %s", w.URL)
	}

	kind := "webtoon"
	if segments[1] == "canvas" {
		kind = "canvas"
	}
	titleNo := uri.Query().Get("title_no")

	return webtoonsWalkAPI(uri, func(cursor int64) (string, error) {
		return http.GetText(http.RequestParams{
			URL:     fmt.Sprintf("https://m.webtoons.com/api/v1/%s/%s/episodes?pageSize=1000&cursor=%d", kind, titleNo, cursor),
			Referer: w.BaseUrl(),
		})
	})
}

// webtoonsAPIMaxPages caps how many pages of the episode API are read, so a
// cursor that keeps moving on can't keep the grabber walking
const webtoonsAPIMaxPages = 100

// webtoonsWalkAPI lists the episodes of a series by following the cursor of
// the episode API until it reports no next one. fetch returns the body of the
// page at a cursor. A cursor that comes back is a cycle and ends the walk, as
// its episodes are already listed; running past webtoonsAPIMaxPages is an
// error, since stopping there would drop the rest of the series unnoticed.
func webtoonsWalkAPI(base *url.URL, fetch func(cursor int64) (string, error)) ([]webtoonsEpisode, error) {
	var episodes []webtoonsEpisode
	seen := map[int64]bool{}
	cursor := int64(0)
	for pages := 1; ; pages++ {
		seen[cursor] = true

		body, err := fetch(cursor)
		if err != nil {
			return nil, err
		}

		items, next, err := webtoonsAPIPage([]byte(body), base)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, items...)

		if next == 0 || seen[next] {
			break
		}
		if pages == webtoonsAPIMaxPages {
			return nil, fmt.Errorf("episode API has more than %d pages", webtoonsAPIMaxPages)
		}
		cursor = next
	}

	return episodes, nil
}

// webtoonsAPIResponse is one page of the episode API's answer
type webtoonsAPIResponse struct {
	Success bool `json:"success"`
	Result  *struct {
		EpisodeList []webtoonsAPIEpisode `json:"episodeList"`
		NextCursor  int64                `json:"nextCursor"`
	} `json:"result"`
}

// webtoonsAPIEpisode is one episode in the API's answer
type webtoonsAPIEpisode struct {
	EpisodeNo    float64 `json:"episodeNo"`
	EpisodeTitle string  `json:"episodeTitle"`
	ViewerLink   string  `json:"viewerLink"`
}

// webtoonsAPIPage returns the episodes of one page of the episode API and the
// cursor of the next one (0 when there's none). The viewer links are relative
// to the series URL they were listed from, so they're resolved against it.
func webtoonsAPIPage(body []byte, base *url.URL) ([]webtoonsEpisode, int64, error) {
	var resp webtoonsAPIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, err
	}
	if !resp.Success || resp.Result == nil {
		return nil, 0, fmt.Errorf("episode API refused the series")
	}

	var episodes []webtoonsEpisode
	for _, ep := range resp.Result.EpisodeList {
		link, err := url.Parse(ep.ViewerLink)
		if err != nil {
			return nil, 0, err
		}

		episodes = append(episodes, webtoonsEpisode{
			title:  sanitizeTitle(ep.EpisodeTitle),
			number: ep.EpisodeNo,
			url:    base.ResolveReference(link).String(),
		})
	}

	return episodes, resp.Result.NextCursor, nil
}

// walkEpisodeList reads the HTML episode list page by page, starting from the
// page already fetched, until a page adds no episode that wasn't seen before
func (w *Webtoons) walkEpisodeList(first *goquery.Document) ([]webtoonsEpisode, error) {
	var episodes []webtoonsEpisode
	seen := map[float64]bool{}
	for page := 1; ; page++ {
		doc := first
		if page > 1 {
			body, err := http.GetText(http.RequestParams{
				URL:     fmt.Sprintf("%s&page=%d", w.URL, page),
				Referer: w.BaseUrl(),
			})
			if err != nil {
				return nil, err
			}

			doc, err = goquery.NewDocumentFromReader(strings.NewReader(body))
			if err != nil {
				return nil, err
			}
		}

		added := 0
		for _, ep := range webtoonsEpisodes(doc) {
			if seen[ep.number] {
				continue
			}
			seen[ep.number] = true
			episodes = append(episodes, ep)
			added++
		}
		if added == 0 {
			break
		}
	}

	return episodes, nil
}

// webtoonsEpisodes returns the episodes listed on one page of the episode
// list, in the order the page shows them
func webtoonsEpisodes(doc *goquery.Document) []webtoonsEpisode {
	var episodes []webtoonsEpisode
	doc.Find("#_listUl li._episodeItem").Each(func(_ int, li *goquery.Selection) {
		number, err := strconv.ParseFloat(li.AttrOr("data-episode-no", ""), 64)
		if err != nil {
			return
		}

		href := li.Find("a.detail_list_link").AttrOr("href", "")
		if href == "" {
			return
		}

		episodes = append(episodes, webtoonsEpisode{
			title:  sanitizeTitle(li.Find("span.subj > span").First().Text()),
			number: number,
			url:    href,
		})
	})

	return episodes
}

// webtoonsPages returns the pages of an episode viewer. The reader's images
// are the ones inside #_imageList; the sidebar's thumbnails of other
// episodes are a different class and are left out.
func webtoonsPages(doc *goquery.Document) []Page {
	var pages []Page
	doc.Find("#_imageList img._images").Each(func(_ int, img *goquery.Selection) {
		src := img.AttrOr("data-url", "")
		if src == "" {
			return
		}

		pages = append(pages, Page{
			Number: int64(len(pages) + 1),
			URL:    src,
		})
	})

	return pages
}
