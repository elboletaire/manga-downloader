// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// WordPressCategory is a grabber for WordPress sites that publish each chapter
// as a post in one category (animeallstar30.com). It matches by markup, not
// domain, and reads the whole category from the REST API, not the archive pages.
type WordPressCategory struct {
	*Grabber
	categoryID int
	// language is the page's <html lang>, see pageLanguage.
	language string
	title    string
	chapters Filterables
	listed   bool
}

// NewWordPressCategory returns a new WordPressCategory grabber.
func NewWordPressCategory(g *Grabber) *WordPressCategory {
	return &WordPressCategory{Grabber: g}
}

// WordPressCategoryChapter is a post of a WordPressCategory site.
type WordPressCategoryChapter struct {
	Chapter
	URL string
}

// wpPost is the part of a WordPress REST post the grabber reads.
type wpPost struct {
	Link  string `json:"link"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
}

// wpPerPage is the REST page size, the largest WordPress allows.
const wpPerPage = 100

// wpCategoryLinkRe and wpPostLinkRe match the REST links WordPress puts in
// the head of a category archive and of a single post.
var (
	wpCategoryLinkRe = regexp.MustCompile(`/wp-json/wp/v2/categories/(\d+)/?$`)
	wpPostLinkRe     = regexp.MustCompile(`/wp-json/wp/v2/posts/(\d+)/?$`)
)

// Test returns true if the URL is a WordPress category archive, or a post in
// one, and remembers the category its chapters come from. The page is
// fetched without retries so an unrelated site fails fast.
func (w *WordPressCategory) Test() (bool, error) {
	rbody, err := http.Get(http.RequestParams{
		URL:          w.URL,
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		return false, err
	}
	defer rbody.Close()

	doc, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return false, err
	}
	w.language = pageLanguage(doc)

	id, err := w.categoryFromPage(doc)
	if err != nil || id == 0 {
		return false, err
	}
	w.categoryID = id

	return true, nil
}

// categoryFromPage returns the category whose chapters a page belongs to, or
// 0 when it's neither a category archive nor a post. A post doesn't name its
// category in the markup, so its REST resource gives the first one.
func (w *WordPressCategory) categoryFromPage(doc *goquery.Document) (int, error) {
	var categoryID, postID int
	doc.Find("link[href]").Each(func(_ int, s *goquery.Selection) {
		href := s.AttrOr("href", "")
		if m := wpCategoryLinkRe.FindStringSubmatch(href); m != nil {
			categoryID, _ = strconv.Atoi(m[1])
		}
		if m := wpPostLinkRe.FindStringSubmatch(href); m != nil {
			postID, _ = strconv.Atoi(m[1])
		}
	})

	if categoryID != 0 && doc.Find("body").HasClass("category") {
		return categoryID, nil
	}
	if postID == 0 {
		return 0, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:          w.apiURL(fmt.Sprintf("posts/%d?_fields=categories", postID)),
		Referer:      w.URL,
		DisableRetry: true,
	})
	// a REST route that doesn't answer (404) or a body that isn't JSON means another site
	var statusErr *http.StatusError
	if errors.As(err, &statusErr) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	var post struct {
		Categories []int `json:"categories"`
	}
	if err := json.Unmarshal([]byte(body), &post); err != nil {
		return 0, nil
	}
	if len(post.Categories) == 0 {
		return 0, nil
	}

	return post.Categories[0], nil
}

// apiURL returns the WordPress REST URL for path (i.e. "posts?categories=3") on
// the site's own host.
func (w *WordPressCategory) apiURL(path string) string {
	return w.BaseUrl() + "/wp-json/wp/v2/" + path
}

// FetchTitle returns the series title, see list.
func (w *WordPressCategory) FetchTitle() (string, error) {
	if err := w.list(); err != nil {
		return "", err
	}
	return w.title, nil
}

// FetchChapters returns the category's chapters, see list.
func (w *WordPressCategory) FetchChapters() (Filterables, []error) {
	if err := w.list(); err != nil {
		return nil, []error{err}
	}
	return w.chapters, nil
}

// list reads the category's posts once, and sets the chapters and the series
// title from them.
func (w *WordPressCategory) list() error {
	if w.listed {
		return nil
	}

	posts, err := w.fetchPosts()
	if err != nil {
		return err
	}
	w.chapters = wpChapters(posts, w.language)

	if w.title, err = w.seriesTitle(posts); err != nil {
		return err
	}
	w.listed = true

	return nil
}

// fetchPosts pages through the category's posts in the REST API until a page
// comes back short.
func (w *WordPressCategory) fetchPosts() ([]wpPost, error) {
	var posts []wpPost
	for page := 1; ; page++ {
		body, err := http.GetText(http.RequestParams{
			URL: w.apiURL(fmt.Sprintf(
				"posts?categories=%d&per_page=%d&_fields=link,title&page=%d",
				w.categoryID, wpPerPage, page,
			)),
			Referer: w.URL,
		})
		if err != nil {
			// WordPress answers 400 for a page past the last one, which is
			// reached when the total is an exact multiple of wpPerPage
			var statusErr *http.StatusError
			if page > 1 && errors.As(err, &statusErr) && statusErr.Code == 400 {
				break
			}
			return nil, err
		}

		var pagePosts []wpPost
		if err := json.Unmarshal([]byte(body), &pagePosts); err != nil {
			return nil, err
		}
		posts = append(posts, pagePosts...)

		if len(pagePosts) < wpPerPage {
			break
		}
	}

	return posts, nil
}

// seriesTitle names the series after its posts when most of the numbered ones
// agree (categories like "Chapters" say nothing), and after the category otherwise.
func (w *WordPressCategory) seriesTitle(posts []wpPost) (string, error) {
	if name, ok := wpPostsSeries(posts); ok {
		return name, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:     w.apiURL(fmt.Sprintf("categories/%d?_fields=name", w.categoryID)),
		Referer: w.URL,
	})
	if err != nil {
		return "", err
	}

	var category struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(body), &category); err != nil {
		return "", err
	}

	// the kind word goes like Blogger's ("One Piece Manga" is "One Piece")
	return stripKindWord(sanitizeTitle(html.UnescapeString(category.Name))), nil
}

// wpPostsSeries returns the series that more than half of the numbered posts
// name in their titles ("Jujutsu Kaisen" in "Jujutsu Kaisen, Chapter 260"), as
// written in them.
func wpPostsSeries(posts []wpPost) (string, bool) {
	counts := map[string]int{}
	spellings := map[string]map[string]int{}
	numbered := 0
	for _, p := range posts {
		post, ok := parseBloggerTitle(sanitizeTitle(html.UnescapeString(p.Title.Rendered)))
		if !ok {
			continue
		}
		numbered++
		key := bloggerKey(post.Series)
		counts[key]++
		if spellings[key] == nil {
			spellings[key] = map[string]int{}
		}
		spellings[key][post.Series]++
	}

	for key, n := range counts {
		if 2*n > numbered {
			return wpCommonSpelling(spellings[key]), true
		}
	}
	return "", false
}

// wpCommonSpelling returns the spelling most posts use, a tie going to the
// smaller one so the result doesn't depend on map order.
func wpCommonSpelling(counts map[string]int) string {
	var best string
	for s, n := range counts {
		if n > counts[best] || (n == counts[best] && s < best) {
			best = s
		}
	}
	return best
}

// wpTitleNumberRe matches the last number of a title, for posts that name the
// chapter by its bare number ("One Piece Manga 1196 Español").
var wpTitleNumberRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\D*$`)

// wpChapters turns the category's posts into chapters, skipping posts without
// a chapter number (site announcements). A part of a split chapter is named
// after its part, as Blogger names them.
func wpChapters(posts []wpPost, lang string) Filterables {
	var chapters Filterables
	for _, p := range posts {
		title := sanitizeTitle(html.UnescapeString(p.Title.Rendered))
		num, part, ok := wpChapterNumber(title)
		if !ok {
			continue
		}

		// a part keeps its number in the title, which a label can't add after it
		chapterTitle := ""
		if part != "" {
			chapterTitle = NumberLabel(num) + " Part " + part
		}
		chapters = append(chapters, &WordPressCategoryChapter{
			Chapter: Chapter{
				// the post title repeats the series name, which the filename already has
				Title:      chapterTitle,
				Number:     num,
				Language:   lang,
				LabelStyle: LabelNumbered,
			},
			URL: p.Link,
		})
	}

	return chapters
}

// wpChapterNumber reads a post title's chapter number and part. It tries the
// series-and-number shape first, as "Gachiakuta 172 parte 2" needs it, then a
// bare chapter number, then the last number.
func wpChapterNumber(title string) (num float64, part string, ok bool) {
	if post, ok := parseBloggerTitle(title); ok {
		return post.Number, post.Part, true
	}
	if num, ok := parseChapterNumber(title); ok {
		return num, "", true
	}

	m := wpTitleNumberRe.FindStringSubmatch(title)
	if m == nil {
		return 0, "", false
	}
	num, _ = strconv.ParseFloat(m[1], 64)
	return num, "", true
}

// FetchChapter fetches a post's pages.
func (w *WordPressCategory) FetchChapter(f Filterable) (*Chapter, error) {
	wchap := f.(*WordPressCategoryChapter)

	rbody, err := http.Get(http.RequestParams{
		URL:     wchap.URL,
		Referer: w.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()

	doc, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return nil, err
	}

	pages := wpPageImages(doc)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}

	return &Chapter{
		Title:      f.GetTitle(),
		LabelStyle: labelStyleOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   w.language,
		Pages:      pages,
	}, nil
}

// wpBvURLRe reads the real image URL out of a Bv lazy-loader placeholder.
var wpBvURLRe = regexp.MustCompile(`bv-img-url="([^"]+)"`)

// wpPageImages returns the post body's images in document order, once each.
// Lazy-loaded images hold a placeholder data: URI in src and the real image
// elsewhere, see wpLazyURL.
func wpPageImages(doc *goquery.Document) []Page {
	var pages []Page
	seen := map[string]bool{}
	doc.Find(".entry-content img").Each(func(_ int, s *goquery.Selection) {
		if wpHiddenImage(s) {
			return
		}
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if strings.HasPrefix(src, "data:") {
			src = wpLazyURL(s)
		}
		if src == "" || strings.HasPrefix(src, "data:") || seen[src] {
			return
		}
		seen[src] = true
		pages = append(pages, Page{
			Number: int64(len(pages) + 1),
			URL:    src,
		})
	})

	return pages
}

// wpHiddenImage reports whether an image is sized to nothing (a cover or a
// tracker the theme hides), which a reader never shows as a page.
func wpHiddenImage(s *goquery.Selection) bool {
	for _, attr := range []string{"width", "height"} {
		if strings.TrimSuffix(strings.TrimSpace(s.AttrOr(attr, "")), "px") == "0" {
			return true
		}
	}
	return false
}

// wpLazyURL returns the real image behind a placeholder: a lazy-load attribute
// if the theme sets one, else the bv-img-url the Bv loader keeps inside its
// base64 SVG placeholder (the placeholder's src is the only copy of it).
func wpLazyURL(s *goquery.Selection) string {
	for _, attr := range []string{"data-src", "data-lazy-src", "bv-img-url", "data-bv-img-url"} {
		if v := strings.TrimSpace(s.AttrOr(attr, "")); v != "" {
			return v
		}
	}

	payload, ok := strings.CutPrefix(strings.TrimSpace(s.AttrOr("src", "")), "data:image/svg+xml;base64,")
	if !ok {
		return ""
	}
	svg, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	if m := wpBvURLRe.FindSubmatch(svg); m != nil {
		return string(m[1])
	}
	return ""
}
