// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
)

// Japscan is a grabber for japscan.foo, whose reader asks for a puzzle about every
// second chapter. A person solves it in the browser window, then pages come over HTTP.
type Japscan struct {
	*Grabber
	series *japscanSeries
	// fetched holds the chapters rendered lately, see FetchChapter
	fetched   map[string]japscanFetched
	fetchedMu sync.Mutex
}

// japscanDomains are the domains the site answers on: the live one first, then
// the ones it moved from. A rotation means a new first entry.
var japscanDomains = []string{"japscan.foo", "japscan.lol", "japscan.com", "japscan.vip"}

// japscanFetchedFor is how long a rendered chapter is kept: its page URLs expire
// within minutes, and a bundle only needs the second fetch soon after the first
const japscanFetchedFor = 5 * time.Minute

// japscanPuzzle is the JS that is true while the reader shows its puzzle
const japscanPuzzle = `window.__captcha && window.__captcha.needed === true`

// japscanReady is the JS that is true once the reader has its pages
const japscanReady = `window.__captcha && window.__captcha.needed === false && document.querySelectorAll("select#pages option").length > 0`

// japscanCountJS is the JS number of the reader's pages
const japscanCountJS = `document.querySelectorAll("select#pages option").length`

// japscanSeries is what the series page gives: the title and the chapter list
type japscanSeries struct {
	title    string
	chapters []*JapscanChapter
}

// JapscanChapter represents a japscan.foo chapter
type JapscanChapter struct {
	Chapter
	URL string
}

// japscanFetched is a chapter rendered by FetchChapter, with when it was
type japscanFetched struct {
	chapter *Chapter
	at      time.Time
}

// japscanSeriesRe matches the reader URL of a chapter (/manga/{slug}/{chapter}/),
// capturing the series URL
var japscanSeriesRe = regexp.MustCompile(`(?i)^(https?://[^/]+/[^/?#]+/[^/?#]+)/[^/?#]+/?(?:[?#].*)?$`)

// japscanPathRe matches a chapter path, capturing its number
var japscanPathRe = regexp.MustCompile(`^/[^/]+/[^/]+/(\d+(?:\.\d+)?)/$`)

// japscanTitleRe parses the chapter text of the list: its number and title
var japscanTitleRe = regexp.MustCompile(`^Chapitre\s+(\d+(?:\.\d+)?)\s*:?\s*(.*)$`)

// japscanVolumeRe matches a volume header's number
var japscanVolumeRe = regexp.MustCompile(`Volume\s+(\d+)`)

// japscanImageRe matches the page images the reader loads
var japscanImageRe = regexp.MustCompile(`^https://c\d+\.japscan\.[a-z]+/`)

func init() {
	for _, d := range japscanDomains {
		browser.RequireVisible(d)
	}
}

// NewJapscan returns a new Japscan grabber
func NewJapscan(g *Grabber) *Japscan {
	return &Japscan{Grabber: g}
}

// Test returns true if the URL is one of the japscan domains. It only checks
// the hostname (no fetch), since starting a browser is expensive.
func (j *Japscan) Test() (bool, error) {
	for _, d := range japscanDomains {
		if hostMatchesDomain(j.URL, d) {
			return true, nil
		}
	}
	return false, nil
}

// resolveSeriesURL moves the URL to the live domain and turns a chapter URL
// into the series URL
func (j *Japscan) resolveSeriesURL() {
	if u, err := url.Parse(j.URL); err == nil {
		for _, d := range japscanDomains[1:] {
			if hostMatchesDomain(j.URL, d) {
				u.Host = "www." + japscanDomains[0]
				j.URL = u.String()
				break
			}
		}
	}
	if m := japscanSeriesRe.FindStringSubmatch(j.URL); m != nil {
		j.URL = m[1] + "/"
	}
}

// FetchTitle fetches and returns the manga title
func (j *Japscan) FetchTitle() (string, error) {
	j.resolveSeriesURL()
	series, err := j.fetchSeries()
	if err != nil {
		return "", err
	}

	return series.title, nil
}

// FetchChapters returns the chapters of the manga
func (j *Japscan) FetchChapters() (Filterables, []error) {
	j.resolveSeriesURL()
	series, err := j.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(series.chapters))
	for _, c := range series.chapters {
		chapters = append(chapters, c)
	}

	return chapters, nil
}

// FetchChapter renders the reader page, once the user has solved its puzzle,
// and returns its pages
func (j *Japscan) FetchChapter(f Filterable) (*Chapter, error) {
	jchap, ok := f.(*JapscanChapter)
	if !ok {
		return nil, errors.New("invalid chapter type")
	}

	// a bundle fetches each chapter twice, and every render costs a puzzle
	if chapter := j.recentChapter(jchap.URL); chapter != nil {
		return chapter, nil
	}

	html, reqs, err := browser.GetPageRequests(jchap.URL, browser.PageWait{
		Match:  japscanImageRe,
		Puzzle: japscanPuzzle,
		Ready:  japscanReady,
		Count:  japscanCountJS,
	}, 0)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	urls, err := japscanPages(reqs, japscanPageCount(doc))
	if err != nil {
		return nil, fmt.Errorf("chapter %s: %w", formatChapterNumber(f.GetNumber()), err)
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "fr",
		ID:       jchap.ID,
		Volume:   jchap.Volume,
	}
	for i, u := range urls {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: u})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	j.rememberChapter(jchap.URL, chapter)
	return chapter, nil
}

// recentChapter returns a copy of the chapter fetched from url within
// japscanFetchedFor, nil otherwise
func (j *Japscan) recentChapter(url string) *Chapter {
	j.fetchedMu.Lock()
	defer j.fetchedMu.Unlock()
	got, ok := j.fetched[url]
	if !ok || time.Since(got.at) > japscanFetchedFor {
		return nil
	}
	chapter := *got.chapter
	chapter.Pages = slices.Clone(got.chapter.Pages)
	return &chapter
}

// rememberChapter keeps a copy of a chapter fetched from url
func (j *Japscan) rememberChapter(url string, chapter *Chapter) {
	j.fetchedMu.Lock()
	defer j.fetchedMu.Unlock()
	if j.fetched == nil {
		j.fetched = map[string]japscanFetched{}
	}
	kept := *chapter
	kept.Pages = slices.Clone(chapter.Pages)
	j.fetched[url] = japscanFetched{chapter: &kept, at: time.Now()}
}

// japscanPageCount is how many pages the reader's page selector lists
func japscanPageCount(doc *goquery.Document) int {
	return doc.Find("select#pages option").Length()
}

// japscanPages returns the page image URLs in order, each once. The URLs are all
// that's needed, so every Image counts, loaded or not; the worker-token Fetch isn't a page.
// It fails unless there are want of them, saying how many failed or never finished.
func japscanPages(reqs []browser.Request, want int) ([]string, error) {
	seen := map[string]bool{}
	var urls []string
	var failed, unfinished int
	for _, r := range reqs {
		if r.Type != "Image" || seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		urls = append(urls, r.URL)
		switch {
		case r.Failed:
			failed++
		case !r.Loaded:
			unfinished++
		}
	}
	if len(urls) != want {
		return nil, fmt.Errorf("found %d page images for the %d pages (%d failed to load, %d never finished)", len(urls), want, failed, unfinished)
	}
	return urls, nil
}

// fetchSeries fetches (and caches) the series page, reading its title and chapter list.
// The series page has no puzzle; the reader does.
func (j *Japscan) fetchSeries() (*japscanSeries, error) {
	if j.series != nil {
		return j.series, nil
	}

	body, err := browser.GetHTML(j.URL, "#list_chapters", 0)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	chapters, err := japscanChapters(doc, j.BaseUrl())
	if err != nil {
		return nil, err
	}

	j.series = &japscanSeries{
		title:    sanitizeTitle(doc.Find("h1").First().Text()),
		chapters: chapters,
	}

	return j.series, nil
}

// japscanChapters reads the chapter list of a series page. The rows hold
// hidden decoys (a chapter 000001 and the like), so only the visible links
// count: the ones with no clip-path, whose number matches their path.
func japscanChapters(doc *goquery.Document, base string) ([]*JapscanChapter, error) {
	var chapters []*JapscanChapter
	seen := map[string]bool{}
	doc.Find("#list_chapters div.list_chapters").Each(func(_ int, row *goquery.Selection) {
		volume := 0
		if m := japscanVolumeRe.FindStringSubmatch(row.Parent().PrevAllFiltered("h4").First().Text()); m != nil {
			volume, _ = strconv.Atoi(m[1])
		}

		row.Children().Each(func(_ int, el *goquery.Selection) {
			style := el.AttrOr("style", "")
			if el.HasClass("d-none") || strings.Contains(style, "clip-path") || strings.Contains(style, "position:absolute") {
				return
			}
			path := japscanPath(el)
			if path == "" || seen[path] {
				return
			}
			text := strings.TrimSpace(strings.ReplaceAll(el.Text(), "\u200b", ""))
			m := japscanTitleRe.FindStringSubmatch(text)
			if m == nil || japscanPadded(m[1]) || m[1] != japscanPathRe.FindStringSubmatch(path)[1] {
				return
			}
			number, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				return
			}
			seen[path] = true
			chapters = append(chapters, &JapscanChapter{
				Chapter: Chapter{
					Number:   number,
					Title:    sanitizeTitle(m[2]),
					Language: "fr",
					Label:    Label{Style: LabelNumbered, Lang: "fr"},
					Volume:   volume,
					ID:       path,
				},
				URL: base + path,
			})
		})
	})

	if len(chapters) == 0 {
		return nil, errors.New("no chapters found (the list markup changed?)")
	}

	return chapters, nil
}

// japscanPadded reports whether a chapter number is zero-padded ("000001"), which
// is how the site's hidden decoy rows number their chapters
func japscanPadded(number string) bool {
	return len(number) > 1 && number[0] == '0' && number[1] != '.'
}

// japscanPath returns the chapter path an element carries in one of its
// (random-named) attributes, "" when none does
func japscanPath(el *goquery.Selection) string {
	for _, attr := range el.Nodes[0].Attr {
		switch {
		case attr.Key == "href", attr.Key == "class", attr.Key == "style", attr.Key == "id", attr.Key == "title", strings.HasPrefix(attr.Key, "data-"):
			continue
		}
		if japscanPathRe.MatchString(attr.Val) {
			return attr.Val
		}
	}
	return ""
}
