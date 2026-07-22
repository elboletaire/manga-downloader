// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Mangataro is a grabber for mangataro.org: the series/reader pages are
// static, but the chapter list and the chapter images are both loaded from a
// JSON API behind a short-lived, time-based token computed client-side (see
// mangataroToken)
type Mangataro struct {
	*Grabber
	title   string
	doc     *goquery.Document
	mangaID string
}

func NewMangataro(g *Grabber) *Mangataro {
	return &Mangataro{Grabber: g}
}

// MangataroChapter represents a Mangataro Chapter
type MangataroChapter struct {
	Chapter
	Id string
}

// Test returns true if the URL is a mangataro.org URL
func (m *Mangataro) Test() (bool, error) {
	re := regexp.MustCompile(`mangataro\.org`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangataro) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	doc, err := m.document()
	if err != nil {
		return "", err
	}

	m.title = sanitizeTitle(doc.Find("h1").First().Text())

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m *Mangataro) FetchChapters() (chapters Filterables, errs []error) {
	id, err := m.id()
	if err != nil {
		return nil, []error{err}
	}

	language := m.Settings.Language
	if language == "" {
		language = "en"
	}

	offset := 0
	limit := 500
	for {
		token, ts := mangataroToken()
		params := url.Values{}
		params.Set("manga_id", id)
		params.Set("offset", strconv.Itoa(offset))
		params.Set("limit", strconv.Itoa(limit))
		params.Set("order", "ASC")
		params.Set("_t", token)
		params.Set("_ts", strconv.FormatInt(ts, 10))

		uri, _ := url.JoinPath(m.BaseUrl(), "auth", "manga-chapters")
		uri = uri + "?" + params.Encode()

		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: m.URL,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		feed := mangataroChaptersFeed{}
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			errs = append(errs, err)
			return
		}
		if !feed.Success {
			errs = append(errs, fmt.Errorf("mangataro chapters api returned an unsuccessful response for %s", m.URL))
			return
		}

		for _, c := range feed.Chapters {
			if c.Language != language {
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
			chapters = append(chapters, &MangataroChapter{
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

	return
}

// FetchChapter fetches a chapter and its pages
func (m *Mangataro) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangataroChapter)

	uri, _ := url.JoinPath(m.BaseUrl(), "auth", "chapter-content")
	uri = fmt.Sprintf("%s?chapter_id=%s", uri, mchap.Id)

	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	feed := struct {
		Success     bool     `json:"success"`
		ChapterType string   `json:"chapter_type"`
		Images      []string `json:"images"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}
	if !feed.Success {
		return nil, fmt.Errorf("mangataro chapter-content api returned an unsuccessful response for chapter %s", mchap.Id)
	}
	if feed.ChapterType != "media" {
		return nil, fmt.Errorf("chapter %s is a %q chapter, only image (%q) chapters are supported", mchap.Id, feed.ChapterType, "media")
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   mchap.Language,
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

// document fetches and caches the series page document
func (m *Mangataro) document() (*goquery.Document, error) {
	if m.doc != nil {
		return m.doc, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	m.doc = doc

	return m.doc, nil
}

// id returns the manga's numeric id, read from the `data-manga-id` attribute
// set on the series page's `<body>` tag
func (m *Mangataro) id() (string, error) {
	if m.mangaID != "" {
		return m.mangaID, nil
	}

	doc, err := m.document()
	if err != nil {
		return "", err
	}

	id, ok := doc.Find("body").Attr("data-manga-id")
	if !ok || id == "" {
		return "", fmt.Errorf("could not find manga id in %s", m.URL)
	}

	m.mangaID = id

	return m.mangaID, nil
}

// mangataroToken computes the time-based token the site's own javascript
// generates client-side to authorize /auth/manga-chapters requests: an
// md5 hash of the unix timestamp concatenated with a secret derived from the
// current UTC hour, truncated to 16 hex chars. Reversed from the site's
// bundled JS (`generateToken()`):
//
//	const timestamp = Math.floor(Date.now()/1000);
//	const hour = new Date().toISOString().slice(0,13).replace(/[-T:]/g,'');
//	const secret = 'mng_ch_'+hour;
//	const hash = md5(timestamp+secret).substring(0,16);
func mangataroToken() (token string, timestamp int64) {
	timestamp = time.Now().Unix()
	hour := time.Now().UTC().Format("2006010215")
	secret := "mng_ch_" + hour
	sum := md5.Sum([]byte(strconv.FormatInt(timestamp, 10) + secret))
	token = fmt.Sprintf("%x", sum)[:16]

	return
}

// mangataroChaptersFeed is the JSON feed for the chapters list
type mangataroChaptersFeed struct {
	Success  bool `json:"success"`
	HasMore  bool `json:"has_more"`
	Chapters []struct {
		Id       string `json:"id"`
		Chapter  string `json:"chapter"`
		Title    string `json:"title"`
		Language string `json:"language"`
	} `json:"chapters"`
}
