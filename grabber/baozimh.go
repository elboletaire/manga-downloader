// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// Baozimh is a grabber for baozimh.com (包子漫画). The series page is plain
// server-rendered HTML listing every chapter, and the reader page embeds every
// page as an <amp-img id="chapter-img-N-M" src=...> with a real CDN URL - no
// browser, no pagination, no javascript needed.
//
// The site geo-routes its visitors: mainland-China IPs get simplified-script
// pages (from www.baozimh.com, via a 301, on cn.bzmgcn.com) while everyone
// else gets traditional-script pages directly on www.baozimh.com or
// www.twmanga.com - but the markup, selectors and reader are identical in both
// scripts. Long chapters are split across several reader pages (each part is
// "第N話(k/M)" and holds up to ~50 images); parts link each other with a
// 下一頁/下一页 "next page" link (see FetchChapter).
//
// It needs its own grabber instead of a PlainHTML selector because chapter
// numbers live in the Chinese title text ("第109话"), which chapterNumberRe
// doesn't recognise, and the reader URL carries the number in a chapter_slot=
// query param that urlChapterNumberRe doesn't match either.
type Baozimh struct {
	*Grabber
	title     string
	titleOnce sync.Once
	titleErr  error
}

func NewBaozimh(g *Grabber) *Baozimh {
	return &Baozimh{Grabber: g}
}

// BaozimhChapter represents a Baozimh Chapter
type BaozimhChapter struct {
	Chapter
	URL string
}

// baozimhChapterNumberRe extracts the chapter number from the start of a
// Chinese title. The site isn't consistent about how it numbers chapters:
// some series use "第N話"/"第N话" ("第109話", "第 1.5 话"), others list a bare
// number before the title ("001 傳染", "29 小餅乾（上）", "40支援 （下）"), and
// entries may carry a free badge prefix ("【免費】第109話"). The [话話]
// character class (and the lack of any hardcoded script) covers both scripts:
// the site serves simplified script to mainland-China visitors and traditional
// to everyone else, and a hardcoded 话 would silently drop every chapter for
// the latter. Go's regexp is RE2, so the date check below is done in code
// instead of a lookahead.
var baozimhChapterNumberRe = regexp.MustCompile(`^(?:【[^】]*】\s*)?(?:第\s*)?(\d+(?:\.\d+)?)`)

// baozimhChapterNumber parses the chapter number off a title, returning false
// for entries without one. That includes not just 序章 (prologue) / 后记/後記
// (afterword) / 番外 (bonus) but also launch notices like "7月18號正式上線！"
// that begin with a digit but are followed by a date unit (月/年/日) rather
// than a chapter title.
func baozimhChapterNumber(text string) (float64, bool) {
	match := baozimhChapterNumberRe.FindStringSubmatch(text)
	if len(match) == 0 {
		return 0, false
	}
	// a number followed straight by 月/年/日 is a date, not a chapter number
	// ("7月18號正式上線！"); everything else that matched is fine, including a
	// number that runs straight into the title ("40支援 （下）")
	if rest := text[len(match[0]):]; strings.HasPrefix(rest, "月") || strings.HasPrefix(rest, "年") || strings.HasPrefix(rest, "日") {
		return 0, false
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// Test returns true if the URL points at any of the baozimh domains. The site
// geo-routes visitors across baozimh.com (both scripts), the cn.bzmgcn.com
// simplified mirror it redirects mainland-China visitors to, and the
// www.twmanga.com traditional mirror. Matching the hostname (after stripping
// "www.") rather than a substring keeps a lookalike like "baozimh.com.evil.io"
// from matching.
func (m *Baozimh) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil {
		return false, err
	}
	switch strings.TrimPrefix(u.Hostname(), "www.") {
	case "baozimh.com", "bzmgcn.com", "twmanga.com", "cnbzmg.com":
		return true, nil
	}
	return false, nil
}

// FetchTitle fetches and returns the manga title
func (m *Baozimh) FetchTitle() (string, error) {
	m.titleOnce.Do(func() {
		body, err := http.Get(http.RequestParams{
			URL: m.URL,
		})
		if err != nil {
			m.titleErr = err
			return
		}
		defer body.Close()

		doc, err := goquery.NewDocumentFromReader(body)
		if err != nil {
			m.titleErr = err
			return
		}

		// the <h1> is the site brand ("包子漫畫"), not the series title; the
		// real one lives in the .comics-detail__title element
		m.title = sanitizeTitle(doc.Find(".comics-detail__title").Text())
		if m.title == "" {
			// don't cache an empty title: root.go re-fetches it per goroutine
			// (each chapter's PackSingle calls FetchTitle), and a cached empty
			// string would silently produce " 123 - ..." filenames
			m.titleErr = errors.New("no series title found (selector drift?)")
		}
	})
	return m.title, m.titleErr
}

// FetchChapters returns the chapters of the manga
func (m *Baozimh) FetchChapters() (chapters Filterables, errs []error) {
	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}

	// The chapter list lives in two containers: #chapter-items (a short preview
	// strip) and #chapters_other_list (the full list); both render rows with the
	// same a.comics-chapters__item class. Rows outside those two containers are
	// re-listed duplicates of the same chapters, so scoping to the containers
	// (plus deduping by URL as a safety net) yields each chapter exactly once.
	seen := map[string]bool{}
	doc.Find(`#chapter-items a.comics-chapters__item, #chapters_other_list a.comics-chapters__item`).Each(func(i int, s *goquery.Selection) {
		title := sanitizeTitle(s.Find("span").Text())

		number, ok := baozimhChapterNumber(title)
		if !ok {
			// 序章 (prologue) and 后记/後記 (afterword) entries carry no "第N话"
			// number; skip them like PlainHTML skips non-chapter rows
			return
		}

		u := s.AttrOr("href", "")
		if u == "" || u == "#" {
			return
		}
		// hrefs are root-relative (/user/page_direct?...) but tolerate an
		// absolute one if the markup ever changes
		if !strings.HasPrefix(u, "http") {
			u = m.BaseUrl() + u
		}
		if seen[u] {
			return
		}
		seen[u] = true

		chapters = append(chapters, &BaozimhChapter{
			Chapter{
				Number: number,
				Title:  title,
			},
			u,
		})
	})

	return
}

// FetchChapter fetches a chapter and its pages. Long chapters are split across
// several reader pages (each part is titled "第N話(k/M)" and holds up to ~50
// images), with each part linking the next one through a 下一頁/下一页 "next
// page" link; the last part carries no such link, so following it until it
// disappears naturally terminates the loop without ever crossing into the next
// chapter. Parts overlap by a few pages (the boundary images are rendered on
// both sides), so pages are deduped by their CDN URL.
func (m *Baozimh) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*BaozimhChapter)

	pages := []Page{}
	seen := map[string]bool{}
	visited := map[string]bool{}
	url := mchap.URL

	for part := 0; part < 20; part++ {
		if visited[url] {
			break
		}
		visited[url] = true

		body, err := http.Get(http.RequestParams{URL: url})
		if err != nil {
			return nil, err
		}
		doc, err := goquery.NewDocumentFromReader(body)
		body.Close()
		if err != nil {
			return nil, err
		}

		// defence in depth: the 下一頁 link is only ever rendered between parts
		// of the same chapter, but if the site ever changed and pointed it at
		// the next chapter, stop before merging that chapter's pages in
		if n, ok := baozimhChapterNumber(strings.TrimSpace(doc.Find("title").Text())); ok && n != f.GetNumber() {
			break
		}

		// only the id="chapter-img-N-M" images are the chapter's pages; the
		// reader also renders a bottom "recommended comics" widget with
		// <amp-img> covers that carry no chapter-img id and must not be included
		doc.Find(`amp-img[id^="chapter-img-"]`).Each(func(i int, s *goquery.Selection) {
			img := s.AttrOr("src", "")
			if img == "" {
				color.Yellow("page %d of %s has no URL to fetch from 😕 (will be ignored)", i, f.GetTitle())
				return
			}
			if seen[img] {
				// already collected from a previous part's overlap
				return
			}
			seen[img] = true
			pages = append(pages, Page{
				Number: int64(len(pages) + 1),
				URL:    img,
			})
		})

		next := baozimhNextPartURL(doc)
		if next == "" {
			break
		}
		// part links are absolute (https://www.twmanga.com/...) but tolerate a
		// root-relative one if the markup ever changes
		if !strings.HasPrefix(next, "http") {
			next = m.BaseUrl() + next
		}
		url = next
	}

	if len(pages) == 0 {
		// the chapter list skips 序章/後記, so a numbered chapter that yields no
		// pages is broken (premium/early-access locked, or selector drift) -
		// fail loudly instead of silently producing an empty archive
		return nil, fmt.Errorf("chapter %q has no pages (premium/early-access locked?)", f.GetTitle())
	}

	return &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "zh",
		Pages:      pages,
	}, nil
}

// baozimhNextPartURL returns the href of a reader page's "next part" link
// (下一頁/下一页), or "" when this is the last part or a single-part chapter.
func baozimhNextPartURL(doc *goquery.Document) string {
	var href string
	doc.Find("a").EachWithBreak(func(i int, s *goquery.Selection) bool {
		text := strings.TrimSpace(s.Text())
		if strings.Contains(text, "下一頁") || strings.Contains(text, "下一页") {
			href = s.AttrOr("href", "")
			return false
		}
		return true
	})
	return href
}
