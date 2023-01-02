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

type Tcb struct {
	Grabber
	chaps *goquery.Selection
	title string
}

// Test returns true if the URL is a valid TCBScans URL
func (t *Tcb) Test() bool {
	re := regexp.MustCompile(`manga\/(.*)\/$`)
	if !re.MatchString(t.URL) {
		return false
	}

	mid := re.FindStringSubmatch(t.URL)[1]
	uri, _ := url.JoinPath(t.GetBaseUrl(), "manga", mid, "ajax", "chapters")

	rbody, err := http.Post(http.RequestParams{
		URL:     uri,
		Referer: t.GetBaseUrl(),
	})
	if err != nil {
		return false
	}

	body, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		panic(err)
	}

	t.chaps = body.Find("li")

	return t.chaps.Length() > 0
}

func (t *Tcb) GetTitle(language string) string {
	if t.title != "" {
		return t.title
	}

	rbody, err := http.Get(http.RequestParams{
		URL: t.URL,
	})
	if err != nil {
		panic(err)
	}
	defer rbody.Close()
	body, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		panic(err)
	}
	title := body.Find("h1").Text()

	t.title = strings.TrimSpace(title)

	return t.title
}

func (t Tcb) FetchChapters(language string) Filterables {
	chapters := Filterables{}
	t.chaps.Each(func(i int, s *goquery.Selection) {
		title := strings.TrimSpace(s.Find("a").Text())
		re := regexp.MustCompile(`(\d+\.?\d*)`)
		ns := re.FindString(title)
		num, err := strconv.ParseFloat(ns, 64)
		if err != nil {
			panic(err)
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

	return chapters
}

func (t Tcb) FetchChapter(f Filterable) Chapter {
	tchap := f.(*TcbChapter)

	rbody, err := http.Get(http.RequestParams{
		URL: tchap.URL,
	})
	if err != nil {
		panic(err)
	}
	defer rbody.Close()
	body, err := goquery.NewDocumentFromReader(rbody)
	if err != nil {
		panic(err)
	}

	pimages := body.Find("div.reading-content img")

	chapter := Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(pimages.Length()),
		Language:   "en",
	}
	var pages []Page
	pimages.Each(func(i int, s *goquery.Selection) {
		u := strings.TrimSpace(s.AttrOr("data-src", ""))
		n := int64(i + 1)
		if u == "" {
			color.Red("page %d has no URL to fetch from 😕 (will be ignored)", n)
			return
		}
		if !strings.HasPrefix(u, "http") {
			u = t.GetBaseUrl() + u
		}
		pages = append(pages, Page{
			Number: n,
			URL:    u,
		})
	})

	chapter.Pages = pages

	return chapter
}

type TcbChapter struct {
	Chapter
	URL string
}
