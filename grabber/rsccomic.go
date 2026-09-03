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

// rscComic is the shared implementation for the Next.js App Router comic
// platform behind witchtoons.net and kaynscans.com. It's fully server
// rendered, so plain HTTP gets both the chapter list and the reader pages —
// they just ship inside the RSC flight payload instead of the DOM (the series
// page only renders a single "start reading" chapter link, so a
// selector-driven grabber would silently see 1 chapter and look like it
// works). Series live under /series/comic/{slug}, the chapter list is
// paginated at 100 chapters per page through a plain ?page=N query param, and
// reader pages embed their images as a "pages":[{imageUrl}] array.
type rscComic struct {
	*Grabber
	pages map[int]*rscComicSeriesPage
}

// RSCComicChapter represents an rscComic chapter
type RSCComicChapter struct {
	Chapter
	URL string
}

// FetchTitle fetches and returns the manga title
func (r *rscComic) FetchTitle() (string, error) {
	page, err := r.fetchSeriesPage(1)
	if err != nil {
		return "", err
	}

	return sanitizeTitle(page.Series.Title), nil
}

// FetchChapters returns the chapters of the manga
func (r *rscComic) FetchChapters() (chapters Filterables, errs []error) {
	first, err := r.fetchSeriesPage(1)
	if err != nil {
		return nil, []error{err}
	}

	pages := []*rscComicSeriesPage{first}
	for i := 2; i <= first.TotalPages; i++ {
		page, err := r.fetchSeriesPage(i)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		pages = append(pages, page)
	}

	// cross-check against the site's own chapterCount, in case totalPages
	// undercounts or a page silently fails
	total := 0
	for _, page := range pages {
		total += len(page.Chapters)
	}
	if first.Series.ChapterCount > 0 && total < first.Series.ChapterCount {
		errs = append(errs, fmt.Errorf(
			"the chapter list is incomplete (%d of %d chapters): a page may have failed to load or the site started paginating differently",
			total, first.Series.ChapterCount,
		))
		return nil, errs
	}

	for _, page := range pages {
		for _, c := range page.Chapters {
			// reader URLs are keyed by chapter number, not by the chapter id
			uri, err := url.JoinPath(r.BaseUrl(), "series", "comic", first.Series.Slug, "chapter", formatChapterNumber(c.Number))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			chapters = append(chapters, &RSCComicChapter{
				Chapter{
					Number: c.Number,
					Title:  chapterTitleOrDefault(c.Title, c.Number),
				},
				uri,
			})
		}
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (r *rscComic) FetchChapter(f Filterable) (*Chapter, error) {
	rchap := f.(*RSCComicChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     rchap.URL,
		Referer: r.URL,
	})
	if err != nil {
		return nil, err
	}

	pages, err := rscComicChapterPages(body)
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
			src = r.BaseUrl() + src
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    src,
		})
	}

	if len(chapter.Pages) == 0 {
		// the platform coin-locks its most recent chapters; those still list
		// but their reader ships no images at all, so fail loudly instead of
		// packing an empty archive
		return nil, fmt.Errorf("no pages found for chapter %s (it may be premium/coin locked)", formatChapterNumber(f.GetNumber()))
	}

	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// rscComicChapterPages extracts the reader's `"pages":[...]` array out of a
// chapter page's RSC flight payload
func rscComicChapterPages(html string) ([]rscComicPage, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	raw, err := extractBalancedJSON(stream, `"pages":`)
	if err != nil {
		return nil, err
	}

	var pages []rscComicPage
	if err := json.Unmarshal([]byte(raw), &pages); err != nil {
		return nil, err
	}

	return pages, nil
}

// fetchSeriesPage fetches (and caches) the given page of the series' chapter
// list, extracting the series info and chapters out of its RSC payload
func (r *rscComic) fetchSeriesPage(page int) (*rscComicSeriesPage, error) {
	if r.pages == nil {
		r.pages = map[int]*rscComicSeriesPage{}
	}
	if cached, ok := r.pages[page]; ok {
		return cached, nil
	}

	uri := r.URL
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
		Referer: r.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	parsed, err := parseRSCComicSeriesPage(body)
	if err != nil {
		return nil, err
	}

	r.pages[page] = parsed

	return parsed, nil
}

// parseRSCComicSeriesPage extracts the series info, its chapters and the
// chapter list pagination out of a series page's RSC flight payload. They're
// sibling keys of the same object, but that object is a react element tuple
// (`["$","$L6c",null,{...}]`), so each key is extracted on its own instead of
// unmarshalling a single wrapper.
func parseRSCComicSeriesPage(html string) (*rscComicSeriesPage, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	page := &rscComicSeriesPage{TotalPages: 1}

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

	if m := rscComicTotalPagesRe.FindStringSubmatch(stream); len(m) > 1 {
		if total, err := strconv.Atoi(m[1]); err == nil && total > 0 {
			page.TotalPages = total
		}
	}

	return page, nil
}

var rscComicTotalPagesRe = regexp.MustCompile(`"totalPages":(\d+)`)

// rscComicSeriesPage is one page of a series' chapter list
type rscComicSeriesPage struct {
	Series struct {
		Title        string `json:"title"`
		Slug         string `json:"slug"`
		ChapterCount int    `json:"chapterCount"`
	}
	Chapters []struct {
		Number float64 `json:"number"`
		// null for most chapters, in which case the site renders "Chapter N"
		Title string `json:"title"`
	}
	TotalPages int
}

// rscComicPage is a single page of a chapter, as embedded in the reader
type rscComicPage struct {
	ImageURL string `json:"imageUrl"`
}
