package grabber

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/exp/slog"
)

// TCB is a grabber for tcbscans.com (and possibly other wordpress sites)
type TCB struct {
	*Grabber
	chapters *goquery.Selection
	title    string
}

// TCBChapter is a chapter for TCBScans
type TCBChapter struct {
	Chapter
	URL string
}

// ValidateURL returns true if the URL is a compatible TCBScans URL
func (t *TCB) ValidateURL() (bool, error) {
	re := regexp.MustCompile(`manga/(.*)/$`)
	if !re.MatchString(t.URL) {
		return false, nil
	}

	mid := re.FindStringSubmatch(t.URL)[1]
	uri, err := url.Parse(t.BaseURL())
	if err != nil {
		return false, err
	}

	uri = uri.JoinPath("manga", mid, "ajax", "chapters")

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, uri.String(), http.NoBody)
	if err != nil {
		return false, err
	}
	request.Header.Add("Referer", t.BaseURL())

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()

	body, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return false, err
	}

	t.chapters = body.Find("li")

	return t.chapters.Length() > 0, nil
}

// FetchTitle fetches and returns the manga title
func (t *TCB) FetchTitle() (string, error) {
	if t.title != "" {
		return t.title, nil
	}

	uri, err := url.Parse(t.URL)
	if err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, uri.String(), http.NoBody)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	body, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	t.title = strings.TrimSpace(body.Find("h1").Text())

	return t.title, nil
}

// FetchChapters returns a slice of chapters
func (t TCB) FetchChapters() (chapters Filterables, err error) {
	t.chapters.Each(func(i int, s *goquery.Selection) {
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
			slog.Error(err.Error())
			return
		}
		chapter := &TCBChapter{
			Chapter{
				Title:  title,
				Number: num,
			},
			s.Find("a").AttrOr("href", ""),
		}

		chapters = append(chapters, chapter)
	})

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (t TCB) FetchChapter(f Filterable) (*Chapter, error) {
	tchap, _ := f.(*TCBChapter)

	uri, err := url.Parse(tchap.URL)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, uri.String(), http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	pageImages := body.Find("div.reading-content img")

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(pageImages.Length()),
		Language:   "en",
	}
	pages := []Page{}
	pageImages.Each(func(i int, s *goquery.Selection) {
		imageURL := strings.TrimSpace(s.AttrOr("data-src", s.AttrOr("src", "")))
		pageNumber := int64(i + 1)
		if imageURL == "" {
			// this error is not critical and is not from our side, so just log it out
			slog.Warn(fmt.Sprintf("page %d of %q has no URL to fetch from (will be ignored)", pageNumber, chapter.GetTitle()))
			return
		}
		if !strings.HasPrefix(imageURL, "http") {
			imageURL, err = url.JoinPath(t.BaseURL(), imageURL)
			if err != nil {
				slog.Warn(fmt.Sprintf("page %d of %q has an invalid URL to fetch from (will be ignored)", pageNumber, chapter.GetTitle()))
				return
			}
		}
		pages = append(pages, Page{
			Number: pageNumber,
			URL:    imageURL,
		})
	})

	chapter.Pages = pages

	return chapter, nil
}
