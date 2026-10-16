// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

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

// tcbSeriesRe matches the series path of a Madara URL: /manga/<slug>/ on most
// themes, /gl/<slug>/ on some (lilymanga.net). The slug must end in a slash,
// so a pasted chapter URL such as /manga/<slug>/<chapter>/ maps to its series,
// while /somanga/ and /glimpse/ don't match at all.
var tcbSeriesRe = regexp.MustCompile(`/((?:manga|gl)/[^/]+)/`)

// Test returns true if the URL is a compatible TCBScans URL
func (t *Tcb) Test() (bool, error) {
	match := tcbSeriesRe.FindStringSubmatch(t.URL)
	if match == nil {
		return false, nil
	}

	t.series = t.BaseUrl() + "/" + match[1] + "/"
	uri, _ := url.JoinPath(t.BaseUrl(), match[1], "ajax", "chapters")

	rbody, err := http.Post(http.RequestParams{
		URL:          uri,
		Referer:      t.BaseUrl(),
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
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

	// Madara prints rank badges inside the h1 and indents its text; some themes
	// add a summary h1 outside .post-title, so never join them.
	h1 := body.Find(".post-title h1").First()
	if h1.Length() == 0 {
		h1 = body.Find("h1").First()
	}
	h1 = h1.Clone()
	h1.Find(".manga-title-badges").Remove()
	t.title = sanitizeTitle(h1.Text())

	return t.title, nil
}

// FetchChapters returns a slice of chapters
func (t Tcb) FetchChapters() (chapters Filterables, errs []error) {
	t.chaps.Each(func(i int, s *goquery.Selection) {
		// fetch title (usually "Chapter N")
		link := s.Find("a")
		if len(link.Children().Nodes) > 0 {
			link.Children().Remove()
		}
		title := strings.TrimSpace(link.Text())
		href := link.AttrOr("href", "")
		num := tcbChapterNumber(title, href)
		title, group := t.splitTitleGroup(title)
		chapter := &TcbChapter{
			Chapter{
				Title:  title,
				Number: num,
				Volume: tcbVolume(s),
				ID:     chapterURLID(href),
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
// no pages to pack.
func tcbReaderTypeError(doc *goquery.Document, title string) error {
	class := doc.Find(".reading-content-wrap").First().AttrOr("class", "")
	switch {
	case strings.Contains(class, "chapter-type-video"):
		return fmt.Errorf("%s is a video, not a manga chapter", title)
	case strings.Contains(class, "chapter-type-text"):
		return fmt.Errorf("%s is a novel chapter, not a manga chapter", title)
	}
	return nil
}

// FetchChapter fetches a chapter and its pages
func (t Tcb) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*TcbChapter)

	// Get first page to find all page URLs
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

	// Get all page URLs from the single-pager select
	pageURLs := []string{}
	body.Find("#single-pager option").Each(func(i int, s *goquery.Selection) {
		if url := s.AttrOr("data-redirect", ""); url != "" {
			pageURLs = append(pageURLs, url)
		}
	})

	// If no pages found in select, this might be a single-page chapter
	if len(pageURLs) == 0 {
		pageURLs = append(pageURLs, tchap.URL)
	}

	pages := []Page{}
	// Visit each page URL to get its image
	for pageNum, pageURL := range pageURLs {
		// Fetch page content
		rbody, err := http.Get(http.RequestParams{
			URL:     pageURL,
			Referer: t.BaseUrl(),
		})
		if err != nil {
			notice.Warn("error fetching page %d: %s", pageNum+1, err.Error())
			continue
		}

		pageDoc, err := goquery.NewDocumentFromReader(rbody)
		rbody.Close()
		if err != nil {
			notice.Warn("error parsing page %d: %s", pageNum+1, err.Error())
			continue
		}

		if err := tcbReaderTypeError(pageDoc, f.GetTitle()); err != nil {
			return nil, err
		}

		if urls, ok, err := wpMangaProtectorImages(pageDoc); ok {
			if err != nil {
				notice.Warn("could not decode the protected pages of %s: %s", f.GetTitle(), err)
			}
			for _, u := range urls {
				pages = append(pages, Page{Number: int64(pageNum + 1), URL: u})
			}
			continue
		}

		// Collect all images in this page
		pageDoc.Find("div.reading-content img").Each(func(i int, s *goquery.Selection) {
			u := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", "")))
			if u == "" {
				notice.Warn("page %d of %s has an image with no URL to fetch from", pageNum+1, f.GetTitle())
				return
			}
			if isThemeImage(u) {
				return
			}
			if !strings.HasPrefix(u, "http") {
				u = t.BaseUrl() + u
			}
			pages = append(pages, Page{
				Number:     int64(pageNum + 1),
				URL:        u,
				RetryFlaky: t.retryFlaky(),
			})
		})
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
