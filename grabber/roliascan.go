// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
}

func NewRoliascan(g *Grabber) *Roliascan {
	return &Roliascan{Grabber: g}
}

// RoliascanChapter represents a Roliascan Chapter
type RoliascanChapter struct {
	Chapter
	Id string
}

// Test returns true if the URL is a roliascan.com URL
func (m *Roliascan) Test() (bool, error) {
	re := regexp.MustCompile(`roliascan\.com`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Roliascan) FetchTitle() (string, error) {
	if m.title == "" {
		if err := m.fetchSeriesPage(); err != nil {
			return "", err
		}
	}

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Roliascan) FetchChapters() (Filterables, []error) {
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
			if c.Language != "" && c.Language != language {
				continue
			}

			num, err := strconv.ParseFloat(c.Chapter, 64)
			if err != nil {
				continue
			}

			title := c.Title
			if title == "" {
				title = "Chapter " + c.Chapter
			}

			chapters = append(chapters, &RoliascanChapter{
				Chapter{
					Number:   num,
					Title:    title,
					Language: c.Language,
				},
				c.Id,
			})
		}

		offset += len(feed.Chapters)
		if !feed.HasMore || len(feed.Chapters) == 0 {
			break
		}
	}

	return chapters, nil
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
		Number:     f.GetNumber(),
		Language:   rchap.Language,
		PagesCount: int64(len(feed.Images)),
	}
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
	Success  bool `json:"success"`
	HasMore  bool `json:"has_more"`
	Chapters []struct {
		Id       string `json:"id"`
		Chapter  string `json:"chapter"`
		Title    string `json:"title"`
		Language string `json:"language"`
	} `json:"chapters"`
}

// roliascanChapterContentFeed is the JSON feed for a chapter's page images
type roliascanChapterContentFeed struct {
	Success     bool     `json:"success"`
	ChapterType string   `json:"chapter_type"`
	Images      []string `json:"images"`
}
