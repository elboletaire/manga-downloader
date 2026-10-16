// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Roliascan is a grabber for roliascan.com, a WordPress site (mangapeak
// theme) that loads both the chapters list and the chapter page images from
// its own JSON endpoints. The chapters endpoint is guarded by a client-side
// "anti-scraping" token that turns out to be reproducible: it's just
// md5(unixTimestamp + "mng_ch_" + currentUTCHourYYYYMMDDHH) truncated to 16
// hex chars, computed in roliascanToken(). The chapter-content endpoint and
// the resulting page images (served from a separate roliascan.org storage
// subdomain) need no token or cookies at all.
type Roliascan struct {
	*Grabber
	title   string
	mangaId string
	// languages are the languages of every chapter the series lists, before
	// the --language filter
	languages []string
}

func NewRoliascan(g *Grabber) *Roliascan {
	return &Roliascan{Grabber: g}
}

// LanguageOptions lists the languages the series has chapters in. The chapters
// are only fetched once FetchChapters has read them, so this is valid after it.
func (m *Roliascan) LanguageOptions() FilterOptions {
	return FilterOptions{
		Values: uniqueValues(m.languages),
		All:    true,
	}
}

// RoliascanChapter represents a Roliascan Chapter
type RoliascanChapter struct {
	Chapter
	Id string
}

// roliascanReaderRe matches reader URLs like /read/{series-slug}/...
var roliascanReaderRe = regexp.MustCompile(`(?i)/read/([^/?#]+)/`)

// Test returns true if the URL is a roliascan.com URL
func (m *Roliascan) Test() (bool, error) {
	re := regexp.MustCompile(`roliascan\.com`)
	return re.MatchString(m.URL), nil
}

// resolveSeriesURL rewrites a reader URL (/read/{slug}/...) to the series URL
// (/manga/{slug}/). No-op for series URLs.
func (m *Roliascan) resolveSeriesURL() {
	matches := roliascanReaderRe.FindStringSubmatch(m.URL)
	if matches == nil {
		return
	}
	u, err := url.Parse(m.URL)
	if err != nil {
		return
	}
	u.Path = "/manga/" + matches[1] + "/"
	u.RawQuery = ""
	u.Fragment = ""
	m.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (m *Roliascan) FetchTitle() (string, error) {
	m.resolveSeriesURL()
	if m.title == "" {
		if err := m.fetchSeriesPage(); err != nil {
			return "", err
		}
	}

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Roliascan) FetchChapters() (Filterables, []error) {
	m.resolveSeriesURL()
	if m.mangaId == "" {
		if err := m.fetchSeriesPage(); err != nil {
			return nil, []error{err}
		}
	}

	language := m.Settings.Language
	if language == "" {
		language = "en"
	}

	var chapters Filterables
	offset := 0
	const limit = 500

	for {
		token, ts := roliascanToken()
		uri := fmt.Sprintf(
			"%s/auth/manga-chapters?manga_id=%s&offset=%d&limit=%d&order=DESC&_t=%s&_ts=%d",
			m.BaseUrl(), m.mangaId, offset, limit, token, ts,
		)
		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: m.URL,
		})
		if err != nil {
			return nil, []error{err}
		}

		feed := roliascanChaptersFeed{}
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			return nil, []error{err}
		}
		if !feed.Success {
			return nil, []error{fmt.Errorf("roliascan: failed to fetch chapters for manga id %s", m.mangaId)}
		}

		for _, c := range feed.Chapters {
			m.languages = append(m.languages, c.Language)
		}
		chapters = append(chapters, roliascanReleases(feed.Chapters, language)...)

		offset += len(feed.Chapters)
		if !feed.HasMore || len(feed.Chapters) == 0 {
			break
		}
	}

	return chapters, nil
}

// roliascanReleases turns the chapters feed into one chapter per release, filtered by
// --language (a release naming no language is kept).
func roliascanReleases(items []roliascanChapterItem, language string) Filterables {
	var chapters Filterables
	for _, c := range items {
		if language != FilterAll && c.Language != "" && c.Language != language {
			continue
		}

		num, err := strconv.ParseFloat(c.Chapter, 64)
		if err != nil {
			continue
		}

		listed := &RoliascanChapter{
			Chapter{
				Number:   num,
				Title:    c.Title,
				Language: c.Language,
				ID:       c.Id,
				Label:    Label{Style: LabelNumbered, Number: c.Chapter},
			},
			c.Id,
		}
		listed.SetGroups(c.GroupName)
		chapters = append(chapters, listed)
	}

	return chapters
}

// FetchChapter fetches a chapter and its pages
func (m *Roliascan) FetchChapter(f Filterable) (*Chapter, error) {
	rchap := f.(*RoliascanChapter)

	uri := fmt.Sprintf("%s/auth/chapter-content?chapter_id=%s", m.BaseUrl(), rchap.Id)
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := roliascanChapterContentFeed{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	if !feed.Success {
		return nil, fmt.Errorf("roliascan: failed to fetch chapter content for chapter id %s", rchap.Id)
	}
	if feed.ChapterType != "" && feed.ChapterType != "media" {
		return nil, fmt.Errorf("roliascan: unsupported chapter type %q for chapter id %s", feed.ChapterType, rchap.Id)
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   rchap.Language,
		ID:         rchap.ID,
		PagesCount: int64(len(feed.Images)),
	}
	chapter.SetGroups(rchap.GetGroups()...)
	for i, img := range feed.Images {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    img,
		})
	}

	return chapter, nil
}

// fetchSeriesPage fetches the series page and caches the title and manga id,
// which is needed to query the chapters endpoint
func (m *Roliascan) fetchSeriesPage() error {
	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}

	m.title = sanitizeTitle(doc.Find("h1").First().Text())
	m.mangaId, _ = doc.Find("body").Attr("data-manga-id")
	if m.mangaId == "" {
		return fmt.Errorf("roliascan: could not find manga id in %s", m.URL)
	}

	return nil
}

// roliascanToken generates the "anti-scraping" token expected by the
// /auth/manga-chapters endpoint, mirroring the site's own generateToken() JS
// function: md5(unixTimestamp + "mng_ch_" + currentUTCHourYYYYMMDDHH),
// truncated to its first 16 hex characters
func roliascanToken() (token string, timestamp int64) {
	now := time.Now()
	timestamp = now.Unix()
	hour := now.UTC().Format("2006010215")
	secret := "mng_ch_" + hour
	sum := md5.Sum([]byte(strconv.FormatInt(timestamp, 10) + secret))
	token = hex.EncodeToString(sum[:])[:16]

	return
}

// roliascanChaptersFeed is the JSON feed for the chapters list
type roliascanChaptersFeed struct {
	Success  bool                   `json:"success"`
	HasMore  bool                   `json:"has_more"`
	Chapters []roliascanChapterItem `json:"chapters"`
}

// roliascanChapterItem is one release in the chapters list. group_name is null
// when the release names no group.
type roliascanChapterItem struct {
	Id        string `json:"id"`
	Chapter   string `json:"chapter"`
	Title     string `json:"title"`
	Language  string `json:"language"`
	GroupName string `json:"group_name"`
}

// roliascanChapterContentFeed is the JSON feed for a chapter's page images
type roliascanChapterContentFeed struct {
	Success     bool     `json:"success"`
	ChapterType string   `json:"chapter_type"`
	Images      []string `json:"images"`
}
