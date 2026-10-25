// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
)

// Toonz is a grabber for toonz.to, a Next.js site behind a Turnstile challenge
// that only a visible browser passes. Its reader mounts <img> tags on scroll,
// so the page filenames come from the chapter's RSC payload instead.
type Toonz struct {
	*Grabber
	series *toonzSeries
}

// headless never passes its Turnstile challenge, so skip the doomed probe
func init() {
	browser.RequireVisible("toonz.to")
}

// NewToonz returns a new Toonz grabber
func NewToonz(g *Grabber) *Toonz {
	return &Toonz{Grabber: g}
}

// ToonzChapter represents a toonz.to chapter
type ToonzChapter struct {
	Chapter
	URL string
}

// toonzChapterRe matches a reader URL (/manhwa/{slug}/chapter/{n}) and captures the series URL
var toonzChapterRe = regexp.MustCompile(`(?i)^(https?://[^/]+/manhwa/[^/?#]+)/chapter/`)

// toonzCountRe extracts the site's own chapter count from the series payload
var toonzCountRe = regexp.MustCompile(`"chapterCount":(\d+)`)

// Test returns true if the URL is a toonz.to URL. It only checks the hostname
// (no fetch), since starting a browser is expensive.
func (t *Toonz) Test() (bool, error) {
	return hostMatchesDomain(t.URL, "toonz.to"), nil
}

// resolveSeriesURL rewrites a reader URL to the series URL. No-op for series URLs.
func (t *Toonz) resolveSeriesURL() {
	if m := toonzChapterRe.FindStringSubmatch(t.URL); m != nil {
		t.URL = m[1]
	}
}

// FetchTitle fetches and returns the manga title
func (t *Toonz) FetchTitle() (string, error) {
	t.resolveSeriesURL()
	series, err := t.fetchSeries()
	if err != nil {
		return "", err
	}

	return series.title, nil
}

// FetchChapters returns the chapters of the manga
func (t *Toonz) FetchChapters() (Filterables, []error) {
	t.resolveSeriesURL()
	series, err := t.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(series.chapters))
	for _, c := range series.chapters {
		number, err := strconv.ParseFloat(c.ChapterNumber, 64)
		if err != nil {
			continue
		}

		// the path uses the listing's own slug ("chapter-4.10"), since the
		// float would drop the trailing zero
		chapters = append(chapters, &ToonzChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    sanitizeTitle(c.Title),
				Language: "en",
				Label:    Label{Style: LabelNumbered},
				ID:       strconv.FormatInt(c.ID, 10),
			},
			URL: strings.TrimRight(t.URL, "/") + "/chapter/" + strings.TrimPrefix(c.Slug, "chapter-"),
		})
	}

	return chapters, nil
}

// FetchChapter renders the reader page and returns its pages
func (t *Toonz) FetchChapter(f Filterable) (*Chapter, error) {
	tchap, ok := f.(*ToonzChapter)
	if !ok {
		return nil, errors.New("invalid chapter type")
	}

	body, err := browser.GetHTML(tchap.URL, "[data-page-index]", 0)
	if err != nil {
		return nil, err
	}

	filenames, err := toonzChapterImages(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "en",
		ID:       tchap.ID,
	}
	for _, name := range filenames {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(len(chapter.Pages) + 1),
			URL:    t.BaseUrl() + "/uploads/" + name,
		})
	}

	if len(chapter.Pages) == 0 {
		return nil, fmt.Errorf("no pages found for chapter %s", formatChapterNumber(f.GetNumber()))
	}

	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}

// toonzSeries is what the series page's payload gives: the title and the chapter list
type toonzSeries struct {
	title    string
	chapters []toonzListedChapter
}

// toonzListedChapter is one entry of the series payload's initialChapters
type toonzListedChapter struct {
	ID            int64  `json:"id"`
	ChapterNumber string `json:"chapterNumber"`
	Slug          string `json:"slug"`
	// null for most chapters
	Title string `json:"title"`
}

// fetchSeries fetches (and caches) the series page, reading its title and chapter list
func (t *Toonz) fetchSeries() (*toonzSeries, error) {
	if t.series != nil {
		return t.series, nil
	}

	// not the h1: the challenge page has one too (<h1>toonz.to</h1>)
	body, err := browser.GetHTML(t.URL, `a[href*="/chapter/"]`, 0)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	chapters, err := toonzSeriesChapters(body)
	if err != nil {
		return nil, err
	}

	// the list is the site's full one today; a shorter one means it started
	// paginating, so fail loudly instead of silently missing chapters
	if m := toonzCountRe.FindStringSubmatch(body); len(m) > 1 {
		if total, err := strconv.Atoi(m[1]); err == nil && len(chapters) < total {
			return nil, fmt.Errorf("the chapter list is incomplete (%d of %d chapters)", len(chapters), total)
		}
	}

	t.series = &toonzSeries{
		title:    sanitizeTitle(doc.Find("h1").First().Text()),
		chapters: chapters,
	}

	return t.series, nil
}

// toonzSeriesChapters extracts the initialChapters list out of a series page's
// RSC flight payload. The list's first entry is a reference to another part of
// the payload rather than a chapter, so the entries that aren't objects are skipped.
func toonzSeriesChapters(html string) ([]toonzListedChapter, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	raw, err := extractBalancedJSON(stream, `"initialChapters":`)
	if err != nil {
		return nil, err
	}

	var entries []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}

	var chapters []toonzListedChapter
	for _, entry := range entries {
		var c toonzListedChapter
		if err := json.Unmarshal(entry, &c); err != nil {
			continue
		}
		chapters = append(chapters, c)
	}

	return chapters, nil
}

// toonzChapterImages extracts the filenames (relative to /uploads/) of a
// reader page, from the "images" array of its RSC flight payload
func toonzChapterImages(html string) ([]string, error) {
	stream, err := nextFlightStream(html)
	if err != nil {
		return nil, err
	}

	raw, err := extractBalancedJSON(stream, `"images":`)
	if err != nil {
		return nil, err
	}

	var images []struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal([]byte(raw), &images); err != nil {
		return nil, err
	}

	filenames := make([]string, 0, len(images))
	for _, image := range images {
		filenames = append(filenames, image.Filename)
	}

	return filenames, nil
}
