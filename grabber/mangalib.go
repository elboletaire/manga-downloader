// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// mangalibApi is the JSON api host for mangalib.me. The site itself
// (mangalib.me) is a Vue/Nuxt SPA sitting behind a passive DDoS-Guard (it
// just sets cookies, it doesn't challenge plain GETs), but both the chapters
// list and the chapter pages are served anonymously by this separate api
// host, no cloudflare/browser needed.
const mangalibApi = "https://api2.mangalib.me/api"

// mangalibImageServer is the CDN base url for page images (mangalib.me is
// site id 1 on the api's /constants?fields[]=imageServers endpoint, whose
// "main"/"secondary" entries both currently point here). Images must be
// requested with a Referer but *without* an Origin header, or the CDN 403s -
// see http.RequestParams.Origin, which mangalib deliberately never sets.
const mangalibImageServer = "https://img2.imglib.info"

// Mangalib is a grabber for mangalib.me
type Mangalib struct {
	*Grabber
	title string
}

func NewMangalib(g *Grabber) *Mangalib {
	return &Mangalib{Grabber: g}
}

// MangalibChapter represents a Mangalib Chapter
type MangalibChapter struct {
	Chapter
	// Volume and Num are the raw "volume"/"number" fields as returned by the
	// chapters list api, needed verbatim to requery the single chapter (the
	// parsed Number float64 isn't precise/safe enough to rebuild them, e.g.
	// leading zeros or non-numeric suffixes some series use for extras).
	Volume string
	Num    string
}

// Test returns true if the URL is a mangalib.me URL
func (m *Mangalib) Test() (bool, error) {
	re := regexp.MustCompile(`mangalib\.me`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangalib) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	slug, err := m.seriesSlug()
	if err != nil {
		return "", err
	}

	body, err := http.GetText(http.RequestParams{
		URL:     mangalibApi + "/manga/" + slug,
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return "", err
	}

	feed := struct {
		Data struct {
			Name    string `json:"name"`
			RusName string `json:"rus_name"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return "", err
	}

	title := feed.Data.RusName
	if title == "" {
		title = feed.Data.Name
	}
	m.title = sanitizeTitle(title)

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m Mangalib) FetchChapters() (chapters Filterables, errs []error) {
	slug, err := m.seriesSlug()
	if err != nil {
		return nil, []error{err}
	}

	body, err := http.GetText(http.RequestParams{
		URL:     mangalibApi + "/manga/" + slug + "/chapters",
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return nil, []error{err}
	}

	feed := mangalibChaptersFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, []error{err}
	}

	for _, c := range feed.Data {
		num, err := strconv.ParseFloat(c.Number, 64)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		chapters = append(chapters, &MangalibChapter{
			Chapter{
				Number:   num,
				Title:    c.Name,
				Language: "ru",
			},
			c.Volume,
			c.Number,
		})
	}

	return chapters, errs
}

// FetchChapter fetches a chapter and its pages
func (m Mangalib) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangalibChapter)
	slug, err := m.seriesSlug()
	if err != nil {
		return nil, err
	}

	// The api resolves a bare number+volume query to a specific
	// translation team's chapter (its "branch") when a chapter has more than
	// one; it picks the same one (the earliest/first-listed team) that the
	// site itself opens by default when a reader doesn't pick a group, so we
	// don't need to resolve/pass a branch_id ourselves.
	params := url.Values{}
	params.Add("number", mchap.Num)
	params.Add("volume", mchap.Volume)

	body, err := http.GetText(http.RequestParams{
		URL:     fmt.Sprintf("%s/manga/%s/chapter?%s", mangalibApi, slug, params.Encode()),
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	feed := struct {
		Data struct {
			Pages []struct {
				Url string `json:"url"`
			} `json:"pages"`
		} `json:"data"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   "ru",
		PagesCount: int64(len(feed.Data.Pages)),
	}
	for i, p := range feed.Data.Pages {
		// p.Url is a server-relative path starting with "//" (e.g.
		// "//manga/one-piece/chapters/123/abc.jpg"); it is *not* a
		// protocol-relative URL (there's no host after the "//"), it's just
		// concatenated directly onto the image server's base url.
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    mangalibImageServer + p.Url,
		})
	}

	return chapter, nil
}

// seriesSlug returns the manga slug from the series URL. Both the
// "{id}--{slug}" form (https://mangalib.me/ru/manga/206--one-piece) and the
// bare slug form (https://mangalib.me/manga/one-piece) work as-is against
// the api, so it's returned verbatim.
func (m Mangalib) seriesSlug() (string, error) {
	re := regexp.MustCompile(`/manga/([^/?#]+)`)
	matches := re.FindStringSubmatch(m.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find manga slug in url %s", m.URL)
	}
	return matches[1], nil
}

// mangalibChaptersFeed is the JSON feed for the chapters list. Each entry is
// one unique (volume, number) chapter - translation-team alternatives are
// nested under "branches" and not exposed here, since the plain chapter
// endpoint already resolves to a sensible default (see FetchChapter).
type mangalibChaptersFeed struct {
	Data []struct {
		Volume string `json:"volume"`
		Number string `json:"number"`
		Name   string `json:"name"`
	} `json:"data"`
}
