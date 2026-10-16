// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Blogger is a grabber for sites published on Blogger, matched by markup so
// clones work too. A series is the blog's search for its title (posts carry no
// label per series), and a pasted chapter post resolves to it through its title.
type Blogger struct {
	*Grabber
	// doc is the page Test fetched, kept for resolving a chapter post.
	doc *goquery.Document
	// language is the page's <html lang>, see pageLanguage.
	language string
	// query is the series search, as typed or read off the post title.
	query    string
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

	rbody, err := http.Get(http.RequestParams{
		URL:          b.URL,
		DisableRetry: true, // identification probe: a failure means "not this site"
	})
	if err != nil {
		return false, err
	}
	defer rbody.Close()

	doc, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return false, err
	}
	if doc.Find(bloggerFingerprint).Length() == 0 {
		return false, nil
	}

	b.doc = doc
	b.language = pageLanguage(doc)
	b.query = query

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

// list fetches the series' posts from the blog's search feed, once.
func (b *Blogger) list() error {
	if b.listed {
		return nil
	}
	if err := b.resolveSeriesURL(); err != nil {
		return err
	}

	var entries []bloggerEntry
	for start := 1; ; {
		feed, err := b.fetchFeed(start)
		if err != nil {
			return err
		}

		total, _ := strconv.Atoi(feed.Feed.Total.Value)
		for _, e := range feed.Feed.Entries {
			entries = append(entries, bloggerEntry{Title: e.Title.Value, URL: e.alternateURL()})
		}

		start += len(feed.Feed.Entries)
		if len(feed.Feed.Entries) == 0 || start > total {
			break
		}
	}

	b.chapters, b.title = bloggerChapters(entries, b.query, b.language)
	if b.title == "" {
		b.title = b.query
	}
	b.listed = true

	return nil
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

// bloggerFeedEntry is one post in a Blogger JSON feed.
type bloggerFeedEntry struct {
	Title struct {
		Value string `json:"$t"`
	} `json:"title"`
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

// fetchFeed fetches one page of the series search feed, 150 posts at a time
// (the most Blogger serves per request), starting at start-index.
func (b *Blogger) fetchFeed(start int) (bloggerFeed, error) {
	var feed bloggerFeed

	feedURL := b.BaseUrl() + "/feeds/posts/summary?" + url.Values{
		"alt":         {"json"},
		"q":           {b.query},
		"max-results": {"150"},
		"start-index": {strconv.Itoa(start)},
	}.Encode()

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
			Chapter{
				Number:   post.Number,
				Language: lang,
				Label:    Label{Style: LabelNumbered, Part: post.Part},
			},
			e.URL,
		})
	}

	return chapters, title
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
	if post.Series == "" {
		return post, false
	}

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
// a hidden textarea.
func bloggerPages(doc *goquery.Document, postURL *url.URL) []Page {
	if pages := bloggerImages(doc.Find("div.post-body img[alt='']"), postURL); len(pages) > 0 {
		return pages
	}

	raw := doc.Find("textarea#zeist-raw-data").First().Text()
	body, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return nil
	}
	return bloggerImages(body.Find("img[alt='']"), postURL)
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
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(bchap))
	}

	chapter := bchap.Chapter
	chapter.Pages = pages
	chapter.PagesCount = int64(len(pages))

	return &chapter, nil
}
