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

// Mgeko is a grabber for mgeko.cc. The series page only lists the ~50 most
// recent chapters (a scrollable preview); the full list lives on a separate
// "<series>/all-chapters/" page linked from a "Load All Chapters" button, so
// FetchChapters always fetches that page instead of the series URL directly.
// Chapter rows carry no "Chapter"/"Ch." keyword mgeko's own text is just
// "198-eng-li" or, for half chapters, "200.5-eng-li" hence the custom
// number parsing instead of the shared chapterNumberRe used by PlainHTML.
type Mgeko struct {
	*Grabber
	title string
}

func NewMgeko(g *Grabber) *Mgeko {
	return &Mgeko{Grabber: g}
}

// MgekoChapter represents a Mgeko chapter
type MgekoChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a mgeko.cc URL
func (m *Mgeko) Test() (bool, error) {
	re := regexp.MustCompile(`mgeko\.(cc|com)`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mgeko) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	m.title = sanitizeTitle(doc.Find(`h1[itemprop="name"]`).Text())

	return m.title, nil
}

// mgekoChapterNumberRe matches the leading chapter number in mgeko's chapter
// slugs/titles (i.e. "198-eng-li" -> 198, "200.5-eng-li" -> 200.5). Rows with
// no leading number ("announcement-eng-li", "side-story-1-eng-li") are site
// announcements/extras rather than real chapters, and are skipped.
var mgekoChapterNumberRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)`)

// parseMgekoChapterNumber extracts the chapter number from a chapter row's
// text
func parseMgekoChapterNumber(text string) (float64, bool) {
	match := mgekoChapterNumberRe.FindStringSubmatch(strings.TrimSpace(text))
	if len(match) == 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// formatChapterNumber formats a chapter number dropping a trailing ".0"
func formatChapterNumber(number float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", number), ".0", "", 1)
}

// FetchChapters returns the chapters of the manga. The series page only
// lists the most recent ~50 chapters, so the full list is fetched from the
// site's own "all-chapters" page instead.
func (m Mgeko) FetchChapters() (chapters Filterables, errs []error) {
	uri := strings.TrimRight(m.URL, "/") + "/all-chapters/"
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: m.URL,
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}

	doc.Find(".chapter-list li a").Each(func(i int, s *goquery.Selection) {
		text := s.Find(".chapter-title").Text()
		number, ok := parseMgekoChapterNumber(text)
		if !ok {
			return
		}

		u := s.AttrOr("href", "")
		if !strings.HasPrefix(u, "http") {
			u = m.BaseUrl() + u
		}

		chapters = append(chapters, &MgekoChapter{
			Chapter{
				Number: number,
				Title:  fmt.Sprintf("Chapter %s", formatChapterNumber(number)),
			},
			u,
		})
	})

	return
}

// FetchChapter fetches a chapter and its pages
func (m Mgeko) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MgekoChapter)
	body, err := http.Get(http.RequestParams{
		URL:     mchap.URL,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Number:   f.GetNumber(),
		Language: "en",
	}

	// the reader embeds every real page as <img id="image-N" src="...">,
	// plus one trailing credits/watermark image with no id we must skip
	doc.Find(`#chapter-reader img[id]`).Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("src", "")
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
