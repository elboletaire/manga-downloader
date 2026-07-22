// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Vortexscans is a grabber for vortexscans.org. The series page only renders
// the ~20 most recent chapters as plain <a> links, but the full chapter list
// is embedded as hydration data on an <astro-island> element's "props"
// attribute, encoded with a custom devalue-like scheme where every value is
// wrapped as a 2-element array: [0, value] for a plain value/object (whose
// own fields are wrapped the same way) or [1, [...]] for an array of wrapped
// elements ("undefined" values are encoded as the 1-element array [0]).
// vsUnwrap below undoes that wrapping. Reader pages are plain server
// rendered HTML: page images are <img data-reader-page-image> tags, no
// browser/JS needed. Recently released chapters can be paywalled (coins);
// those render zero <img data-reader-page-image> tags.
type Vortexscans struct {
	*Grabber
	manga *vortexscansManga
}

func NewVortexscans(g *Grabber) *Vortexscans {
	return &Vortexscans{Grabber: g}
}

// VortexscansChapter represents a Vortexscans Chapter
type VortexscansChapter struct {
	Chapter
	Slug string
}

// vortexscansManga is the series info parsed from the embedded hydration data
type vortexscansManga struct {
	Title    string
	Chapters []vortexscansChapterData
}

// vortexscansChapterData is a single chapter entry parsed from the embedded
// hydration data
type vortexscansChapterData struct {
	Number float64
	Slug   string
	Title  string
}

// Test returns true if the URL is a vortexscans.org URL
func (v *Vortexscans) Test() (bool, error) {
	re := regexp.MustCompile(`vortexscans\.org`)
	return re.MatchString(v.URL), nil
}

// FetchTitle fetches and returns the manga title
func (v *Vortexscans) FetchTitle() (string, error) {
	manga, err := v.fetchManga()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(manga.Title), nil
}

// FetchChapters returns the chapters of the manga
func (v *Vortexscans) FetchChapters() (chapters Filterables, errs []error) {
	manga, err := v.fetchManga()
	if err != nil {
		return nil, []error{err}
	}

	for _, c := range manga.Chapters {
		title := c.Title
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(c.Number, 'f', -1, 64)
		}
		chapters = append(chapters, &VortexscansChapter{
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
func (v Vortexscans) FetchChapter(f Filterable) (*Chapter, error) {
	vchap := f.(*VortexscansChapter)

	slug := v.seriesSlug()
	if slug == "" {
		return nil, errors.New("could not find series slug in url " + v.URL)
	}
	uri := v.BaseUrl() + "/series/" + slug + "/" + vchap.Slug

	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: v.URL,
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
func (v Vortexscans) seriesSlug() string {
	re := regexp.MustCompile(`/series/([^/]+)`)
	matches := re.FindStringSubmatch(v.URL)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

// fetchManga fetches and caches the series title and full chapter list, both
// parsed out of the hydration data embedded in an <astro-island> "props"
// attribute on the series page
func (v *Vortexscans) fetchManga() (*vortexscansManga, error) {
	if v.manga != nil {
		return v.manga, nil
	}

	body, err := http.Get(http.RequestParams{
		URL:     v.URL,
		Referer: v.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	var manga *vortexscansManga
	doc.Find("astro-island").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		props, ok := s.Attr("props")
		if !ok || !strings.Contains(props, `"initialChap"`) {
			return true
		}

		var root map[string]interface{}
		if err := json.Unmarshal([]byte(props), &root); err != nil {
			return true
		}

		post, ok := vsUnwrap(root["post"]).(map[string]interface{})
		if !ok {
			return true
		}

		title, _ := post["postTitle"].(string)
		// "initialChap" (the full chapter list) is a sibling of "post" at
		// the top level of the props object, not nested inside it
		rawChapters, _ := vsUnwrap(root["initialChap"]).([]interface{})
		if title == "" || len(rawChapters) == 0 {
			return true
		}

		m := &vortexscansManga{Title: title}
		for _, rc := range rawChapters {
			cm, ok := rc.(map[string]interface{})
			if !ok {
				continue
			}
			number, _ := cm["number"].(float64)
			slug, _ := cm["slug"].(string)
			t, _ := cm["title"].(string)
			if slug == "" {
				continue
			}
			m.Chapters = append(m.Chapters, vortexscansChapterData{
				Number: number,
				Slug:   slug,
				Title:  t,
			})
		}
		manga = m

		return false
	})

	if manga == nil {
		return nil, errors.New("could not find the chapter list in the series page")
	}

	v.manga = manga

	return manga, nil
}

// vsUnwrap recursively decodes vortexscans' devalue-like hydration
// encoding: every value is serialized as a 2-element array, [0, value] for a
// plain value (or an object whose own fields are wrapped the same way), or
// [1, [...]] for an array of wrapped elements. Values that don't match this
// shape (already-decoded JSON primitives, or unsupported wrapper types) are
// returned unchanged.
func vsUnwrap(v interface{}) interface{} {
	arr, ok := v.([]interface{})
	if !ok {
		return v
	}
	// a 1-element array (just the type marker, no value) encodes "undefined"
	if len(arr) < 2 {
		return nil
	}

	typ, ok := arr[0].(float64)
	if !ok {
		return v
	}

	if int(typ) == 1 {
		items, ok := arr[1].([]interface{})
		if !ok {
			return arr[1]
		}
		out := make([]interface{}, len(items))
		for i, it := range items {
			out[i] = vsUnwrap(it)
		}
		return out
	}

	val := arr[1]
	if m, ok := val.(map[string]interface{}); ok {
		out := make(map[string]interface{}, len(m))
		for k, v2 := range m {
			out[k] = vsUnwrap(v2)
		}
		return out
	}
	return val
}
