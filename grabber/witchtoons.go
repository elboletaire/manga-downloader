// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Witchtoons is a grabber for witchtoons.net (the witchscans.com rebrand,
// which 301s to this domain but drops the path: the old themesia wordpress
// theme is gone and series now live under /series/comic/{slug}). It's a
// Next.js App Router site, but fully server rendered, so plain HTTP gets both
// the chapter list and the reader pages — they just ship inside the RSC
// flight payload instead of the DOM (the series page only renders a single
// "start reading" chapter link). The chapter list is paginated at 100
// chapters per page through a plain ?page=N query param.
type Witchtoons struct {
	*Grabber
	pages map[int]*witchtoonsSeriesPage
}

func NewWitchtoons(g *Grabber) *Witchtoons {
	return &Witchtoons{Grabber: g}
}

// WitchtoonsChapter represents a Witchtoons Chapter
type WitchtoonsChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a witchtoons.net URL
func (w *Witchtoons) Test() (bool, error) {
	re := regexp.MustCompile(`witchtoons\.net`)
	return re.MatchString(w.URL), nil
}

// FetchTitle fetches and returns the manga title
func (w *Witchtoons) FetchTitle() (string, error) {
	page, err := w.fetchSeriesPage(1)
	if err != nil {
		return "", err
	}

	return sanitizeTitle(page.Series.Title), nil
}

// FetchChapters returns the chapters of the manga
func (w *Witchtoons) FetchChapters() (chapters Filterables, errs []error) {
	first, err := w.fetchSeriesPage(1)
	if err != nil {
		return nil, []error{err}
	}

	pages := []*witchtoonsSeriesPage{first}
	for i := 2; i <= first.TotalPages; i++ {
		page, err := w.fetchSeriesPage(i)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		pages = append(pages, page)
	}

	for _, page := range pages {
		for _, c := range page.Chapters {
			number := formatChapterNumber(c.Number)
			title := strings.TrimSpace(c.Title)
			if title == "" {
				title = "Chapter " + number
			}
			// reader URLs are keyed by chapter number, not by the chapter id
			uri, err := url.JoinPath(w.BaseUrl(), "series", "comic", first.Series.Slug, "chapter", number)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			chapters = append(chapters, &WitchtoonsChapter{
				Chapter{
					Number: c.Number,
					Title:  title,
				},
				uri,
			})
		}
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (w Witchtoons) FetchChapter(f Filterable) (*Chapter, error) {
	wchap := f.(*WitchtoonsChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     wchap.URL,
		Referer: w.URL,
	})
	if err != nil {
		return nil, err
	}

	pages, err := witchtoonsChapterPages(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
	}

	for _, p := range pages {
		src := strings.TrimSpace(p.ImageURL)
		if src == "" {
			continue
		}
		// page URLs are relative and signed (?sig=...&exp=...), valid for
		// about an hour, which is plenty since they're downloaded right away
		if !strings.HasPrefix(src, "http") {
			src = w.BaseUrl() + src
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    src,
		})
	}

	if len(chapter.Pages) == 0 {
		// the site coin-locks its most recent chapters; those still list but
		// their reader ships no images at all, so fail loudly instead of
		// packing an empty archive
		return nil, fmt.Errorf("no pages found for chapter %s (it may be premium/coin locked)", formatChapterNumber(f.GetNumber()))
	}

	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// witchtoonsChapterPages extracts the reader's `"pages":[...]` array out of a
// chapter page's RSC flight payload
func witchtoonsChapterPages(html string) ([]witchtoonsPage, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	raw, err := extractBalancedJSON(stream, `"pages":`)
	if err != nil {
		return nil, err
	}

	var pages []witchtoonsPage
	if err := json.Unmarshal([]byte(raw), &pages); err != nil {
		return nil, err
	}

	return pages, nil
}

// fetchSeriesPage fetches (and caches) the given page of the series' chapter
// list, extracting the series info and chapters out of its RSC payload
func (w *Witchtoons) fetchSeriesPage(page int) (*witchtoonsSeriesPage, error) {
	if w.pages == nil {
		w.pages = map[int]*witchtoonsSeriesPage{}
	}
	if cached, ok := w.pages[page]; ok {
		return cached, nil
	}

	uri := w.URL
	if page > 1 {
		parsed, err := url.Parse(uri)
		if err != nil {
			return nil, err
		}
		query := parsed.Query()
		query.Set("page", strconv.Itoa(page))
		parsed.RawQuery = query.Encode()
		uri = parsed.String()
	}

	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: w.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	parsed, err := parseWitchtoonsSeriesPage(body)
	if err != nil {
		return nil, err
	}

	w.pages[page] = parsed

	return parsed, nil
}

// parseWitchtoonsSeriesPage extracts the series info, its chapters and the
// chapter list pagination out of a series page's RSC flight payload. They're
// sibling keys of the same object, but that object is a react element tuple
// (`["$","$L6c",null,{...}]`), so each key is extracted on its own instead of
// unmarshalling a single wrapper.
func parseWitchtoonsSeriesPage(html string) (*witchtoonsSeriesPage, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	page := &witchtoonsSeriesPage{TotalPages: 1}

	raw, err := extractBalancedJSON(stream, `"series":`)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &page.Series); err != nil {
		return nil, err
	}

	raw, err = extractBalancedJSON(stream, `"chapters":`)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &page.Chapters); err != nil {
		return nil, err
	}

	if m := witchtoonsTotalPagesRe.FindStringSubmatch(stream); len(m) > 1 {
		if total, err := strconv.Atoi(m[1]); err == nil && total > 0 {
			page.TotalPages = total
		}
	}

	return page, nil
}

var witchtoonsTotalPagesRe = regexp.MustCompile(`"totalPages":(\d+)`)

// witchtoonsSeriesPage is one page of a series' chapter list
type witchtoonsSeriesPage struct {
	Series struct {
		Title string `json:"title"`
		Slug  string `json:"slug"`
	}
	Chapters []struct {
		Number float64 `json:"number"`
		// null for most chapters, in which case the site renders "Chapter N"
		Title string `json:"title"`
	}
	TotalPages int
}

// witchtoonsPage is a single page of a chapter, as embedded in the reader
type witchtoonsPage struct {
	ImageURL string `json:"imageUrl"`
}
