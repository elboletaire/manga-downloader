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

	pimages := body.Find("div.reading-content img")
	pcount := pimages.Length()
	// progress := make(chan int, pcount)

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(pcount),
		Language:   "en",
	}
	pages := []Page{}
	pimages.Each(func(i int, s *goquery.Selection) {
		u := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", "")))
		n := i + 1
		if u == "" {
			// this error is not critical and is not from our side, so just log it out
			color.Yellow("page %d of %s has no URL to fetch from 😕 (will be ignored)", n, chapter.GetTitle())
			return
		}
		if !strings.HasPrefix(u, "http") {
			u = t.BaseUrl() + u
		}
		// progress <- n
		pages = append(pages, Page{
			Number: int64(n),
			URL:    u,
		})
	})

	chapter.Pages = pages

	// close(progress)

	return chapter, nil
}
