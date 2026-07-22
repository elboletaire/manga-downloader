// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// PlainHTML is a grabber for any plain HTML page (with no ajax pagination whatsoever)
type PlainHTML struct {
	*Grabber
	doc  *goquery.Document
	rows *goquery.Selection
	site SiteSelector
}

func NewPlainHTML(g *Grabber) *PlainHTML {
	return &PlainHTML{Grabber: g}
}

type SiteSelector struct {
	Title        string
	Rows         string
	Link         string
	Chapter      string
	ChapterTitle string
	Image        string
}

// PlainHTMLChapter represents a PlainHTML Chapter
type PlainHTMLChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a valid grabber URL
func (m *PlainHTML) Test() (bool, error) {
	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return false, err
	}
	m.doc, err = goquery.NewDocumentFromReader(body)
	if err != nil {
		return false, err
	}

	// order is important, since some sites have very similar selectors
	selectors := []SiteSelector{
		// tcbonepiecechapters.com (former tcbscans.com)
		{
			Title:        "h1",
			Rows:         "main .mx-auto .grid .col-span-2 a",
			Chapter:      ".font-bold",
			ChapterTitle: ".text-gray-500",
			Image:        "picture img",
		},
		// asurascans.com (former asuratoon.com)
		{
			Title:        "h1",
			Rows:         `a[data-astro-prefetch][href*="/chapter/"]`,
			Chapter:      "span.font-medium",
			ChapterTitle: "span.font-medium",
			Image:        "img[data-page-index]",
		},
		// sanascans.com: same Astro-based reader template family as
		// asurascans.com, but chapter URLs use "/chapter-N" (hyphen) instead
		// of "/chapter/N" (slash), so the two selectors don't collide. The
		// series page has several decorative <h1> stat labels (Status, Type,
		// Chapters, Last update) alongside the real title, so a plain "h1"
		// selector would concatenate all of them - scope it to the one with
		// itemprop="name". Recent chapters are often coin-locked (paid early
		// access) and render with zero page images (no error, just an empty
		// chapter) - pick an older/free chapter when testing.
		{
			Title:        `h1[itemprop="name"]`,
			Rows:         `a[data-astro-prefetch][href*="/chapter-"]`,
			Chapter:      "span.font-medium",
			ChapterTitle: "span.font-medium",
			Image:        "img[data-reader-page-image]",
		},
		// zonatmo.org (TuMangaOnline, former zonatmo.com)
		{
			Title:        "h1.element-title",
			Rows:         "li.upload-link",
			Chapter:      ".chapter-number",
			ChapterTitle: ".chapter-number",
			Link:         ".chapter-detail a.btn-primary",
			Image:        "img.reader-image",
		},
		// mangapill.com: each chapter row is a plain <a> with the chapter
		// number as its own text (no dedicated child selector), hence the
		// empty Chapter/ChapterTitle (see FetchChapters)
		{
			Title: "h1",
			Rows:  "#chapters [data-filter-list] a",
			Image: "img.js-page",
		},
		// demonicscans.org: reader page embeds the real (non-lazy) image
		// src directly, no JS variable needed
		{
			Title: "h1",
			Rows:  "#chapters-list li",
			Link:  "a",
			Image: "img.imgholder",
		},
		// mangakatana.com: reader page images never land in the HTML as real
		// <img src>/data-src (they stay as data-src="#" placeholders), they
		// are only assigned client-side from an obfuscated JS array (see
		// getPlainHTMLImageURL), so this Image selector is mostly unused
		{
			Title:        "h1.heading",
			Rows:         ".chapters tr",
			Chapter:      ".chapter a",
			ChapterTitle: ".chapter a",
			Link:         ".chapter a",
			Image:        "#imgs img",
		},
		// rawkuma.net: raw (Japanese) manga, Kiru WordPress theme. The reader
		// page looks JS-rendered (images load via a "chapter" JS module) but
		// the page URLs are already plain <img src='...'> tags (single-quoted
		// attrs, still fine for goquery) inside a `[data-image-data]` section
		// in the initial HTML - no browser needed. Plain "h1" is ambiguous: an
		// info-sidebar "Last Updates" label is also an <h1> earlier in the
		// DOM, so the title selector must be scoped to the real series title.
		{
			Title:        `h1[itemprop="name"]`,
			Rows:         "#chapter-list [data-chapter-number]",
			Chapter:      "span",
			ChapterTitle: "span",
			Link:         "a",
			Image:        "[data-image-data] img",
		},
		// dynasty-scans.com: chapters are <a class="name"> rows inside a
		// <dl class="chapter-list">; volume headers are sibling <dt>s so a
		// plain descendant selector already skips them. The row is the link
		// itself (empty Chapter/ChapterTitle/Link, like mangapill above).
		// Reader pages only place the first page's <img> in the DOM; the
		// rest come from a `var pages = [...]` JSON blob handled in
		// getPlainHTMLImageURL, so Image here is just a fallback.
		{
			Title: ".tag-title b",
			Rows:  "dl.chapter-list dd a.name",
			Image: "#reader img",
		},
		// mangaread.org & manhuaplus.com: plain-HTTP Madara wordpress themes
		// (not cloudflare-walled, unlike the toongod/dragontea/manhuaus group
		// in plainhtmlbrowser.go). The full chapter list ships in the series
		// page HTML (no ajax pagination) and reader pages are single-page.
		{
			Title:        "h1",
			Rows:         "li.wp-manga-chapter",
			Chapter:      "a",
			ChapterTitle: "a",
			Link:         "a",
			Image:        "div.reading-content img",
		},
		// silentquill.net (Armageddon Scanlation): mangastream/themesia
		// theme, same markup as sushiscan (PlainHTMLBrowser) but reachable
		// with plain HTTP, no cloudflare challenge. Reader pages embed all
		// pages in the ts_reader javascript call, already handled generically.
		// lagoonscans.com (Themesia's "MangaReader" WP theme, same publisher
		// as sushiscan's ts_reader-based theme): reader images come from a
		// ts_reader.run(...) blob, already handled by getPlainHTMLImageURL.
		{
			Title: "h1",
		},
		// rokaricomics.com: mangastream/themesia theme (same markup as the
		// cloudflare-gated sushiscan.net in plainhtmlbrowser.go), but this
		// domain answers plain HTTP with no challenge. Reader images come
		// from the embedded ts_reader javascript call. Note: the most recent
		// chapter(s) of a series may be locked behind a coin paywall (no
		// images in the HTML); test with an older, unlocked chapter.
		// violetscans.org: mangastream/themesia theme (same markup as
		// sushiscan.net, see plainhtmlbrowser.go) but reachable over plain
		// HTTP, no cloudflare challenge. Reader pages come from the embedded
		// ts_reader javascript call. Some recent chapters are locked behind
		// an in-site coin paywall (no href, just a modal trigger); those
		// list with an unusable URL since there is nothing to fetch for them.
		{
			Title: "h1.entry-title",
		},
		// witchscans.com: mangastream/themesia WordPress theme, reader
		// images come from the ts_reader.run blob already handled by
		// getPlainHTMLImageURL, so Image is just a fallback here
		{
			Title:        "h1",
			Rows:         "#chapterlist li",
			Chapter:      ".chapternum",
			ChapterTitle: ".chapternum",
			Link:         "a",
			Image:        "#readerarea img",
		},
		// hivetoons.org (VoidScans/HiveToons): the series page also embeds a
		// "recently added" widget and a "continue reading" link that reuse
		// the same /series/{slug}/chapter-{n} href shape, so the row
		// selector pins the `.p-3` class that's unique to the real,
		// full (non-paginated) chapter-list anchors to avoid duplicates.
		{
			Title:        `h1[itemprop="name"]`,
			Rows:         `a.p-3[href*="/chapter-"]`,
			Chapter:      "h3",
			ChapterTitle: "h3",
			Image:        "img[data-reader-page-image]",
		},
		// templetoons.com (Temple Scan): Next.js RSC-streamed page, but
		// server-rendered, so plain HTTP already gets the full chapter list
		// (unlike mangak.io, no need to reach for the __NEXT_DATA__ blob).
		// Each row is itself the <a> (Link empty), the chapter number lives
		// in a nested h1. Reader pages embed the page list as a flat JSON
		// "pages" array handled by getPlainHTMLImageURL, so Image is unused
		// but kept as a fallback.
		{
			Title:        "h1.text-3xl",
			Rows:         "div.grid.grid-cols-6 > a",
			Chapter:      "h1",
			ChapterTitle: "h1",
			Image:        "img",
		},
		// furyosociety.com: French scanlation site on the FoOlSlide engine.
		// Each chapter is listed twice (a desktop <div> and a mobile <a>);
		// only the mobile <a> is itself the reader link, so scoping Rows to
		// it avoids duplicate chapters. The reader page renders every page
		// image server-side (no ajax pagination), so a plain selector works.
		{
			Title:        "h1.fs-comic-title",
			Rows:         ".fs-chapter-list a.element.mobile",
			Chapter:      ".title-grp .title",
			ChapterTitle: ".name",
			Image:        ".fs-reader-page-container img",
		},
		// reader.deathtollscans.net: FoOlSlide reader. The reader page only
		// renders the current page's <img class="open">, but embeds every
		// page's URL in a `var pages = [...]` JSON blob (see
		// getPlainHTMLImageURL), so the Image selector below is just a
		// fallback/documentation of what a single rendered page looks like.
		{
			Title:        "h1.title",
			Rows:         ".list .element",
			Chapter:      ".title a",
			ChapterTitle: ".title a",
			Link:         ".title a",
			Image:        "img.open",
		},
		// elftoon.com: mangastream/themesia theme (same family as
		// sushiscan.net, see plainhtmlbrowser.go), reachable over plain HTTP
		// with no cloudflare. Some recent chapters are gem/coin-locked behind
		// a modal (href="#"), so Rows only matches rows whose overlay link is
		// a real URL; the reader page's images come from the embedded
		// ts_reader javascript call, already handled generically.
		{
			Title:        "h1",
			Rows:         `#chapterlist li:has(a.chapter-link-overlay[href^="http"])`,
			Chapter:      ".chapternum",
			ChapterTitle: ".chapternum",
			Link:         "a.chapter-link-overlay",
			Image:        "#readerarea img",
		},
		// asmotoon.com (Asmodeus Scans): chapter rows are plain <a> children
		// of #chapters. The reader lazy-loads pages, so most <img> keep a
		// shared placeholder src until scrolled into view; the real per-page
		// id always lives in the img's uid attribute (see
		// getPlainHTMLImageURL), so src/data-src are only used as a fallback.
		{
			Title:        "h1",
			Rows:         "#chapters > a",
			Chapter:      ".text-sm.truncate",
			ChapterTitle: ".text-sm.truncate",
			Image:        "img.myImage",
		},
		// madarascans.com/.org: mangareader/themesia theme; the reader page
		// already carries all its images in a ts_reader.run(...) blob (see
		// getPlainHTMLImageURL) so the Image selector is only a fallback.
		// Recent chapters are paywalled ("locked" class, no ts_reader blob in
		// the plain HTML) and simply won't parse any pages; only free
		// chapters are downloadable.
		{
			Title:        "h1",
			Rows:         ".ch-item",
			Chapter:      ".ch-num",
			ChapterTitle: ".ch-num",
			Link:         "a.ch-main-anchor",
			Image:        "#readerarea img",
		},
		// ritharscans.com: chapter pages aren't in <img> tags at all, they're
		// parsed out of an Alpine.js `immersiveReader(...)` blob (see
		// getPlainHTMLImageURL), so Image is unused but set for consistency
		// writerscans.com: each chapter row is a plain <a> inside #chapters;
		// reader page images are lazy-loaded (a placeholder `src` and a `uid`
		// attribute), see the getPlainHTMLImageURL template-literal fallback
		// mistscans.com (former Arven Scans members): each chapter row is the
		// <a> itself (no dedicated Link), the number lives in a child span
		// (see FetchChapters/Chapter), and reader images are lazy-loaded
		// client-side from a `uid` attribute (see getPlainHTMLImageURL)
		{
			Title:        "h1",
			Rows:         "#chapters a",
			Chapter:      ".text-sm.truncate",
			ChapterTitle: ".text-sm.truncate",
			Image:        "img",
		},
		// en-thunderscans.com: mangastream/themesia theme (reader page uses
		// ts_reader.run, already handled generically). Some recent chapters
		// are coin-locked premium content whose row has no href (just a
		// data-bs-toggle modal trigger); the `a[href]` filter on Rows keeps
		// those out of the chapter list entirely instead of yielding bogus
		// URLs. Title uses a specific class because the page has several
		// unrelated bare <h1> tags (Type, Status, Released...) whose text
		// would otherwise get concatenated in.
		{
			Title:        "h1.entry-title",
			Rows:         "#chapterlist li[data-num] a[href]",
			Chapter:      ".chapternum",
			ChapterTitle: ".chapternum",
			Image:        ".readercontent img",
		},
		// writerscans.com & mistscans.com (meowing.org platform): each chapter
		// row is a plain <a> inside #chapters; reader page images are lazy-loaded
		// (a placeholder `src` and a `uid` attribute), see the
		// getPlainHTMLImageURL template-literal fallback. Image is a union so it
		// matches both sites' reader markup.
		{
			Title:        "h1",
			Rows:         "#chapters a",
			Chapter:      ".text-sm.truncate",
			ChapterTitle: ".text-sm.truncate",
			Image:        "img.myImage, #pages img",
		},
	}

	// for the same priority reasons, we need to iterate over the selectors
	// using a simple `,` joining all selectors would return missmatches
	for _, selector := range selectors {
		rows := m.doc.Find(selector.Rows)
		if rows.Length() > 0 {
			m.rows = rows
			m.site = selector
			break
		}
	}

	if m.rows == nil {
		return false, nil
	}

	return m.rows.Length() > 0, nil
}

// Ttitle returns the manga title
func (m PlainHTML) FetchTitle() (string, error) {
	title := m.doc.Find(m.site.Title)

	return sanitizeTitle(title.Text()), nil
}

// chapterNumberRe matches a chapter number in a chapter title, accepting
// "Chapter 10", "Ch. 10", "C. 10", the Spanish "Capítulo 10" and the French
// "Chapitre 10" (case insensitive).
var chapterNumberRe = regexp.MustCompile(`(?i)\b(?:chapter|chapitre|cap[ií]tulo|ch|c)\.?\s*(\d+\.?\d*)`)

// volumeNumberRe matches a volume number ("Volume 18", "Vol. 2", the Spanish
// "Volumen 3"), used as a fallback for sites that list volumes instead of
// chapters (e.g. sushiscan).
var volumeNumberRe = regexp.MustCompile(`(?i)\bvol(?:ume|umen)?\.?\s*(\d+\.?\d*)`)

// parseChapterNumber extracts the chapter number from a chapter title, falling
// back to a volume number. Returns false if no number could be found (these
// are usually site announcements rather than actual chapters).
func parseChapterNumber(text string) (float64, bool) {
	match := chapterNumberRe.FindStringSubmatch(text)
	if len(match) == 0 {
		// only checked as fallback so "Vol.2 Chapter 15" still prefers the chapter
		match = volumeNumberRe.FindStringSubmatch(text)
	}
	if len(match) == 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// FetchChapters returns a slice of chapters
func (m PlainHTML) FetchChapters() (chapters Filterables, errs []error) {
	m.rows.Each(func(i int, s *goquery.Selection) {
		// an empty Chapter/ChapterTitle selector means the row itself carries
		// the text (i.e. mangapill, where each chapter row is a plain <a>
		// with no dedicated child element for the chapter number)
		chapterText := s.Text()
		if m.site.Chapter != "" {
			chapterText = s.Find(m.site.Chapter).Text()
		}
		number, ok := parseChapterNumber(chapterText)
		if !ok {
			return
		}

		u := s.AttrOr("href", "")
		if m.site.Link != "" {
			u = s.Find(m.site.Link).AttrOr("href", "")
		}
		u = m.resolveURL(u)
		title := chapterText
		if m.site.ChapterTitle != "" {
			title = s.Find(m.site.ChapterTitle).Text()
		}
		title = sanitizeTitle(title)
		chapter := &PlainHTMLChapter{
			Chapter{
				Number: number,
				Title:  title,
			},
			u,
		}

		chapters = append(chapters, chapter)
	})

	return
}

// resolveURL turns a possibly-relative href into an absolute URL, resolved
// against the series page URL (m.URL). This correctly handles both
// root-relative hrefs ("/manga/x/chapter-1", the common case, equivalent to
// the old BaseUrl()-prefixing behaviour) and directory-relative hrefs with no
// leading slash (e.g. templetoons.com, whose rows link to "slug/chapter-1"
// relative to the series page's own directory).
func (m PlainHTML) resolveURL(href string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	base, err := url.Parse(m.URL)
	if err != nil {
		return m.BaseUrl() + href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return m.BaseUrl() + href
	}
	return base.ResolveReference(ref).String()
}

// FetchChapter fetches a chapter and its pages
func (m PlainHTML) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*PlainHTMLChapter)
	body, err := http.Get(http.RequestParams{
		URL: mchap.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	return m.chapterFromDoc(f, doc), nil
}

// chapterFromDoc builds a Chapter from an already parsed reader page
func (m PlainHTML) chapterFromDoc(f Filterable, doc *goquery.Document) *Chapter {
	pimages := getPlainHTMLImageURL(m.site.Image, doc)
	pcount := len(pimages)

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(pcount),
		Language:   "en",
	}

	for i, img := range pimages {
		if img == "" {
			// this error is not critical and is not from our side, so just log it out
			color.Yellow("page %d of %s has no URL to fetch from 😕 (will be ignored)", i, chapter.GetTitle())
			continue
		}
		if !strings.HasPrefix(img, "http") {
			img = m.BaseUrl() + img
		}

		page := Page{
			Number: int64(i),
			URL:    img,
		}
		chapter.Pages = append(chapter.Pages, page)
	}

	return chapter
}

func getPlainHTMLImageURL(selector string, doc *goquery.Document) []string {
	// Check for chapImages JavaScript variable first

	html, _ := doc.Html()
	re := regexp.MustCompile(`var chapImages = '([^']+)'`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		// Found chapImages variable, split by comma
		return strings.Split(matches[1], ",")
	}
	// some sites store a plain text array with the urls into a hidden layer
	pimages := doc.Find("#arraydata")
	if pimages.Length() == 1 {
		return strings.Split(pimages.Text(), ",")
	}

	// mangastream/themesia based readers (e.g. sushiscan) embed all the pages
	// of the chapter in a ts_reader javascript call
	re = regexp.MustCompile(`(?s)ts_reader\.run\(.*?"images":\s*\[(.*?)\]`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		urls := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(matches[1], -1)
		imgs := []string{}
		for _, u := range urls {
			imgs = append(imgs, strings.ReplaceAll(u[1], `\/`, `/`))
		}
		return imgs
	}

	// mangakatana.com assigns each page's real URL from an obfuscated JS
	// array (variable name changes, e.g. `thzq`) into data-src once the page
	// loads: `obj.attr('data-src', thzq[i])`; the <img> tags themselves only
	// ever contain a data-src="#" placeholder in the raw HTML. Find the
	// array's name from that assignment, then pull its literal contents.
	re = regexp.MustCompile(`\.attr\('data-src',\s*(\w+)\[i\]\)`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		arrayRe := regexp.MustCompile(`var\s+` + regexp.QuoteMeta(matches[1]) + `\s*=\s*\[(.*?)\];`)
		arrayMatches := arrayRe.FindStringSubmatch(html)
		if len(arrayMatches) > 1 {
			urls := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(arrayMatches[1], -1)
			imgs := []string{}
			for _, u := range urls {
				imgs = append(imgs, u[1])
			}
			return imgs
		}
	}

	// FoOlSlide readers (e.g. deathtollscans.net) embed the full ordered page
	// list as a proper JSON array in a `var pages = [...]` variable, one
	// object per page with a "url" field; only the current page is actually
	// rendered as an <img> in the HTML. Anchoring on the following
	// `var next_chapter` avoids truncating at a stray "]" inside a string.
	re = regexp.MustCompile(`(?s)var pages\s*=\s*(\[.+?\]);\s*var next_chapter`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		type foolslidePage struct {
			URL string `json:"url"`
		}
		var fpages []foolslidePage
		if err := json.Unmarshal([]byte(matches[1]), &fpages); err == nil {
			imgs := []string{}
			for _, p := range fpages {
				imgs = append(imgs, p.URL)
			}
			return imgs
		}
	}

	// dynasty-scans.com (Dynasty Reader) embeds the full, ordered page list
	// as a JSON array in a `var pages = [...]` blob; only the first page's
	// <img> actually exists in the DOM, the rest are swapped in by JS on
	// navigation, so the Image selector alone would only find one page.
	re = regexp.MustCompile(`(?s)var pages\s*=\s*(\[.*?\]);`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		var pages []struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal([]byte(matches[1]), &pages); err == nil {
			imgs := []string{}
			for _, p := range pages {
				imgs = append(imgs, p.Image)
			}
			return imgs
		}
	}

	// templetoons.com's reader (Next.js RSC streaming) embeds the page list
	// as a flat JSON array inside a double-escaped JS string, i.e. the raw
	// HTML literally contains `\"pages\":[\"https://...\",...]`. Extract it
	// and unescape the `\"` delimiters.
	re = regexp.MustCompile(`\\"pages\\":\[([^\]]+)\]`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		urls := regexp.MustCompile(`"([^"\\]+)\\?"`).FindAllStringSubmatch(matches[1], -1)
		imgs := []string{}
		for _, u := range urls {
			imgs = append(imgs, u[1])
		}
		return imgs
	}

	// ritharscans.com's Alpine.js reader stores every page's relative path
	// (plus a `baseLink` to prefix them with) inline in an `x-data` attribute:
	// x-data="immersiveReader({ pages: [{"path":"...","width":...},...], baseLink: 'https://.../storage/', ... })"
	reader := doc.Find(`[x-data*="immersiveReader("]`)
	if reader.Length() > 0 {
		xdata := reader.First().AttrOr("x-data", "")
		base := ""
		if m := regexp.MustCompile(`baseLink:\s*'([^']*)'`).FindStringSubmatch(xdata); len(m) > 1 {
			base = m[1]
		}
		paths := regexp.MustCompile(`"path":"([^"]+)"`).FindAllStringSubmatch(xdata, -1)
		imgs := []string{}
		for _, p := range paths {
			imgs = append(imgs, base+strings.ReplaceAll(p[1], `\/`, "/"))
		}
		if len(imgs) > 0 {
			return imgs
		}
	}

	// some sites (e.g. writerscans.com) lazy-load reader images: the <img>
	// only carries a placeholder `src` plus a `uid` attribute, and an inline
	// script builds the real URL from a template literal like
	// `https://cdn.example.com/uploads/${uid}`. If we find that pattern,
	// build each image's URL from its `uid` instead of reading src/data-src.
	re = regexp.MustCompile("`(https?://[^`]+?)\\$\\{uid\\}`")
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		prefix := matches[1]
		pimages := doc.Find(selector)
		imgs := []string{}
		pimages.Each(func(i int, s *goquery.Selection) {
			uid := s.AttrOr("uid", "")
			imgs = append(imgs, prefix+uid)
		})
		return imgs
	}

	// images are inside picture objects
	pimages = doc.Find(selector)

	imgs := []string{}
	pimages.Each(func(i int, s *goquery.Selection) {
		// asmotoon.com: pages lazy-load via vanilla-lazyload, so src stays a
		// shared placeholder until the browser scrolls the image into view.
		// The real per-page identifier is always present in the uid
		// attribute, so prefer reconstructing the CDN URL from it.
		if uid := s.AttrOr("uid", ""); uid != "" {
			imgs = append(imgs, "https://cdn.meowing.org/uploads/"+uid)
			return
		}
		src := s.AttrOr("src", "")
		if src == "" || strings.HasPrefix(src, "data:image") {
			src = s.AttrOr("data-src", "")
		}
		imgs = append(imgs, strings.Trim(src, " \n\r\t")) // trim whitespaces
	})

	return imgs
}

// sanitizeTitle sanitizes titles, trimming and removing extra spaces from titles
func sanitizeTitle(title string) string {
	spaces := regexp.MustCompile(`\s+`)
	title = spaces.ReplaceAllString(title, " ")
	title = strings.TrimSpace(title)

	return title
}
