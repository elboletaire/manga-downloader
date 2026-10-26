// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
)

// Tcb is a grabber for Madara-based wordpress sites (i.e. lhtranslation.net)
type Tcb struct {
	*Grabber
	chaps  *goquery.Selection
	title  string
	series string // series URL, which a pasted chapter URL is reduced to
}

func NewTcb(g *Grabber) *Tcb {
	return &Tcb{Grabber: g}
}

// TcbChapter is a chapter for TCBScans
type TcbChapter struct {
	Chapter
	URL string
}

// tcbKnownSeriesRe matches a series under /manga/, /gl/, /seri/ or /read-N/, keeping
// the folders before it (/en/manga/x/, WordPress in a subfolder). The slug must end
// in a slash, so a chapter URL maps to its series.
var tcbKnownSeriesRe = regexp.MustCompile(`^https?://[^/]+/((?:[^/?#]+/)*?(?:manga|gl|seri|read-\d+)/[^/?#]+)/`)

// tcbSeriesRe matches any other series path, /<seg>/<slug>/ (/comics/, /series/...).
// The first segment can't be all digits, so a blog's /2026/09/post.html isn't one.
var tcbSeriesRe = regexp.MustCompile(`^https?://[^/]+/((?:[^/?#]*[^/?#\d][^/?#]*)/[^/?#]+)/`)

// tcbSeriesPath returns the series path of a Madara URL, or "" when it has none.
// The known prefixes are tried first.
func tcbSeriesPath(u string) string {
	for _, re := range []*regexp.Regexp{tcbKnownSeriesRe, tcbSeriesRe} {
		if m := re.FindStringSubmatch(u); m != nil {
			return m[1]
		}
	}
	return ""
}

// tcbMovedHostRe matches diamondfansub.com, which now redirects every series
// and chapter page to webtoonia.fun (a chapter URL there lands on the series),
// and www.webtoonia.fun, which redirects to the apex.
var tcbMovedHostRe = regexp.MustCompile(`(?i)^(https?://)(?:www\.)?(?:diamondfansub|webtoonia)\.(?:com|fun)/`)

// Test returns true if the URL is a compatible TCBScans URL
func (t *Tcb) Test() (bool, error) {
	t.URL = tcbMovedHostRe.ReplaceAllString(t.URL, "${1}webtoonia.fun/")
	series := tcbSeriesPath(t.URL)
	if series == "" {
		return false, nil
	}

	t.series = t.BaseUrl() + "/" + series + "/"
	uri, _ := url.JoinPath(t.BaseUrl(), series, "ajax", "chapters")

	rbody, err := http.Post(http.RequestParams{
		URL:          uri,
		Referer:      t.BaseUrl(),
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		// a status error declines quietly: with any path accepted, most such URLs
		// aren't Madara at all, so a 404 here isn't a failure to report
		var statusErr *http.StatusError
		if errors.As(err, &statusErr) {
			return false, nil
		}
		return false, err
	}

	page, err := io.ReadAll(rbody)
	rbody.Close()
	if err != nil {
		return false, err
	}
	body, err := goquery.NewDocumentFromReader(bytes.NewReader(page))
	if err != nil {
		return false, err
	}

	// Scope to actual chapter rows: some Madara themes (e.g. gdscans.com)
	// group chapters under "Volume N" <li> wrappers, and a bare "li" match
	// would pick up those volume headers (and their empty list-wrapper <li>)
	// as bogus duplicate chapters alongside the real ones.
	t.chaps = body.Find("li.wp-manga-chapter")
	if t.chaps.Length() > 0 {
		if t.chaps, err = t.fetchListingPages(uri, t.chaps, tcbLastPage(body)); err != nil {
			return false, err
		}
	}
	if t.chaps.Length() == 0 {
		// Some installs answer with the series page, which names the manga id
		// admin-ajax.php needs.
		if id := tcbMangaIDRe.FindSubmatch(page); id != nil {
			if t.chaps, err = t.fetchAdminAjaxChapters(string(id[1])); err != nil {
				return false, nil
			}
		}
	}

	return t.chaps.Length() > 0, nil
}

var tcbMangaIDRe = regexp.MustCompile(`"manga_id":"(\d+)"`)

// tcbMaxListingPages caps how many pages of a chapter list are read.
const tcbMaxListingPages = 100

// tcbPageRe matches the ?t=N page links of a paginated chapter list.
var tcbPageRe = regexp.MustCompile(`[?&]t=(\d+)`)

// tcbLastPage returns the last page of a chapter list's pager, 0 or 1 when the list
// has none (a single request answers it all).
func tcbLastPage(body *goquery.Document) int {
	last := 0
	body.Find("div.pagination a[href]").Each(func(_ int, a *goquery.Selection) {
		if m := tcbPageRe.FindStringSubmatch(a.AttrOr("href", "")); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > last {
				last = n
			}
		}
	})
	return last
}

// fetchListingPages adds the rows of pages 2 to last of a paginated chapter list
// (?t=N on the ajax endpoint), stopping early at a page with no new chapter.
func (t *Tcb) fetchListingPages(uri string, rows *goquery.Selection, last int) (*goquery.Selection, error) {
	seen := map[string]bool{}
	rows.Each(func(_ int, s *goquery.Selection) {
		seen[s.Find("a").AttrOr("href", "")] = true
	})
	for page := 2; page <= min(last, tcbMaxListingPages); page++ {
		rbody, err := http.Post(http.RequestParams{
			URL:          fmt.Sprintf("%s?t=%d", uri, page),
			Referer:      t.BaseUrl(),
			DisableRetry: true,
		})
		if err != nil {
			return nil, err
		}
		doc, err := goquery.NewDocumentFromReader(rbody)
		rbody.Close()
		if err != nil {
			return nil, err
		}
		added := doc.Find("li.wp-manga-chapter").FilterFunction(func(_ int, s *goquery.Selection) bool {
			href := s.Find("a").AttrOr("href", "")
			if seen[href] {
				return false
			}
			seen[href] = true
			return true
		})
		if added.Length() == 0 {
			break
		}
		rows = rows.AddSelection(added)
	}
	return rows, nil
}

// fetchAdminAjaxChapters reads the list through admin-ajax.php for the Madara
// installs whose ajax/chapters endpoint doesn't return it.
func (t *Tcb) fetchAdminAjaxChapters(id string) (*goquery.Selection, error) {
	rbody, err := http.Post(http.RequestParams{
		URL:          t.BaseUrl() + "/wp-admin/admin-ajax.php",
		Referer:      t.series,
		Body:         "action=manga_get_chapters&manga=" + id,
		DisableRetry: true,
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()
	doc, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return nil, err
	}
	return doc.Find("li.wp-manga-chapter"), nil
}

// GetTitle fetches and returns the manga title
func (t *Tcb) FetchTitle() (string, error) {
	if t.title != "" {
		return t.title, nil
	}

	rbody, err := http.Get(http.RequestParams{
		URL: t.series,
	})
	if err != nil {
		return "", err
	}
	defer rbody.Close()
	body, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return "", err
	}

	t.title = tcbSeriesTitle(body, t.BaseUrl())
	return t.title, nil
}

// tcbSeriesTitle reads a series' name off its page, skipping the site's logo heading,
// then falls back to the breadcrumb or JSON-LD headline of themes without one.
func tcbSeriesTitle(body *goquery.Document, base string) string {
	// Madara prints rank badges inside the h1 and indents its text; some themes
	// add a summary h1 outside .post-title, so never join them.
	h1 := body.Find(".post-title h1").First()
	if h1.Length() == 0 {
		h1 = body.Find("h1").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return !tcbIsSiteRoot(s.Closest("a").AttrOr("href", ""), base)
		}).First()
	}
	if h1.Length() > 0 {
		h1 = h1.Clone()
		h1.Find(".manga-title-badges").Remove()
		if title := sanitizeTitle(h1.Text()); title != "" {
			return title
		}
	}
	if crumb := sanitizeTitle(body.Find("ol.breadcrumb > li").Last().Text()); crumb != "" {
		return crumb
	}
	return tcbHeadlineTitle(body)
}

// tcbIsSiteRoot reports whether a link points at the site's home page: "/", or
// the base URL with or without its www. prefix. An empty or fragment link isn't.
func tcbIsSiteRoot(href, base string) bool {
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	if u.Host == "" {
		return u.Path == "/"
	}
	b, err := url.Parse(base)
	if err != nil {
		return false
	}
	same := strings.TrimPrefix(u.Hostname(), "www.") == strings.TrimPrefix(b.Hostname(), "www.")
	return same && strings.Trim(u.Path, "/") == "" && u.RawQuery == ""
}

// tcbHeadlineTitle returns the headline of the first JSON-LD block that has one.
func tcbHeadlineTitle(body *goquery.Document) string {
	var title string
	body.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		var ld struct {
			Headline string `json:"headline"`
		}
		if json.Unmarshal([]byte(s.Text()), &ld) == nil {
			title = sanitizeTitle(ld.Headline)
		}
		return title == ""
	})
	return title
}

// tcbStrayChapterRe matches a chapter slug that names its series before "-chapter-".
var tcbStrayChapterRe = regexp.MustCompile(`^(.+)-chapter-`)

// tcbOwnRows drops the rows of other series that a listing files under this one
// (mangaowl.io lists another series' chapter 1 on beyond-the-limit). It acts only
// when some row carries the series slug, so listings that never do keep every row.
func tcbOwnRows(rows *goquery.Selection, series string) *goquery.Selection {
	slug := tcbLastSegment(series)
	if slug == "" {
		return rows
	}
	slugOf := func(s *goquery.Selection) string {
		return tcbLastSegment(s.Find("a").AttrOr("href", ""))
	}
	named := false
	rows.Each(func(_ int, s *goquery.Selection) {
		named = named || strings.HasPrefix(slugOf(s), slug+"-")
	})
	if !named {
		return rows
	}
	return rows.FilterFunction(func(_ int, s *goquery.Selection) bool {
		m := tcbStrayChapterRe.FindStringSubmatch(slugOf(s))
		return m == nil || strings.HasPrefix(m[1], slug)
	})
}

// tcbLastSegment returns the last path segment of a URL, ignoring a trailing slash.
func tcbLastSegment(u string) string {
	u = strings.TrimRight(u, "/")
	return u[strings.LastIndex(u, "/")+1:]
}

// FetchChapters returns a slice of chapters
func (t Tcb) FetchChapters() (chapters Filterables, errs []error) {
	tcbOwnRows(t.chaps, t.series).Each(func(i int, s *goquery.Selection) {
		// fetch title (usually "Chapter N")
		link := s.Find("a")
		if len(link.Children().Nodes) > 0 {
			link.Children().Remove()
		}
		title := strings.TrimSpace(link.Text())
		// webtoonia.fun tags members-only chapters with a trailing " - 💦" (other sites may mean something else by it)
		locked := false
		if strings.Contains(t.URL, "://webtoonia.fun/") {
			title, locked = strings.CutSuffix(title, tcbLockedSuffix)
		}
		href := link.AttrOr("href", "")
		num := tcbChapterNumber(title, href)
		title, group := t.splitTitleGroup(title)
		chapter := &TcbChapter{
			Chapter{
				Title:  title,
				Number: num,
				Volume: tcbVolume(s),
				ID:     chapterURLID(href),
				Locked: locked,
			},
			href,
		}

		chapter.SetGroups(group)
		chapters = append(chapters, chapter)
	})

	return
}

// tcbTitleGroups maps sites that glue the scanlation group to a chapter title
// ("30th Battle - REDLION") to the group names they use; the row has no markup
// for it, and other trailing parts ("Part 1") are real titles.
var tcbTitleGroups = map[string][]string{
	"reader.decadencescans.com": {"Transcendence", "Friendship Scans", "REDLION", "Dragomir Scans", "Phoenix Serenade Scans", "Lazy Scans"},
}

// splitTitleGroup moves a trailing " - <group>" off the title on the sites in
// tcbTitleGroups, leaving other titles untouched.
func (t Tcb) splitTitleGroup(title string) (string, string) {
	u, _ := url.Parse(t.URL)
	for _, g := range tcbTitleGroups[u.Host] {
		n := len(title) - len(" - "+g)
		if n >= 0 && strings.EqualFold(title[n:], " - "+g) {
			return title[:n], g
		}
	}
	return title, ""
}

const tcbLockedSuffix = " - 💦"

var tcbNumberRe = regexp.MustCompile(`\d+\.?\d*`)

// tcbChapterNumber reads a row's number from its title, then from the last
// segment of its URL. Unnumbered rows ("One-shot") get 0 instead of failing.
func tcbChapterNumber(title, href string) float64 {
	last := strings.TrimRight(href, "/")
	last = last[strings.LastIndex(last, "/")+1:]
	for _, s := range []string{title, last} {
		if num, err := strconv.ParseFloat(tcbNumberRe.FindString(s), 64); err == nil {
			return num
		}
	}
	return 0
}

// tcbVolume returns the volume of a chapter row from the "Volume N" wrapper
// some Madara themes group the list under, 0 for a flat list
func tcbVolume(row *goquery.Selection) int {
	header := row.Closest("li.parent.has-child").ChildrenFiltered("a.has-child")
	if header.Length() == 0 {
		return 0
	}
	// the header may carry a title after the number ("Volume 2: Arc name")
	m := volumeNumberRe.FindStringSubmatch(header.First().Text())
	if m == nil {
		return 0
	}
	return parseVolume(m[1])
}

// tcbReaderTypeError rejects Madara chapters that aren't images: the reader
// wrapper's chapter-type-* class marks videos (anime) and novels, which have
// no pages to pack. A text reader without prose is a comic, though.
func tcbReaderTypeError(doc *goquery.Document, title string) error {
	class := doc.Find(".reading-content-wrap").First().AttrOr("class", "")
	switch {
	case strings.Contains(class, "chapter-type-video"):
		return fmt.Errorf("%s is a video, not a manga chapter", title)
	case strings.Contains(class, "chapter-type-text") && tcbHasProse(doc):
		return fmt.Errorf("%s is a novel chapter, not a manga chapter", title)
	}
	return nil
}

// FetchChapter fetches a chapter and its pages
func (t Tcb) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*TcbChapter)

	rbody, err := http.Get(http.RequestParams{
		URL: tchap.URL,
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()
	body, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return nil, err
	}
	if err := tcbReaderTypeError(body, ChapterLabel(f)); err != nil {
		return nil, err
	}

	var pages []Page
	// Protected readers (wp-manga-chapter-images-protection) don't ship this
	// list, so this branch needs no decoder.
	if urls := tcbPreloadedImages(body); len(urls) > 0 {
		for i, u := range urls {
			if u = strings.TrimSpace(u); u == "" || isThemeImage(u) {
				continue
			}
			pages = append(pages, t.readerImage(i, u))
		}
	} else if pages, err = t.walkPager(f, body); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)), // Use actual number of found pages
		Language:   "en",
		Pages:      pages,
		ID:         chapterURLID(tchap.URL),
	}
	chapter.SetGroups(tchap.GetGroups()...)

	return chapter, nil
}

var tcbPreloadedRe = regexp.MustCompile(`chapter_preloaded_images\s*=\s*`)

// tcbPreloadedImages returns the chapter's image list that Madara's paged reader
// embeds on every reader page, saving one GET per page.
func tcbPreloadedImages(doc *goquery.Document) []string {
	script := doc.Find("script#chapter_preloaded_images").Text()
	loc := tcbPreloadedRe.FindStringIndex(script)
	if loc == nil {
		return nil
	}
	var urls []string
	if err := json.NewDecoder(strings.NewReader(script[loc[1]:])).Decode(&urls); err != nil {
		return nil
	}
	return urls
}

// walkPager collects the images of a paged reader one page at a time, or of
// the chapter page itself when it has no pager.
func (t Tcb) walkPager(f Filterable, body *goquery.Document) ([]Page, error) {
	// The pager is rendered more than once (top and bottom), with the same pages.
	pageURLs := []string{}
	seen := map[string]bool{}
	body.Find("#single-pager option").Each(func(i int, s *goquery.Selection) {
		if url := s.AttrOr("data-redirect", ""); url != "" && !seen[url] {
			seen[url] = true
			pageURLs = append(pageURLs, url)
		}
	})
	if len(pageURLs) == 0 {
		return t.readerPageImages(f, 0, body), nil
	}

	// Paged readers cost one HTML GET per page, so walk them concurrently and
	// flatten in page order.
	results := make([][]Page, len(pageURLs))
	errs := make([]error, len(pageURLs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, tcbPageWorkers)
	for pageNum, pageURL := range pageURLs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[pageNum], errs[pageNum] = t.fetchReaderPage(f, pageNum, pageURL)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	pages := []Page{}
	for _, r := range results {
		pages = append(pages, r...)
	}
	return pages, nil
}

// tcbPageWorkers bounds the concurrent reader-page fetches of one chapter.
const tcbPageWorkers = 8

// fetchReaderPage returns the images of one reader page, warning and returning
// none when the page can't be fetched or parsed, and an error for a reader
// that isn't a manga chapter.
func (t Tcb) fetchReaderPage(f Filterable, pageNum int, pageURL string) ([]Page, error) {
	rbody, err := http.Get(http.RequestParams{
		URL:     pageURL,
		Referer: t.BaseUrl(),
	})
	if err != nil {
		notice.Warn("error fetching page %d: %s", pageNum+1, err.Error())
		return nil, nil
	}

	pageDoc, err := goquery.NewDocumentFromReader(rbody)
	rbody.Close()
	if err != nil {
		notice.Warn("error parsing page %d: %s", pageNum+1, err.Error())
		return nil, nil
	}

	if err := tcbReaderTypeError(pageDoc, ChapterLabel(f)); err != nil {
		return nil, err
	}
	return t.readerPageImages(f, pageNum, pageDoc), nil
}

// tcbHasProse reports whether a text reader still has prose once its images,
// scripts and Madara's warning box are removed, so a novel with an illustration
// stays a novel. Readers without .reading-content are read as a whole.
func tcbHasProse(doc *goquery.Document) bool {
	content := doc.Find(".reading-content").First()
	if content.Length() == 0 {
		content = doc.Find(".reading-content-wrap").First()
	}
	content = content.Clone()
	content.Find("script, style, input, img, canvas, .chapter-warning").Remove()
	return strings.TrimSpace(content.Text()) != ""
}

// tcbPageSelector matches the page elements of a reader: its images, or the
// canvases some themes paint pages on, whose URL is data-page-url.
const tcbPageSelector = "div.reading-content img, div.reading-content canvas[data-page-url]"

// tcbImageURL returns a reader image's URL from the first lazy-load attribute
// that holds one. src comes last, since lazy-loading plugins leave a data:
// placeholder there (arthurscan's lzl plugin keeps the real URL in data-lzl-src).
func tcbImageURL(s *goquery.Selection) string {
	for _, attr := range []string{"data-src", "data-lazy-src", "data-lzl-src", "src", "data-page-url"} {
		if u := strings.TrimSpace(s.AttrOr(attr, "")); u != "" && !strings.HasPrefix(u, "data:") {
			return u
		}
	}
	return ""
}

// readerImage builds the page for an image URL found on reader page pageNum.
func (t Tcb) readerImage(pageNum int, u string) Page {
	if !strings.HasPrefix(u, "http") {
		u = t.BaseUrl() + u
	}
	return Page{
		Number:     int64(pageNum + 1),
		URL:        u,
		RetryFlaky: t.retryFlaky(),
	}
}

// readerPageImages returns the images of reader page pageNum.
func (t Tcb) readerPageImages(f Filterable, pageNum int, pageDoc *goquery.Document) (pages []Page) {
	if urls, ok, err := wpMangaProtectorImages(pageDoc); ok {
		if err != nil {
			notice.Warn("could not decode the protected pages of %s: %s", ChapterLabel(f), err)
		}
		for _, u := range urls {
			pages = append(pages, Page{Number: int64(pageNum + 1), URL: u})
		}
		return pages
	}

	pageDoc.Find(tcbPageSelector).Each(func(i int, s *goquery.Selection) {
		u := tcbImageURL(s)
		if u == "" {
			notice.Warn("page %d of %s has an image with no URL to fetch from", pageNum+1, ChapterLabel(f))
			return
		}
		if !isThemeImage(u) {
			pages = append(pages, t.readerImage(pageNum, u))
		}
	})
	return pages
}
