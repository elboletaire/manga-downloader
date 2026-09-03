// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	nethttp "net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// Baozimh is a grabber for baozimh.com (包子漫画). The series page is plain
// server-rendered HTML listing every chapter, and the reader page embeds every
// page as an <amp-img id="chapter-img-N-M" src=...> - no browser, no
// pagination, no javascript needed to find the pages. The embedded URL's
// host is a placeholder, though (see resolveBzcdnHost): the reader's own JS
// normally swaps it for a live regional edge on page load, so FetchChapter
// does that resolution itself before returning page URLs.
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
	// the parsed series page is fetched once and shared between FetchTitle and
	// FetchChapters, which would otherwise each re-fetch (and re-download) the
	// same page. All receivers are pointers so these writes are never discarded.
	docOnce sync.Once
	doc     *goquery.Document
	docErr  error
	// cdnSuffixOnce/cdnSuffix cache the outcome of resolveBzcdnHost's probe
	// for the lifetime of this Baozimh instance (one download run), so only
	// the first bare-CDN image URL triggers a round of probing.
	cdnSuffixOnce sync.Once
	cdnSuffix     string
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
		doc, err := m.seriesDoc()
		if err != nil {
			m.titleErr = err
			return
		}

		// the <h1> is the site brand ("包子漫畫"), not the series title; the
		// real one lives in the .comics-detail__title element
		m.title = sanitizeTitle(doc.Find(".comics-detail__title").Text())
		if m.title == "" {
			// don't cache an empty title: root.go calls FetchTitle again per
			// goroutine (each chapter's PackSingle), and a cached empty string
			// would silently produce " 123 - ..." filenames
			m.titleErr = errors.New("no series title found (selector drift?)")
		}
	})
	return m.title, m.titleErr
}

// seriesDoc fetches and parses the series page once, then hands the same
// document to FetchTitle and FetchChapters instead of each fetching the page.
func (m *Baozimh) seriesDoc() (*goquery.Document, error) {
	m.docOnce.Do(func() {
		body, err := http.Get(http.RequestParams{
			URL: m.URL,
		})
		if err != nil {
			m.docErr = err
			return
		}
		defer body.Close()

		m.doc, m.docErr = goquery.NewDocumentFromReader(body)
	})
	return m.doc, m.docErr
}

// FetchChapters returns the chapters of the manga
func (m *Baozimh) FetchChapters() (chapters Filterables, errs []error) {
	doc, err := m.seriesDoc()
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
				// len(pages)+1 matches the 1-based Number the next page would get,
				// rather than the 0-based DOM index (which drifts after a skip)
				color.Yellow("page %d of %s has no URL to fetch from 😕 (will be ignored)", len(pages)+1, f.GetTitle())
				return
			}
			img = m.resolveBzcdnHost(img)
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

// bzcdnBareHostRe matches an unresolved baozimh image CDN host such as
// "s2.bzcdn.net" - the literal hostname the reader HTML embeds in every
// <amp-img src>. See bzcdnRegionSuffixes for why that host alone is dead.
var bzcdnBareHostRe = regexp.MustCompile(`^(s\d+)\.bzcdn\.net$`)

// bzcdnRegionSuffixes are live regional CDN edges for baozimh images,
// e.g. "s2" + "-rsa1-usla.bzcdn.net" = "s2-rsa1-usla.bzcdn.net". The
// bare "sN.bzcdn.net" host embedded in reader HTML is never meant to be
// fetched directly: the site's own reader JS (choose_cdn() in
// page_runtime_v5.js) races several regional edges behind it on page load
// and rewrites every <amp-img src> to whichever answers first, entirely
// client-side. A scraper that never runs that JS is left with the bare
// host, which (as of #182) resolves to a single origin that outright
// refuses every connection - not a dead mirror to retry, a placeholder
// that was never meant to be requested as-is.
//
// This list was extracted by running the site's own obfuscated JS
// (REQ_DOMAINS in page_runtime_v5.js, one entry deobfuscated via a decode
// function it exports) rather than guessed, and is ordered the same way:
// the three edges the site itself weights highest (and independently
// confirmed live - '-rsa1-usla' first, since that's the one seen actually
// serving images in a live browser) before its lower-priority Hong Kong
// edge. If baozimh rotates its edges again, re-probe with PROBE_NETLOG=1
// or by fetching page_runtime_v5.js fresh and decoding REQ_DOMAINS the
// same way.
var bzcdnRegionSuffixes = []string{
	"-rsa1-usla.bzcdn.net",
	"-ogsm1-uspho.bzcdn.net",
	"-mha1-nlams.bzcdn.net",
	"-dpa1-cnhk.bzcdn.net",
}

// bzcdnProbeClient is dedicated to resolveBzcdnHost's candidate checks. It
// carries its own short timeout so a candidate that hangs (rather than
// failing fast like a refused connection) can't stall a chapter download;
// the shared http package's client has no timeout, since normal page/image
// downloads may legitimately be slow.
var bzcdnProbeClient = &nethttp.Client{Timeout: 5 * time.Second}

// bzcdnProbe reports whether url answers with 200 OK. A var (not a plain
// function) so tests can substitute a fake without touching the network.
var bzcdnProbe = func(url string) bool {
	resp, err := bzcdnProbeClient.Head(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == nethttp.StatusOK
}

// resolveBzcdnHost rewrites a bare "sN.bzcdn.net" image URL (see
// bzcdnBareHostRe) to a live regional edge, HEAD-probing
// bzcdnRegionSuffixes in order and caching the first one that answers for
// the rest of this Baozimh instance's lifetime - mirroring the site's own
// choose_cdn(), which resolves once per page load and reuses the result
// for every image on it, not once per image. Returns raw unchanged if it
// isn't a bare bzcdn host (already regional, or a different domain
// entirely), or if every candidate fails - the caller's normal
// fetch/retry path then surfaces whatever error the dead original host
// produces, exactly like before this fix existed.
func (m *Baozimh) resolveBzcdnHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	match := bzcdnBareHostRe.FindStringSubmatch(u.Hostname())
	if match == nil {
		return raw
	}
	prefix := match[1]

	m.cdnSuffixOnce.Do(func() {
		for _, suffix := range bzcdnRegionSuffixes {
			candidate := *u
			candidate.Host = prefix + suffix
			if bzcdnProbe(candidate.String()) {
				m.cdnSuffix = suffix
				return
			}
		}
	})

	if m.cdnSuffix == "" {
		return raw
	}
	u.Host = prefix + m.cdnSuffix
	return u.String()
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
