package grabber

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// Tcb is a grabber for tcbscans.com (and possibly other wordpress sites)
type Tcb struct {
	*Grabber
	chaps *goquery.Selection
	title string
}

func NewTcb(g *Grabber) *Tcb {
	return &Tcb{Grabber: g}
}

// TcbChapter is a chapter for TCBScans
type TcbChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a compatible TCBScans URL
func (t *Tcb) Test() (bool, error) {
	re := regexp.MustCompile(`manga\/(.*)\/$`)
	if !re.MatchString(t.URL) {
		return false, nil
	}

	mid := re.FindStringSubmatch(t.URL)[1]
	uri, _ := url.JoinPath(t.BaseUrl(), "manga", mid, "ajax", "chapters")

	rbody, err := http.Post(http.RequestParams{
		URL:     uri,
		Referer: t.BaseUrl(),
	})
	if err != nil {
		return false, err
	}

	body, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return false, err
	}

	t.chaps = body.Find("li")

	return t.chaps.Length() > 0, nil
}

// GetTitle fetches and returns the manga title
func (t *Tcb) FetchTitle() (string, error) {
	if t.title != "" {
		return t.title, nil
	}

	rbody, err := http.Get(http.RequestParams{
		URL: t.URL,
	})
	if err != nil {
		return "", err
	}
	defer rbody.Close()
	body, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		return "", err
	}

	t.title = strings.TrimSpace(body.Find("h1").Text())

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
		re := regexp.MustCompile(`(\d+\.?\d*)`)
		ns := re.FindString(title)
		num, err := strconv.ParseFloat(ns, 64)
		if err != nil {
			errs = append(errs, err)
		}
		chapter := &TcbChapter{
			Chapter{
				Title:  title,
				Number: num,
			},
			s.Find("a").AttrOr("href", ""),
		}

		chapters = append(chapters, chapter)
	})

	return
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
			color.Yellow("error fetching page %d: %s", pageNum+1, err.Error())
			continue
		}

		pageDoc, err := goquery.NewDocumentFromReader(rbody)
		rbody.Close()
		if err != nil {
			color.Yellow("error parsing page %d: %s", pageNum+1, err.Error())
			continue
		}

		// Collect all images in this page
		pageDoc.Find("div.reading-content img").Each(func(i int, s *goquery.Selection) {
			u := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", "")))
			if u == "" {
				color.Yellow("page %d of %s has an image with no URL to fetch from", pageNum+1, f.GetTitle())
				return
			}
			if !strings.HasPrefix(u, "http") {
				u = t.BaseUrl() + u
			}
			pages = append(pages, Page{
				Number: int64(pageNum + 1),
				URL:    u,
			})
		})
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)), // Use actual number of found pages
		Language:   "en",
		Pages:      pages,
	}

	return chapter, nil
}
