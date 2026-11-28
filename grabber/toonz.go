// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
)

// Toonz is a grabber for toonz.to and its sister toonx.to, a Next.js site behind
// Cloudflare. The full chapter list comes from the API behind the series page's
// "Show all" button, the pages from the reader's RSC payload.
type Toonz struct {
	*Grabber
	series *toonzSeries

	// sessionMu serialises the session refreshes; generation counts them, so
	// chapters that fail together render the series page once. A failed refresh
	// is kept in refreshErr for the rest of the run.
	sessionMu  sync.Mutex
	generation int
	refreshErr error
	// get and render are the network steps of a reader fetch, replaced in tests
	get    func(rawURL, referer string) ([]byte, error)
	render func(seriesURL string) error
}

// headless never passes its Turnstile challenge, so skip the doomed probe
func init() {
	browser.RequireVisible("toonz.to")
}

// NewToonz returns a new Toonz grabber
func NewToonz(g *Grabber) *Toonz {
	t := &Toonz{Grabber: g, get: toonzGet}
	t.render = t.renderSeries
	return t
}

// ToonzChapter represents a toonz.to chapter
type ToonzChapter struct {
	Chapter
	URL string
}

// toonzChapterRe matches a reader URL (/{category}/{slug}/chapter/{n}) and captures the series URL
var toonzChapterRe = regexp.MustCompile(`(?i)^(https?://[^/]+/(?:manga|manhwa|adult)/[^/?#]+)/chapter/`)

// toonzSlugRe captures the series slug out of a series URL
var toonzSlugRe = regexp.MustCompile(`(?i)/(?:manga|manhwa|adult)/([^/?#]+)`)

// toonzCountRe extracts the site's own chapter count from the series payload: the
// one right after the initialChapters array, not a related card's
var toonzCountRe = regexp.MustCompile(`\],"chapterCount":(\d+)`)

// toonzComicRe captures the ID and slug of every comic card on a series page
var toonzComicRe = regexp.MustCompile(`"comicId":(\d+),"comicSlug":"([^"]+)"`)

// Test returns true if the URL is a toonz.to or toonx.to URL. It only checks the
// hostname (no fetch), since starting a browser is expensive.
func (t *Toonz) Test() (bool, error) {
	return hostMatchesDomain(t.URL, "toonz.to") || hostMatchesDomain(t.URL, "toonx.to"), nil
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

// FetchChapter fetches the reader page and returns its pages
func (t *Toonz) FetchChapter(f Filterable) (*Chapter, error) {
	tchap, ok := f.(*ToonzChapter)
	if !ok {
		return nil, errors.New("invalid chapter type")
	}

	// plain HTTP is enough once the series page has been rendered: the reader
	// ships its pages in the payload, and the browser wait selector only fit
	// some readers (/manga/ ones have no [data-page-index])
	body, err := t.readerPage(tchap.URL)
	if err != nil {
		return nil, err
	}

	filenames, err := toonzChapterImages(string(body))
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

// toonzSeries is what the series page gives: the title and the chapter list
type toonzSeries struct {
	title    string
	chapters []toonzListedChapter
}

// toonzListedChapter is one entry of the chapter API's list
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

	stream, err := nextFlightStream(body)
	if err != nil {
		return nil, err
	}

	comicID, err := toonzComicID(stream, t.URL)
	if err != nil {
		return nil, err
	}

	total, err := toonzChapterCount(stream)
	if err != nil {
		return nil, err
	}

	chapters, err := t.fetchChapterList(comicID)
	if err != nil {
		return nil, err
	}

	if err := checkToonzCount(len(chapters), total); err != nil {
		return nil, err
	}

	t.series = &toonzSeries{
		title:    sanitizeTitle(doc.Find("h1").First().Text()),
		chapters: chapters,
	}

	return t.series, nil
}

// readerPage fetches a reader page over plain HTTP. A 403 or 503 means the
// Cloudflare clearance expired (a long run, or a series the app loaded earlier),
// so the series page is rendered again and the GET retried once.
func (t *Toonz) readerPage(rawURL string) ([]byte, error) {
	t.sessionMu.Lock()
	seen := t.generation
	t.sessionMu.Unlock()

	body, err := t.get(rawURL, t.URL)
	if !toonzSessionExpired(err) {
		return body, err
	}

	if err := t.refreshSession(seen); err != nil {
		return nil, err
	}

	return t.get(rawURL, t.URL)
}

// refreshSession renders the series page unless another chapter already did
// since seen was read, so concurrent chapters trigger a single render. A failed
// render is returned to every chapter that asks for the rest of the run.
func (t *Toonz) refreshSession(seen int) error {
	t.sessionMu.Lock()
	defer t.sessionMu.Unlock()

	if t.refreshErr != nil {
		return t.refreshErr
	}
	if t.generation != seen {
		return nil
	}

	t.refreshErr = t.render(t.URL)
	t.generation++

	return t.refreshErr
}

// renderSeries renders the series page in the browser, which harvests its cookies
func (t *Toonz) renderSeries(seriesURL string) error {
	_, err := browser.GetHTML(seriesURL, `a[href*="/chapter/"]`, 0)
	return err
}

// toonzSessionExpired reports whether err is a response that a fresh session would fix
func toonzSessionExpired(err error) bool {
	var status *http.StatusError
	return errors.As(err, &status) && (status.Code == 403 || status.Code == 503)
}

// toonzGet fetches a URL over plain HTTP with the harvested session
func toonzGet(rawURL, referer string) ([]byte, error) {
	body, err := http.Get(http.RequestParams{URL: rawURL, Referer: referer})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return io.ReadAll(body)
}

// fetchChapterList reads a series' full chapter list from the JSON API its
// "Show all" button calls, with the session the series page harvested
func (t *Toonz) fetchChapterList(comicID string) ([]toonzListedChapter, error) {
	body, err := http.Get(http.RequestParams{URL: t.BaseUrl() + "/api/comics/" + comicID + "/chapters", Referer: t.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	return parseToonzChapterList(data)
}

// parseToonzChapterList decodes the chapter API's {"chapters": [...]} response
func parseToonzChapterList(data []byte) ([]toonzListedChapter, error) {
	var payload struct {
		Chapters []toonzListedChapter `json:"chapters"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	return payload.Chapters, nil
}

// toonzComicID reads the series' comic ID out of the payload. Other comics'
// cards share the page, so the ID is the one beside this series' slug.
func toonzComicID(stream, seriesURL string) (string, error) {
	m := toonzSlugRe.FindStringSubmatch(seriesURL)
	if m == nil {
		return "", fmt.Errorf("no series slug in %s", seriesURL)
	}

	for _, card := range toonzComicRe.FindAllStringSubmatch(stream, -1) {
		if strings.EqualFold(card[2], m[1]) {
			return card[1], nil
		}
	}

	return "", errors.New("could not find the series' comic ID on its page")
}

// toonzChapterCount reads the site's own chapter count out of the series payload
func toonzChapterCount(stream string) (int, error) {
	m := toonzCountRe.FindStringSubmatch(stream)
	if m == nil {
		return 0, errors.New("could not find the chapter count on the series page")
	}

	return strconv.Atoi(m[1])
}

// checkToonzCount fails a list shorter than the site's own count, so a partial
// list (a paginated API, a short response) never passes silently
func checkToonzCount(listed, total int) error {
	if listed < total {
		return fmt.Errorf("the chapter list is incomplete (%d of %d chapters)", listed, total)
	}

	return nil
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
