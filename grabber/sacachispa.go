// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Sacachispa is a grabber for sacachispa.site, an English scanlation site on
// the Next.js App Router. Like team-shadowi, the series page streams its whole
// dataset through the `self.__next_f.push` RSC flight payload, including every
// chapter's already-resolved page-image URLs, so a single fetch covers the
// title, the chapters and their pages. The DOM itself renders only a handful
// of chapter links (the first plus the latest few), so a selector-driven
// grabber would silently miss most chapters.
type Sacachispa struct {
	*Grabber
	data *sacachispaSeriesData
}

func NewSacachispa(g *Grabber) *Sacachispa {
	return &Sacachispa{Grabber: g}
}

// SacachispaChapter represents a sacachispa.site chapter. Its pages are
// already known from the series page fetch, so FetchChapter needs no further
// network call.
type SacachispaChapter struct {
	Chapter
	// SourceType is the chapter's hosting backend ("upload" ships the pages
	// inline; the payload also has imagechest/cubari fields for chapters
	// hosted elsewhere, which would arrive with no pages at all)
	SourceType string
}

// Test returns true if the URL is a sacachispa.site URL
func (s *Sacachispa) Test() (bool, error) {
	re := regexp.MustCompile(`sacachispa\.site`)
	return re.MatchString(s.URL), nil
}

// FetchTitle fetches and returns the manga title
func (s *Sacachispa) FetchTitle() (string, error) {
	data, err := s.fetchData()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(data.Title), nil
}

// FetchChapters returns the chapters of the manga, including their page
// images (already embedded in the series page payload)
func (s *Sacachispa) FetchChapters() (Filterables, []error) {
	data, err := s.fetchData()
	if err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(data.Chapters))
	for _, c := range data.Chapters {
		title := "Chapter " + formatChapterNumber(c.Number)
		if t := strings.TrimSpace(c.Title); t != "" {
			title += " - " + t
		}

		pages := make([]Page, 0, len(c.Pages))
		for i, url := range c.Pages {
			if strings.TrimSpace(url) == "" {
				continue
			}
			pages = append(pages, Page{
				Number: int64(i + 1),
				URL:    url,
			})
		}

		chapters = append(chapters, &SacachispaChapter{
			Chapter: Chapter{
				Number:     c.Number,
				Title:      title,
				PagesCount: int64(len(pages)),
				Pages:      pages,
				Language:   "en",
			},
			SourceType: c.SourceType,
		})
	}

	return chapters, nil
}

// FetchChapter returns the chapter and its pages. The series page fetch
// (FetchChapters) already carried every chapter's full image list, so there's
// nothing left to fetch here.
func (s Sacachispa) FetchChapter(f Filterable) (*Chapter, error) {
	schap, ok := f.(*SacachispaChapter)
	if !ok {
		return nil, errors.New("invalid chapter type")
	}

	if len(schap.Pages) == 0 {
		// only "upload" chapters ship their pages in the payload; anything
		// hosted elsewhere would pack an empty archive, so fail loudly
		return nil, fmt.Errorf(
			"no pages found for chapter %s (source type %q isn't hosted on the site)",
			formatChapterNumber(f.GetNumber()), schap.SourceType,
		)
	}

	chapter := schap.Chapter

	return &chapter, nil
}

// fetchData fetches and caches the series page's embedded RSC payload (title,
// chapters and each chapter's page images)
func (s *Sacachispa) fetchData() (*sacachispaSeriesData, error) {
	if s.data != nil {
		return s.data, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:     s.seriesURL(),
		Referer: s.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	data, err := parseSacachispaSeriesData(body)
	if err != nil {
		return nil, err
	}

	s.data = data

	return s.data, nil
}

// seriesURL maps a chapter URL down to its series URL: chapter pages carry
// only their own pages in the payload, while the series page carries
// everything, so the grabber always fetches the latter
func (s Sacachispa) seriesURL() string {
	if i := strings.Index(s.URL, "/chapter/"); i != -1 {
		return s.URL[:i]
	}
	return s.URL
}

// sacachispaTitleRe matches the series title inside the flight stream: the
// chapters' own "title" keys are null (or live inside the chapters array,
// after this marker), so anchoring on the sibling seriesId key is what keeps
// this from ever grabbing a chapter title.
var sacachispaTitleRe = regexp.MustCompile(`"seriesId":"[^"]*","title":"((?:[^"\\]|\\.)*)"`)

// parseSacachispaSeriesData extracts the series title and the `"chapters":[...]`
// array embedded in the page's Next.js RSC stream. They live in different
// react element tuples, so each is extracted on its own.
func parseSacachispaSeriesData(html string) (*sacachispaSeriesData, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	data := &sacachispaSeriesData{}

	m := sacachispaTitleRe.FindStringSubmatch(stream)
	if m == nil {
		return nil, errors.New("no series title found in the page data (is the URL a series page?)")
	}
	// the captured group is still JSON-escaped, so decode it as a JSON string
	if err := json.Unmarshal([]byte(`"`+m[1]+`"`), &data.Title); err != nil {
		return nil, err
	}

	raw, err := extractBalancedJSON(stream, `"chapters":`)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &data.Chapters); err != nil {
		return nil, err
	}

	return data, nil
}

// sacachispaSeriesData is the payload embedded in sacachispa.site series pages
type sacachispaSeriesData struct {
	Title    string
	Chapters []struct {
		// decimals like 8.5 exist, so this must stay a float
		Number float64 `json:"chapter_number"`
		// null for most chapters, in which case the site renders "Chapter N"
		Title      string   `json:"title"`
		SourceType string   `json:"source_type"`
		Pages      []string `json:"pages"`
	}
}
