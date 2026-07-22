// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Hijala is a grabber for en-hijala.com: no Cloudflare, everything is plain
// HTTP. Reader page images are embedded server-side in the page's escaped
// Next.js RSC payload for every page, but only the first ~6 <img> tags get a
// real "src" server-side (the rest hydrate client-side), so pages are
// extracted with a regex over the raw HTML rather than an <img> selector.
// The series page only server-renders chapter 1 plus the ~20 newest
// chapters, so the full chapters list is fetched from the site's paginated
// JSON API (api.en-hijala.com/api/chapters), keyed by a numeric "postId"
// that's only exposed inside the series page's escaped RSC payload (there's
// no plain slug->id API endpoint).
type Hijala struct {
	*Grabber
	title  string
	postID string
}

func NewHijala(g *Grabber) *Hijala {
	return &Hijala{Grabber: g}
}

// HijalaChapter represents a Hijala chapter
type HijalaChapter struct {
	Chapter
	Slug string
}

// hijalaPostIDRe extracts the numeric postId from the series page's
// Next.js RSC payload, where it appears JSON-escaped as `postId\":39` (the
// backslash is literal, escaping the quote for the outer JSON.stringify).
var hijalaPostIDRe = regexp.MustCompile(`postId\\?":(\d+)`)

// hijalaImageRe extracts page image URLs from the reader page. Only the
// first ~6 pages are eagerly rendered with a populated <img src>; the rest
// are lazy-loaded and only get their src client-side via React hydration,
// but every page's URL (eager or lazy) is already present in the page's
// escaped Next.js RSC payload, so scanning the raw HTML text for this
// pattern (rather than relying on <img> attributes) picks up all of them.
var hijalaImageRe = regexp.MustCompile(`https://storage\.en-hijala\.com/upload/series/[^"\\ ]*\.(?:jpe?g|png|webp|gif)`)

// hijalaPageNumberRe extracts the ordinal ("page-0007") embedded in each
// image filename, used to sort pages since the RSC payload doesn't
// guarantee reading order the way <img> document order would.
var hijalaPageNumberRe = regexp.MustCompile(`/page-(\d+)_`)

// Test returns true if the URL is an en-hijala.com URL
func (h *Hijala) Test() (bool, error) {
	re := regexp.MustCompile(`en-hijala\.com`)
	return re.MatchString(h.URL), nil
}

// FetchTitle fetches and returns the manga title
func (h *Hijala) FetchTitle() (string, error) {
	if h.title != "" {
		return h.title, nil
	}

	body, err := http.GetText(http.RequestParams{URL: h.URL})
	if err != nil {
		return "", err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", err
	}

	title := strings.TrimSpace(doc.Find("title").First().Text())
	title = strings.TrimSuffix(title, " - Hijala Translations")
	title = strings.TrimSuffix(title, " Manga")

	h.title = sanitizeTitle(title)

	return h.title, nil
}

// seriesPostID fetches the series page and extracts the numeric postId
// used by the site's JSON API.
func (h *Hijala) seriesPostID() (string, error) {
	if h.postID != "" {
		return h.postID, nil
	}

	body, err := http.GetText(http.RequestParams{URL: h.URL})
	if err != nil {
		return "", err
	}

	matches := hijalaPostIDRe.FindStringSubmatch(body)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series id in %s", h.URL)
	}

	h.postID = matches[1]

	return h.postID, nil
}

// FetchChapters returns the chapters of the manga
func (h *Hijala) FetchChapters() (chapters Filterables, errs []error) {
	postID, err := h.seriesPostID()
	if err != nil {
		return nil, []error{err}
	}

	skip := 0
	for {
		uri := fmt.Sprintf("https://api.en-hijala.com/api/chapters?postId=%s&skip=%d", postID, skip)
		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: h.URL,
		})
		if err != nil {
			errs = append(errs, err)
			return
		}

		feed := hijalaChaptersFeed{}
		if err = json.Unmarshal([]byte(body), &feed); err != nil {
			errs = append(errs, err)
			return
		}
		if len(feed.Post.Chapters) == 0 {
			return
		}

		for _, c := range feed.Post.Chapters {
			title := c.Title
			if title == "" {
				title = "Chapter " + strconv.FormatFloat(float64(c.Number), 'f', -1, 64)
			}
			chapters = append(chapters, &HijalaChapter{
				Chapter{
					Number: float64(c.Number),
					Title:  title,
				},
				c.Slug,
			})
		}

		skip += len(feed.Post.Chapters)
		if skip >= feed.TotalChapterCount {
			return
		}
	}
}

// FetchChapter fetches a chapter and its pages
func (h *Hijala) FetchChapter(f Filterable) (*Chapter, error) {
	hchap := f.(*HijalaChapter)

	seriesSlug, err := h.seriesSlug()
	if err != nil {
		return nil, err
	}

	uri, _ := url.JoinPath(h.BaseUrl(), "series", seriesSlug, hchap.Slug)
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: h.URL,
	})
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
	}

	seen := map[string]bool{}
	type imgURL struct {
		num int
		url string
	}
	var imgs []imgURL
	for _, src := range hijalaImageRe.FindAllString(body, -1) {
		if seen[src] {
			continue
		}
		seen[src] = true

		num := len(imgs) + 1
		if m := hijalaPageNumberRe.FindStringSubmatch(src); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				num = n
			}
		}
		imgs = append(imgs, imgURL{num: num, url: src})
	}
	sort.Slice(imgs, func(i, j int) bool { return imgs[i].num < imgs[j].num })

	for i, img := range imgs {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    img.url,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// seriesSlug returns the series slug from the URL (i.e. "double-click" for
// https://en-hijala.com/series/double-click)
func (h *Hijala) seriesSlug() (string, error) {
	re := regexp.MustCompile(`/series/([^/]+)`)
	matches := re.FindStringSubmatch(h.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series slug in url %s", h.URL)
	}
	return matches[1], nil
}

// hijalaChaptersFeed is the JSON feed for the paginated chapters list
type hijalaChaptersFeed struct {
	Post struct {
		Chapters []struct {
			Slug   string `json:"slug"`
			Number int    `json:"number"`
			Title  string `json:"title"`
		} `json:"chapters"`
	} `json:"post"`
	TotalChapterCount int `json:"totalChapterCount"`
}
