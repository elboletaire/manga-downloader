// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Blogger is a grabber for sites published on Blogger, matched by markup so
// clones work too. A series is the blog's search for its title (posts carry no
// label per series), and a pasted chapter post resolves to it through its title.
// ZeistManga themes are the exception: their posts are listed under the series'
// label instead (see zeistLabels).
type Blogger struct {
	*Grabber
	// doc is the page Test fetched, kept for resolving a chapter post.
	doc *goquery.Document
	// language is the page's <html lang>, see pageLanguage.
	language string
	// query is the series search, as typed or read off the post title.
	query string
	// zeist is set for a ZeistManga theme, see zeistFingerprint.
	zeist    bool
	title    string
	chapters Filterables
	listed   bool
}

// NewBlogger creates a new Blogger grabber.
func NewBlogger(g *Grabber) *Blogger {
	return &Blogger{Grabber: g}
}

// BloggerChapter is a chapter post of a Blogger series.
type BloggerChapter struct {
	Chapter
	URL string
	// feed is the post's content in the label feed (ZeistManga only), for a
	// reader that fills its pages in by script.
	feed string
}

// bloggerPostRe matches the default permalink of a post: /YYYY/MM/slug.html.
var bloggerPostRe = regexp.MustCompile(`^/\d{4}/\d{2}/[^/]+\.html$`)

// bloggerFingerprint matches the widget bundle every Blogger page loads.
const bloggerFingerprint = `link[href*="blogger.com/static/v1/widgets/"], script[src*="blogger.com/static/v1/widgets/"]`

// bloggerURLShape reports whether rawURL is a series search (returning its
// query) or a chapter post, the two shapes the grabber handles. A chapter post
// returns an empty query, as its series is only known from its title.
func bloggerURLShape(rawURL string) (query string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if u.Path == "/search" && u.Query().Get("q") != "" {
		return u.Query().Get("q"), true
	}
	return "", bloggerPostRe.MatchString(u.Path)
}

// Test returns true if the URL is a series search or a chapter post of a
// Blogger site. The URL's shape picks the page to fetch; the markup decides
// whether the site is Blogger.
func (b *Blogger) Test() (bool, error) {
	query, ok := bloggerURLShape(b.URL)
	if !ok {
		return false, nil
	}

	body, err := b.identifyPage(b.URL)
	if err != nil {
		return false, nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return false, err
	}
	if doc.Find(bloggerFingerprint).Length() == 0 {
		return false, nil
	}

	b.doc = doc
	b.language = pageLanguage(doc)
	b.query = query
	b.zeist = zeistFingerprint.MatchString(body) || doc.Find(zeistListSelector).Length() > 0

	return true, nil
}

// resolveSeriesURL derives the series search from a chapter post's title, using
// the page Test already fetched. A series search is a no-op.
func (b *Blogger) resolveSeriesURL() error {
	if b.query != "" {
		return nil
	}
	if b.doc == nil {
		return errSeriesNotFound(b.URL)
	}

	if post, ok := bloggerPostTitle(b.doc); ok {
		b.query = post.Series
		return nil
	}
	// a title without a number is a series post, searched under its own name
	if name := bloggerSeriesPostTitle(b.doc); name != "" {
		b.query = name
		return nil
	}

	return errSeriesNotFound(b.URL)
}

// bloggerSeriesPostTitle returns the title of a post that names a series
// instead of a chapter (no number in it), or "" when it has none.
func bloggerSeriesPostTitle(doc *goquery.Document) string {
	name := stripBlogName(doc.Find(`meta[property="og:title"]`).AttrOr("content", ""))
	name = bloggerLeerRe.ReplaceAllString(name, "")
	name = stripKindWord(bloggerBracketRe.ReplaceAllString(name, " "))
	if name == "" || bloggerNumberRe.MatchString(name) {
		return ""
	}
	return name
}

// bloggerPostTitle reads a chapter post's title from the first place that
// parses: the post heading (h3, or h1/h2 in templates without it), then the
// og:title and <title>, which may carry a " - Blog Name" suffix.
func bloggerPostTitle(doc *goquery.Document) (bloggerPost, bool) {
	candidates := []string{
		doc.Find("h3.post-title").First().Text(),
		doc.Find("h1.post-title, h2.post-title").First().Text(),
		stripBlogName(doc.Find(`meta[property="og:title"]`).AttrOr("content", "")),
	}
	doc.Find("title").Each(func(_ int, s *goquery.Selection) {
		candidates = append(candidates, stripBlogName(s.Text()))
	})

	for _, c := range candidates {
		if post, ok := parseBloggerTitle(c); ok {
			return post, true
		}
	}
	return bloggerPost{}, false
}

// list fetches the series' posts once: from the label feeds of a ZeistManga
// theme when one of its labels has chapter posts, else from the blog's search feed.
func (b *Blogger) list() error {
	if b.listed {
		return nil
	}
	if b.zeist && b.listLabels() {
		b.listed = true
		return nil
	}
	if err := b.resolveSeriesURL(); err != nil {
		return err
	}

	feed, err := b.readFeed("/feeds/posts/summary", url.Values{"q": {b.query}})
	if err != nil {
		return err
	}
	var entries []bloggerEntry
	for _, e := range feed {
		entries = append(entries, bloggerEntry{Title: e.Title.Value, URL: e.alternateURL()})
	}

	b.chapters, b.title = bloggerChapters(entries, b.query, b.language)
	if b.title == "" {
		b.title = b.query
	}
	b.listed = true

	return nil
}

// readFeed reads every post of the JSON feed at path, with the given query.
func (b *Blogger) readFeed(path string, query url.Values) ([]bloggerFeedEntry, error) {
	var entries []bloggerFeedEntry
	for start := 1; ; {
		feed, err := b.fetchFeed(path, query, start)
		if err != nil {
			return nil, err
		}

		total, _ := strconv.Atoi(feed.Feed.Total.Value)
		entries = append(entries, feed.Feed.Entries...)

		start += len(feed.Feed.Entries)
		if len(feed.Feed.Entries) == 0 || start > total {
			break
		}
	}
	return entries, nil
}

// bloggerFeed is the part of a Blogger JSON feed the grabber reads.
type bloggerFeed struct {
	Feed struct {
		Total struct {
			Value string `json:"$t"`
		} `json:"openSearch$totalResults"`
		Entries []bloggerFeedEntry `json:"entry"`
	} `json:"feed"`
}

// bloggerFeedEntry is one post in a Blogger JSON feed. Only the full feeds
// carry its content.
type bloggerFeedEntry struct {
	Title struct {
		Value string `json:"$t"`
	} `json:"title"`
	Content struct {
		Value string `json:"$t"`
	} `json:"content"`
	Links []struct {
		Rel  string `json:"rel"`
		Href string `json:"href"`
	} `json:"link"`
}

// alternateURL returns the post's own URL.
func (e bloggerFeedEntry) alternateURL() string {
	for _, l := range e.Links {
		if l.Rel == "alternate" {
			return l.Href
		}
	}
	return ""
}

// fetchFeed fetches one page of the JSON feed at path, 150 posts at a time (the
// most Blogger serves per request), starting at start-index.
func (b *Blogger) fetchFeed(path string, query url.Values, start int) (bloggerFeed, error) {
	var feed bloggerFeed

	query.Set("alt", "json")
	query.Set("max-results", "150")
	query.Set("start-index", strconv.Itoa(start))
	feedURL := b.BaseUrl() + path + "?" + query.Encode()

	rbody, err := http.Get(http.RequestParams{
		URL:     feedURL,
		Referer: b.BaseUrl() + "/",
	})
	if err != nil {
		return feed, err
	}
	defer rbody.Close()

	err = json.NewDecoder(rbody).Decode(&feed)
	return feed, err
}

// bloggerEntry is a post of the series search feed.
type bloggerEntry struct {
	Title string
	URL   string
}

// bloggerChapters keeps the posts whose titles name the series, numbered by
// their titles, and returns them with the series name as written in them.
func bloggerChapters(entries []bloggerEntry, query, lang string) (Filterables, string) {
	key := bloggerKey(query)
	var title string
	chapters := Filterables{}

	for _, e := range entries {
		post, ok := parseBloggerTitle(e.Title)
		if !ok || bloggerKey(post.Series) != key {
			continue
		}
		if title == "" {
			title = post.Series
		}

		chapters = append(chapters, &BloggerChapter{
			Chapter: Chapter{
				Number:   post.Number,
				Language: lang,
				Label:    Label{Style: LabelNumbered, Part: post.Part},
			},
			URL: e.URL,
		})
	}

	return chapters, title
}

// zeistListSelector is the chapter list of a ZeistManga-like template that names
// its label in data-labelchapter, without the theme header.
const zeistListSelector = "div.chapter_get[data-labelchapter]"

// zeistRunRe and zeistLabelRe match the label a ZeistManga script lists a
// series' posts by: its widget call ("nPL.run('Name')") or a label variable.
var (
	zeistRunRe   = regexp.MustCompile(`\b(?:nPL2?|clwd)\.run\(\s*['"]([^'"]+)['"]`)
	zeistLabelRe = regexp.MustCompile(`\blabel\s*=\s*['"]([^'"]+)['"]`)
	// zeistFingerprint matches the header comment of the ZeistManga theme, and of
	// its AniManga fork, which lists its chapters the same way.
	zeistFingerprint = regexp.MustCompile(`Name\s*:\s*(?:ZeistManga|AniManga)\b`)
)

// zeistLabels returns the labels a ZeistManga page may list its series' posts
// by, most specific first: the search query, the chapter list's data-label, the
// labels its scripts run with, and the series post's title.
func zeistLabels(doc *goquery.Document, query string) []string {
	var labels []string
	add := func(label string) {
		label = strings.TrimSpace(label)
		if label != "" && !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}

	add(query)
	add(doc.Find("#nextPrevJS[data-label], #nextPrevJSbottom[data-label]").First().AttrOr("data-label", ""))
	add(doc.Find(zeistListSelector).First().AttrOr("data-labelchapter", ""))
	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		for _, re := range []*regexp.Regexp{zeistRunRe, zeistLabelRe} {
			for _, m := range re.FindAllStringSubmatch(s.Text(), -1) {
				add(m[1])
			}
		}
	})
	// the series post's title, before the blog's name ("Juujika no Rokunin - Manga Ai-land")
	if head, _, _ := strings.Cut(doc.Find(`meta[property="og:title"]`).AttrOr("content", ""), " - "); !bloggerNumberRe.MatchString(head) {
		add(head)
	}
	add(bloggerSeriesPostTitle(doc))

	return labels
}

// listLabels lists a ZeistManga series from the first of its labels with chapter
// posts, naming it by that label. It reports false otherwise, or when a feed
// fails, so the search feed gets its turn.
func (b *Blogger) listLabels() bool {
	for _, label := range zeistLabels(b.doc, b.query) {
		entries, err := b.readFeed("/feeds/posts/default/-/"+url.PathEscape(label), url.Values{})
		if err != nil {
			continue
		}
		if chapters := zeistChapters(entries, label, b.language); len(chapters) > 0 {
			b.chapters, b.title = chapters, label
			return true
		}
	}
	return false
}

// zeistChapters numbers a label's posts by their titles, which often name no
// series ("Cap 362", "Chapter 02"), skipping the series' own post. Each post keeps
// its feed content for the pages its reader fills in by script.
func zeistChapters(entries []bloggerFeedEntry, label, lang string) Filterables {
	key := bloggerKey(label)
	chapters := Filterables{}

	for _, e := range entries {
		if bloggerKey(e.Title.Value) == key {
			continue
		}

		chapter := &BloggerChapter{
			Chapter: Chapter{Language: lang, Label: Label{Style: LabelUnnumbered}},
			URL:     e.alternateURL(),
			feed:    e.Content.Value,
		}
		// a post with no number ("Epílogo", "Oneshot") is listed by its title
		if post, ok := readBloggerTitle(e.Title.Value); ok {
			chapter.Number = post.Number
			chapter.Label = Label{Style: LabelNumbered, Part: post.Part}
		} else {
			chapter.Title = strings.TrimSpace(e.Title.Value)
		}
		chapters = append(chapters, chapter)
	}

	return chapters
}

// bloggerPost is what a post title says: the series, the chapter number and
// the part of a chapter split over several posts ("Parte 2"), if any.
type bloggerPost struct {
	Series string
	Number float64
	Part   string
}

var (
	bloggerBracketRe = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)`)
	bloggerPartRe    = regexp.MustCompile(`(?i)\b(?:parte|part)\s+(\d+)\b`)
	bloggerNumberRe  = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
	// the "Leer " (read) some posts prefix to their title
	bloggerLeerRe = regexp.MustCompile(`(?i)^\s*leer\s+`)
	// the "| Manga" (or ": Webcomic", ", Chapter") between a series and its number,
	// possibly repeated ("Dragon Ball Super Manga Chapter").
	bloggerKindRe = regexp.MustCompile(`(?i)(?:\s*[|:,]?\s*\b(?:manga|webcomic|manhwa|cap[ií]tulo|chapter)\b)*\s*[|:,]?\s*$`)
	// the " - Blog Name" suffix of a page title, as the last separated part
	bloggerSuffixRe = regexp.MustCompile(`\s+[|–-]\s+([^|–-]*)$`)
)

// stripBlogName drops a trailing " - Blog Name" from a page title. A part with a
// number is kept, as it's a title and not a blog name ("Kenichi 2 | Manga 27").
func stripBlogName(title string) string {
	m := bloggerSuffixRe.FindStringSubmatchIndex(title)
	if m == nil || bloggerNumberRe.MatchString(title[m[2]:m[3]]) {
		return title
	}
	return title[:m[0]]
}

// stripKindWord trims a series name and drops the kind word after it ("One
// Piece Manga" is "One Piece"), shared with WordPressCategory's category names.
func stripKindWord(name string) string {
	return strings.TrimSpace(bloggerKindRe.ReplaceAllString(strings.TrimSpace(name), ""))
}

// parseBloggerTitle reads "Blue Lock Manga 364 Parte 2 [Español]" or "Kenichi 2
// | Manga 27". The chapter number is the last one, so "Kenichi 2" stays a name.
func parseBloggerTitle(title string) (bloggerPost, bool) {
	post, ok := readBloggerTitle(title)
	return post, ok && post.Series != ""
}

// readBloggerTitle is parseBloggerTitle without the series name: a title that
// is only a number ("Chapter 02") still gives its number, with an empty Series.
func readBloggerTitle(title string) (bloggerPost, bool) {
	var post bloggerPost

	title = bloggerLeerRe.ReplaceAllString(title, "")
	title = bloggerBracketRe.ReplaceAllString(title, " ")
	if m := bloggerPartRe.FindStringSubmatch(title); m != nil {
		post.Part = m[1]
		title = bloggerPartRe.ReplaceAllString(title, " ")
	}

	locs := bloggerNumberRe.FindAllStringIndex(title, -1)
	if len(locs) == 0 {
		return post, false
	}
	last := locs[len(locs)-1]
	num, err := strconv.ParseFloat(title[last[0]:last[1]], 64)
	if err != nil {
		return post, false
	}

	post.Number = num
	post.Series = stripKindWord(title[:last[0]])

	return post, true
}

// bloggerAccents folds the Spanish accents, so "Módulo" and "Modulo" match.
var bloggerAccents = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// bloggerKey normalises a series name for comparison: lowercase, accents
// folded, and punctuation and spacing reduced to single spaces.
func bloggerKey(name string) string {
	name = bloggerAccents.Replace(strings.ToLower(name))
	return strings.Join(strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

// bloggerPages returns a chapter post's pages: Blogger's own decorations carry an
// alt text, the uploaded pages alt="". Script-rendered templates keep the body in
// a hidden textarea or ZeistManga's #reader, and some wrap it in .check-box.
func bloggerPages(doc *goquery.Document, postURL *url.URL) []Page {
	if pages := bloggerImages(doc.Find("div.post-body img[alt='']"), postURL); len(pages) > 0 {
		return pages
	}

	raw := doc.Find("textarea#zeist-raw-data").First().Text()
	if body, err := goquery.NewDocumentFromReader(strings.NewReader(raw)); err == nil {
		if pages := bloggerImages(body.Find("img[alt='']"), postURL); len(pages) > 0 {
			return pages
		}
	}

	// the reader's own icons are SVG, never pages
	if pages := bloggerImages(doc.Find("#reader img:not([src*='.svg'])"), postURL); len(pages) > 0 {
		return pages
	}
	return bloggerImages(doc.Find(".check-box img[alt='']"), postURL)
}

// bloggerFeedPages returns the images in a post's feed content, for a reader
// whose pages the page doesn't carry.
func bloggerFeedPages(content string, postURL *url.URL) []Page {
	body, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return nil
	}
	return bloggerImages(body.Find("img"), postURL)
}

// bloggerImages turns the src of each image into a numbered page.
func bloggerImages(imgs *goquery.Selection, postURL *url.URL) []Page {
	var pages []Page

	imgs.Each(func(_ int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		u, err := postURL.Parse(src)
		if err != nil {
			return
		}
		pages = append(pages, Page{
			Number: int64(len(pages) + 1),
			URL:    u.String(),
		})
	})

	return pages
}

// FetchTitle returns the series name.
func (b *Blogger) FetchTitle() (string, error) {
	if err := b.list(); err != nil {
		return "", err
	}
	return b.title, nil
}

// FetchChapters returns the series' chapter posts.
func (b *Blogger) FetchChapters() (Filterables, []error) {
	if err := b.list(); err != nil {
		return nil, []error{err}
	}
	return b.chapters, nil
}

// FetchChapter fetches a chapter post and returns its pages.
func (b *Blogger) FetchChapter(f Filterable) (*Chapter, error) {
	bchap := f.(*BloggerChapter)

	rbody, err := http.Get(http.RequestParams{
		URL:     bchap.URL,
		Referer: b.BaseUrl() + "/",
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()

	doc, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return nil, err
	}

	postURL, err := url.Parse(bchap.URL)
	if err != nil {
		return nil, err
	}

	pages := bloggerPages(doc, postURL)
	if len(pages) == 0 && bchap.feed != "" {
		pages = bloggerFeedPages(bchap.feed, postURL)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(bchap))
	}

	chapter := bchap.Chapter
	chapter.Pages = pages
	chapter.PagesCount = int64(len(pages))

	return &chapter, nil
}
