// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Kaynscan is a grabber for kaynscan.org, an Astro-based site whose series
// page only server-renders the ~20 most recent chapters as plain <a> rows;
// the full chapter list (including every chapter's paywall/early-access
// status) is embedded as a devalue-tagged JSON blob inside one of the page's
// <astro-island props="..."> hydration attributes. Reader pages, by
// contrast, are fully server-rendered: page images sit in plain
// <img data-reader-page-image> tags and download over plain HTTP with no
// cookies/session needed, so no browser is required anywhere for this site.
type Kaynscan struct {
	*Grabber
	series *kaynscanSeriesProps
}

func NewKaynscan(g *Grabber) *Kaynscan {
	return &Kaynscan{Grabber: g}
}

// KaynscanChapter represents a Kaynscan chapter
type KaynscanChapter struct {
	Chapter
	Slug string
}

// Test returns true if the URL is a kaynscan.org URL
func (k *Kaynscan) Test() (bool, error) {
	re := regexp.MustCompile(`kaynscan\.org`)
	return re.MatchString(k.URL), nil
}

// FetchTitle fetches and returns the manga title
func (k *Kaynscan) FetchTitle() (string, error) {
	body, err := http.Get(http.RequestParams{
		URL:     k.URL,
		Referer: k.BaseUrl(),
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	return sanitizeTitle(doc.Find(`h1[itemprop="name"]`).First().Text()), nil
}

// FetchChapters returns the chapters of the manga
func (k *Kaynscan) FetchChapters() (chapters Filterables, errs []error) {
	props, err := k.fetchSeriesProps()
	if err != nil {
		return nil, []error{err}
	}

	for _, c := range props.InitialChap {
		title := strings.TrimSpace(c.Title)
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(c.Number, 'f', -1, 64)
		}
		chapters = append(chapters, &KaynscanChapter{
			Chapter{
				Number: c.Number,
				Title:  title,
			},
			c.Slug,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (k Kaynscan) FetchChapter(f Filterable) (*Chapter, error) {
	kchap := f.(*KaynscanChapter)

	uri, err := url.JoinPath(k.URL, kchap.Slug)
	if err != nil {
		return nil, err
	}

	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: k.BaseUrl(),
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

	doc.Find("img[data-reader-page-image]").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	if chapter.PagesCount == 0 {
		return nil, errors.New("no pages found (the chapter may be locked behind an early-access paywall)")
	}

	return chapter, nil
}

// fetchSeriesProps fetches and caches the series info embedded in the series
// page's astro-island hydration JSON (see kaynscanSeriesProps); the visible
// HTML chapter rows only cover the most recent chapters, this blob has all
// of them
func (k *Kaynscan) fetchSeriesProps() (*kaynscanSeriesProps, error) {
	if k.series != nil {
		return k.series, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:     k.URL,
		Referer: k.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	raw, err := extractKaynscanIslandProps(body, "totalChapterCount")
	if err != nil {
		return nil, err
	}

	var generic interface{}
	if err = json.Unmarshal([]byte(raw), &generic); err != nil {
		return nil, err
	}

	unwrapped, err := json.Marshal(unwrapDevalue(generic))
	if err != nil {
		return nil, err
	}

	props := &kaynscanSeriesProps{}
	if err = json.Unmarshal(unwrapped, props); err != nil {
		return nil, err
	}

	k.series = props

	return props, nil
}

// extractKaynscanIslandProps finds the <astro-island props="..."> hydration
// blob whose JSON contains the given marker key and returns its unescaped,
// still devalue-tagged JSON string
func extractKaynscanIslandProps(body, marker string) (string, error) {
	markerIdx := strings.Index(body, marker)
	if markerIdx == -1 {
		return "", errors.New("kaynscan: could not find " + marker + " in the series page")
	}

	tagStart := strings.LastIndex(body[:markerIdx], "<astro-island")
	if tagStart == -1 {
		return "", errors.New("kaynscan: could not find the astro-island tag holding " + marker)
	}

	attrIdx := strings.Index(body[tagStart:], `props="`)
	if attrIdx == -1 {
		return "", errors.New("kaynscan: astro-island tag has no props attribute")
	}
	valStart := tagStart + attrIdx + len(`props="`)

	valEnd := strings.IndexByte(body[valStart:], '"')
	if valEnd == -1 {
		return "", errors.New("kaynscan: unterminated props attribute")
	}

	return html.UnescapeString(body[valStart : valStart+valEnd]), nil
}

// unwrapDevalue recursively strips Astro's devalue-style [tag, value] tuple
// wrapping (every prop value is serialized as a 2-element array of a numeric
// type tag and the actual value) so the payload can be re-marshaled into
// plain Go structs
func unwrapDevalue(node interface{}) interface{} {
	switch v := node.(type) {
	case []interface{}:
		if len(v) == 2 {
			if _, ok := v[0].(float64); ok {
				return unwrapDevalue(v[1])
			}
		}
		out := make([]interface{}, len(v))
		for i, e := range v {
			out[i] = unwrapDevalue(e)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, e := range v {
			out[key] = unwrapDevalue(e)
		}
		return out
	default:
		return node
	}
}

// kaynscanSeriesProps is the (already devalue-unwrapped) shape of the
// astro-island props embedding the full chapter list
type kaynscanSeriesProps struct {
	InitialChap []struct {
		Number float64 `json:"number"`
		Slug   string  `json:"slug"`
		Title  string  `json:"title"`
	} `json:"initialChap"`
	TotalChapterCount int `json:"totalChapterCount"`
}
