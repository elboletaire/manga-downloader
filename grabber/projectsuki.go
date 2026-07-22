// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Projectsuki is a grabber for projectsuki.com. The series page is plain,
// server-rendered HTML with the full chapter list already embedded (no
// pagination/AJAX needed), but the reader is a single-page-per-request app
// (`/read/{bookId}/{chapterId}/{page}`) whose images are only linked one at
// a time. Requesting a page number past the last one 302-redirects to the
// real last page, which is (ab)used to discover a chapter's page count with
// a single request; the remaining page URLs are then built directly, since
// every image filename follows a fixed "00"+pageNumber pattern (e.g. page 1
// is ".../001", page 17 is ".../0017") that was confirmed to hold across
// series/chapters.
type Projectsuki struct {
	*Grabber
	title string
}

func NewProjectsuki(g *Grabber) *Projectsuki {
	return &Projectsuki{Grabber: g}
}

// ProjectsukiChapter represents a Projectsuki Chapter
type ProjectsukiChapter struct {
	Chapter
	BookID    int
	ChapterID int
}

// projectsukiReadPathRe matches the book and chapter ids out of a reader
// link, i.e. "/read/159270/12910/1" -> book 159270, chapter 12910
var projectsukiReadPathRe = regexp.MustCompile(`^/read/(\d+)/(\d+)/`)

// projectsukiChapterNumRe extracts the chapter number out of a row's link
// text, i.e. "Chapter 233 - Some Title" -> 233
var projectsukiChapterNumRe = regexp.MustCompile(`Chapter\s+([\d.]+)`)

// projectsukiImageRe extracts the book id, image hash and page number out of
// a page image URL, i.e.
// "https://projectsuki.com/images/gallery/159270/5b1dd2d9.../0016?123" ->
// book 159270, hash "5b1dd2d9...", page 16
var projectsukiImageRe = regexp.MustCompile(`/images/gallery/(\d+)/([0-9a-f]+)/00(\d+)`)

// Test returns true if the URL is a projectsuki.com URL
func (p *Projectsuki) Test() (bool, error) {
	re := regexp.MustCompile(`projectsuki\.com`)
	return re.MatchString(p.URL), nil
}

// FetchTitle fetches and returns the manga title
func (p *Projectsuki) FetchTitle() (string, error) {
	if p.title != "" {
		return p.title, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: p.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	p.title = sanitizeTitle(doc.Find(`h2[itemprop="title"]`).First().Text())

	return p.title, nil
}

// FetchChapters returns the chapters of the manga
func (p Projectsuki) FetchChapters() (Filterables, []error) {
	bookID, err := p.bookID()
	if err != nil {
		return nil, []error{err}
	}

	body, err := http.Get(http.RequestParams{
		URL: p.URL,
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}

	var chapters Filterables
	var errs []error

	doc.Find("table.table-sm tbody tr.row").Each(func(_ int, row *goquery.Selection) {
		link := row.Find(`a[href^="/read/"]`).First()
		href, ok := link.Attr("href")
		if !ok {
			return
		}

		matches := projectsukiReadPathRe.FindStringSubmatch(href)
		if len(matches) != 3 {
			return
		}
		chapterID, err := strconv.Atoi(matches[2])
		if err != nil {
			errs = append(errs, err)
			return
		}

		// collapse the link's text (spread over several lines/&nbsp;s) into
		// a single, normalized string, i.e. "Chapter 233 - Some Title"
		text := strings.Join(strings.Fields(strings.ReplaceAll(link.Text(), " ", " ")), " ")

		numMatches := projectsukiChapterNumRe.FindStringSubmatch(text)
		if numMatches == nil {
			errs = append(errs, fmt.Errorf("could not parse chapter number from %q", text))
			return
		}
		number, err := strconv.ParseFloat(numMatches[1], 64)
		if err != nil {
			errs = append(errs, err)
			return
		}

		chapters = append(chapters, &ProjectsukiChapter{
			Chapter: Chapter{
				Number: number,
				Title:  text,
			},
			BookID:    bookID,
			ChapterID: chapterID,
		})
	})

	return chapters, errs
}

// FetchChapter fetches a chapter and its pages
func (p Projectsuki) FetchChapter(f Filterable) (*Chapter, error) {
	pchap := f.(*ProjectsukiChapter)

	// requesting a page number past the chapter's last page redirects to the
	// real last page, which lets us learn the page count (and image hash)
	// with a single request instead of paging through every image.
	uri := fmt.Sprintf("%s/read/%d/%d/999999", p.BaseUrl(), pchap.BookID, pchap.ChapterID)
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: p.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	src, ok := doc.Find(".strip-reader img").First().Attr("src")
	if !ok {
		return nil, fmt.Errorf("could not find the reader image for chapter %d", pchap.ChapterID)
	}

	matches := projectsukiImageRe.FindStringSubmatch(src)
	if len(matches) != 4 {
		return nil, fmt.Errorf("could not parse the reader image url %q", src)
	}
	hash := matches[2]
	lastPage, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(lastPage),
		Language:   "en",
	}

	for i := 1; i <= lastPage; i++ {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i),
			URL:    fmt.Sprintf("%s/images/gallery/%d/%s/00%d", p.BaseUrl(), pchap.BookID, hash, i),
		})
	}

	return chapter, nil
}

// bookID returns the book id from the URL (i.e. 159270 for
// https://projectsuki.com/book/159270)
func (p Projectsuki) bookID() (int, error) {
	re := regexp.MustCompile(`/book/(\d+)`)
	matches := re.FindStringSubmatch(p.URL)
	if len(matches) != 2 {
		return 0, fmt.Errorf("could not find book id in url %s", p.URL)
	}
	return strconv.Atoi(matches[1])
}
