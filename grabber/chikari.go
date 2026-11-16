// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

// chikariAPIBase is the JSON api of chikari.moe, an English SvelteKit reader
// whose api is open to plain HTTP. A var so the tests can point it at a server.
var chikariAPIBase = "https://chikari.moe/api"

// chikariAPIInterval spaces every api call. The site answers 429 with a
// verification challenge (Cloudflare Turnstile) after about thirty calls in a
// minute, and blocks for ten minutes or more after that; one call every three
// seconds never triggered it.
const chikariAPIInterval = 3 * time.Second

// chikariAPIPacer spaces the api calls of every Chikari instance, since the
// limit is per IP, not per download. Page images aren't rate limited.
var chikariAPIPacer = &pacer{interval: chikariAPIInterval}

// chikariPageSize is the chapter listing's limit per request
const chikariPageSize = 500

// errChikariChallenge is returned once the api asks for a verification. The
// block outlasts any retry loop, so calls stop going out for the rest of the run.
var errChikariChallenge = errors.New("chikari.moe asked for a verification (429 challenge_required): wait about ten minutes before retrying, calling the site again only extends the block")

// Chikari is a grabber for chikari.moe, a SvelteKit reader whose JSON api
// serves the series, its chapter listing and each chapter's page list. Every
// scanlation group's release of a chapter is listed on its own.
type Chikari struct {
	*Grabber
	// challenged is set once the api answers 429, so later calls fail at once
	challenged atomic.Bool
}

func NewChikari(g *Grabber) *Chikari {
	return &Chikari{Grabber: g}
}

// ChikariChapter is one scanlation group's release of a chapter. Its ID is the
// group's scan slug, the scan a chapter's pages are requested with
type ChikariChapter struct {
	Chapter
}

// chikariDomainRe matches the chikari.moe site, with or without www.
var chikariDomainRe = regexp.MustCompile(`^https?://(?:www\.)?chikari\.moe(?:[/?#]|$)`)

// chikariReaderRe matches a chapter reader link, /series/{slug}/{number}, with
// the number a whole or decimal one (52.5)
var chikariReaderRe = regexp.MustCompile(`^/series/([^/]+)/[0-9]+(?:\.[0-9]+)?/?$`)

// chikariSeriesRe matches a series page, /series/{slug}
var chikariSeriesRe = regexp.MustCompile(`^/series/([^/]+)/?$`)

// Test returns true if the URL is a chikari.moe URL. It decides from the
// domain alone, so it never fetches
func (c *Chikari) Test() (bool, error) {
	return chikariDomainRe.MatchString(c.URL), nil
}

// resolveSeriesURL rewrites a reader link to its series page. No-op otherwise.
func (c *Chikari) resolveSeriesURL() {
	u, err := url.Parse(c.URL)
	if err != nil {
		return
	}
	m := chikariReaderRe.FindStringSubmatch(u.Path)
	if m == nil {
		return
	}
	u.Path = "/series/" + m[1]
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	c.URL = u.String()
}

// chikariSlug returns the series slug of the (resolved) series URL
func (c *Chikari) chikariSlug() (string, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return "", errSeriesNotFound(c.URL)
	}
	m := chikariSeriesRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", errSeriesNotFound(c.URL)
	}
	return m[1], nil
}

// FetchTitle fetches and returns the series title
func (c *Chikari) FetchTitle() (string, error) {
	c.resolveSeriesURL()
	slug, err := c.chikariSlug()
	if err != nil {
		return "", err
	}

	body, err := c.apiGet(chikariAPIBase+"/series/"+url.PathEscape(slug), c.URL)
	if err != nil {
		return "", err
	}

	series := struct {
		Title string `json:"title"`
	}{}
	if err := json.Unmarshal([]byte(body), &series); err != nil {
		return "", err
	}

	return sanitizeTitle(series.Title), nil
}

// chikariListing is one page of the chapter listing
type chikariListing struct {
	Items []chikariListed `json:"items"`
	Total int             `json:"total"`
}

// chikariListed is a chapter in the listing: one entry per chapter, whose
// scans are the groups that released it
type chikariListed struct {
	Number *float64   `json:"number"`
	Volume jsonVolume `json:"volume"`
	Title  string     `json:"title"`
	Lang   string     `json:"lang"`
	Scans  []struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"scans"`
}

// FetchChapters returns one chapter per scanlation group's release of each
// chapter, the group named in Chapter.Group so --scanlator can pick among them
func (c *Chikari) FetchChapters() (chapters Filterables, errs []error) {
	c.resolveSeriesURL()
	slug, err := c.chikariSlug()
	if err != nil {
		return nil, []error{err}
	}

	listed := []chikariListed{}
	for offset := 0; ; {
		body, err := c.apiGet(fmt.Sprintf("%s/series/%s/chapters?limit=%d&offset=%d&order=asc",
			chikariAPIBase, url.PathEscape(slug), chikariPageSize, offset), c.URL)
		if err != nil {
			return nil, []error{err}
		}
		page := chikariListing{}
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			return nil, []error{err}
		}
		listed = append(listed, page.Items...)
		offset += len(page.Items)
		if len(page.Items) == 0 || offset >= page.Total {
			break
		}
	}

	for _, item := range listed {
		if item.Number == nil {
			errs = append(errs, fmt.Errorf("chapter %q has no number", item.Title))
			continue
		}
		language := item.Lang
		if language == "" {
			language = "en"
		}

		// a chapter no scan has released yet is listed once, unnamed
		releases := item.Scans
		if len(releases) == 0 {
			releases = []struct {
				Slug string `json:"slug"`
				Name string `json:"name"`
			}{{}}
		}
		for _, scan := range releases {
			chapter := &ChikariChapter{Chapter{
				Title:    item.Title,
				Number:   *item.Number,
				Language: language,
				ID:       scan.Slug,
				Volume:   int(item.Volume),
				Label:    Label{Style: LabelNumbered},
			}}
			chapter.SetGroups(scan.Name)
			chapters = append(chapters, chapter)
		}
	}

	return chapters, errs
}

// chikariPages is the page list of one release of a chapter
type chikariPages struct {
	Pages []string `json:"pages"`
	// ScanFallback is set when the requested scan has no pages and the site
	// serves another one instead
	ScanFallback bool `json:"scan_fallback"`
	// ClubBlock is set when a club-gated chapter can't be read; it's null otherwise
	ClubBlock any `json:"club_block"`
}

// FetchChapter fetches the pages of one scanlation group's release
func (c *Chikari) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*ChikariChapter)
	c.resolveSeriesURL()
	slug, err := c.chikariSlug()
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("%s/series/%s/chapters/%s", chikariAPIBase, url.PathEscape(slug),
		strconv.FormatFloat(chap.Number, 'f', -1, 64))
	if chap.ID != "" {
		uri += "?scan=" + url.QueryEscape(chap.ID)
	}
	body, err := c.apiGet(uri, c.URL)
	if err != nil {
		return nil, err
	}

	pages := chikariPages{}
	if err := json.Unmarshal([]byte(body), &pages); err != nil {
		return nil, err
	}
	if pages.ClubBlock != nil && pages.ClubBlock != false {
		return nil, fmt.Errorf("chapter %s is club-gated (club_block): the site reads it only for club members", f.GetTitle())
	}
	if pages.ScanFallback {
		return nil, fmt.Errorf("chapter %s has no pages from the scan %q, the site would serve another one instead", f.GetTitle(), chap.ID)
	}
	if len(pages.Pages) == 0 {
		return nil, fmt.Errorf("chapter %s has no pages", f.GetTitle())
	}

	chapter := &Chapter{
		Title:      chap.GetTitle(),
		Label:      labelOf(chap),
		Number:     chap.GetNumber(),
		PagesCount: int64(len(pages.Pages)),
		Language:   chap.Language,
		ID:         chap.ID,
		Volume:     chap.Volume,
	}
	chapter.SetGroups(chap.GetGroups()...)
	for i, u := range pages.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    u,
		})
	}

	return chapter, nil
}

// apiGet is the one way the api is called: paced to chikariAPIInterval and
// tried once. A 429 is the verification gate, which a retry only extends, so
// it's reported and every later call is refused without going out
func (c *Chikari) apiGet(uri, referer string) (string, error) {
	if c.challenged.Load() {
		return "", errChikariChallenge
	}

	chikariAPIPacer.wait()
	body, err := http.GetText(http.RequestParams{
		URL:          uri,
		Referer:      referer,
		Accept:       "application/json",
		DisableRetry: true,
	})

	var statusErr *http.StatusError
	if errors.As(err, &statusErr) && statusErr.Code == 429 {
		c.challenged.Store(true)
		return "", errChikariChallenge
	}

	return body, err
}
