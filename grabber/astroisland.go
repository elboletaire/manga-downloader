// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// astroPlatform is the shared implementation for Astro-based comic sites
// (vortexscans.org, hivetoons.org — and formerly kaynscan.org, before it moved
// to the Next.js RSC platform in rsccomic.go). Their series pages only
// server-render the ~20 most recent chapters as plain <a> rows (the rest hides
// behind a client-side "Load more", so a selector-driven grabber silently
// truncates the list), but the FULL chapter list is embedded as hydration data
// on an <astro-island> element's "props" attribute, devalue-encoded (see
// unwrapDevalue). Reader pages are fully server rendered: page images sit in
// plain <img data-reader-page-image> tags and download over plain HTTP, no
// browser needed anywhere. Recently released chapters can be paywalled
// (coins/early access); those render zero reader images.
type astroPlatform struct {
	*Grabber
	series *astroSeriesProps
}

// AstroChapter represents a chapter of an astroPlatform site
type AstroChapter struct {
	Chapter
	Slug string
}

// astroSeriesProps is the (already devalue-unwrapped) shape of the
// astro-island props embedding the series info and the full chapter list.
// "initialChap" is a sibling of "post" at the top level of the props object,
// not nested inside it.
type astroSeriesProps struct {
	Post struct {
		PostTitle string `json:"postTitle"`
	} `json:"post"`
	InitialChap []struct {
		Number float64 `json:"number"`
		Slug   string  `json:"slug"`
		Title  string  `json:"title"`
	} `json:"initialChap"`
	TotalChapterCount int `json:"totalChapterCount"`
}

// FetchTitle fetches and returns the manga title
func (a *astroPlatform) FetchTitle() (string, error) {
	props, err := a.fetchSeriesProps()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(props.Post.PostTitle), nil
}

// FetchChapters returns the chapters of the manga
func (a *astroPlatform) FetchChapters() (chapters Filterables, errs []error) {
	props, err := a.fetchSeriesProps()
	if err != nil {
		return nil, []error{err}
	}

	// the marker key only proves *an* island was found, not that it's the one
	// holding the chapter list; without this the grabber would report an empty
	// series as a success the moment the props shape changes
	if len(props.InitialChap) == 0 {
		return nil, []error{errors.New("could not find the chapter list in the series page")}
	}

	// the whole point of parsing the island instead of the rendered anchors is
	// getting the full list; if the site ever starts paginating it, fail loudly
	// instead of silently truncating the series
	if len(props.InitialChap) < props.TotalChapterCount {
		return nil, []error{errors.New(
			"the embedded chapter list is incomplete (" +
				strconv.Itoa(len(props.InitialChap)) + " of " +
				strconv.Itoa(props.TotalChapterCount) +
				" chapters): the site may have started paginating it",
		)}
	}

	for _, c := range props.InitialChap {
		chapters = append(chapters, &AstroChapter{
			Chapter{
				Number: c.Number,
				Title:  chapterTitleOrDefault(c.Title, c.Number),
			},
			c.Slug,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (a *astroPlatform) FetchChapter(f Filterable) (*Chapter, error) {
	achap := f.(*AstroChapter)

	slug, err := a.seriesSlug()
	if err != nil {
		return nil, err
	}
	uri, err := url.JoinPath(a.BaseUrl(), "series", slug, achap.Slug)
	if err != nil {
		return nil, err
	}

	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: a.URL,
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
		return nil, errors.New("no pages found for this chapter (it might be locked behind coins/paid early access)")
	}

	return chapter, nil
}

// seriesSlug returns the series slug from the URL (i.e. "some-manga" for
// https://vortexscans.org/series/some-manga)
func (a astroPlatform) seriesSlug() (string, error) {
	re := regexp.MustCompile(`/series/([^/?#]+)`)
	matches := re.FindStringSubmatch(a.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series slug in url %s", a.URL)
	}
	return matches[1], nil
}

// fetchSeriesProps fetches and caches the series info embedded in the series
// page's astro-island hydration JSON (see astroSeriesProps)
func (a *astroPlatform) fetchSeriesProps() (*astroSeriesProps, error) {
	if a.series != nil {
		return a.series, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:     a.URL,
		Referer: a.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	props, err := findAstroSeriesProps(body)
	if err != nil {
		return nil, err
	}

	a.series = props

	return props, nil
}

// findAstroSeriesProps scans astro-island blobs in order for one with both a
// title and a chapter list, since a marker match alone could be a decoy (e.g.
// a "trending" widget) instead of the real series island.
func findAstroSeriesProps(body string) (*astroSeriesProps, error) {
	from := 0
	for {
		raw, next, err := extractAstroIslandProps(body, "totalChapterCount", from)
		if err != nil {
			return nil, err
		}
		from = next

		var generic interface{}
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			continue
		}

		unwrapped, err := json.Marshal(unwrapDevalue(generic))
		if err != nil {
			continue
		}

		props := &astroSeriesProps{}
		if err := json.Unmarshal(unwrapped, props); err != nil {
			continue
		}

		if props.Post.PostTitle == "" || len(props.InitialChap) == 0 {
			continue
		}

		return props, nil
	}
}

// extractAstroIslandProps finds the <astro-island props="..."> blob whose
// JSON contains marker, searching from byte offset from, and returns its
// unescaped JSON string plus the offset to resume searching from.
func extractAstroIslandProps(body, marker string, from int) (raw string, next int, err error) {
	idx := strings.Index(body[from:], marker)
	if idx == -1 {
		return "", 0, errors.New("astro-island: could not find " + marker + " in the series page")
	}
	markerIdx := from + idx

	tagStart := strings.LastIndex(body[:markerIdx], "<astro-island")
	if tagStart == -1 {
		return "", 0, errors.New("astro-island: could not find the astro-island tag holding " + marker)
	}

	attrIdx := strings.Index(body[tagStart:], `props="`)
	if attrIdx == -1 {
		return "", 0, errors.New("astro-island: tag has no props attribute")
	}
	valStart := tagStart + attrIdx + len(`props="`)

	valEnd := strings.IndexByte(body[valStart:], '"')
	if valEnd == -1 {
		return "", 0, errors.New("astro-island: unterminated props attribute")
	}

	return html.UnescapeString(body[valStart : valStart+valEnd]), valStart + valEnd, nil
}

// unwrapDevalue recursively decodes the devalue-style encoding Astro islands
// use for their hydration props: every value is serialized as a 2-element
// array [tag, value] — [0, v] for a plain value or object (whose own fields
// are wrapped the same way) and [1, [...]] for an array of wrapped elements —
// while a lone [tag] with no value encodes "undefined". Maps have each field
// unwrapped; anything else passes through unchanged.
func unwrapDevalue(node interface{}) interface{} {
	switch v := node.(type) {
	case []interface{}:
		if len(v) > 0 {
			if tag, ok := v[0].(float64); ok {
				// a lone [tag] with no value encodes "undefined"
				if len(v) == 1 {
					return nil
				}
				if len(v) == 2 {
					if int(tag) == 1 {
						if items, ok := v[1].([]interface{}); ok {
							out := make([]interface{}, len(items))
							for i, e := range items {
								out[i] = unwrapDevalue(e)
							}
							return out
						}
					}
					return unwrapDevalue(v[1])
				}
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
