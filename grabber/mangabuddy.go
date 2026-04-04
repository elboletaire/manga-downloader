package grabber

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// MangaBuddy is a grabber for mangabuddy.com
type MangaBuddy struct {
	*Grabber
	title string
}

func NewMangaBuddy(g *Grabber) *MangaBuddy {
	return &MangaBuddy{Grabber: g}
}

// MangaBuddyChapter represents a MangaBuddy chapter
type MangaBuddyChapter struct {
	Chapter
	URL string
}

// Test checks if the URL is a MangaBuddy URL
func (m *MangaBuddy) Test() (bool, error) {
	re := regexp.MustCompile(`mangabuddy\.com`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches the manga title from the main page
func (m *MangaBuddy) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	body, err := http.Get(http.RequestParams{URL: m.URL})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	m.title = sanitizeTitle(doc.Find("h1").Text())

	return m.title, nil
}

// FetchChapters fetches all chapters from the MangaBuddy chapters API
func (m *MangaBuddy) FetchChapters() (Filterables, []error) {
	slug := m.slug()

	body, err := http.Get(http.RequestParams{
		URL: m.BaseUrl() + "/api/manga/" + slug + "/chapters?source=detail",
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}

	re := regexp.MustCompile(`C(?:hapter|\.)?\s*(\d+\.?\d*)`)

	var chapters Filterables
	var errs []error

	doc.Find("li a").Each(func(_ int, s *goquery.Selection) {
		title := s.Find("strong").Text()
		match := re.FindStringSubmatch(title)
		if len(match) == 0 {
			return
		}

		number, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			errs = append(errs, err)
			return
		}

		href := s.AttrOr("href", "")
		if !strings.HasPrefix(href, "http") {
			href = m.BaseUrl() + href
		}

		chapters = append(chapters, &MangaBuddyChapter{
			Chapter: Chapter{
				Number: number,
				Title:  title,
			},
			URL: href,
		})
	})

	return chapters, errs
}

// FetchChapter fetches a chapter's pages
func (m *MangaBuddy) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaBuddyChapter)

	body, err := http.Get(http.RequestParams{URL: mchap.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	pimages := getPlainHTMLImageURL(".chapter-image img", doc)

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pimages)),
		Language:   "en",
	}

	for i, img := range pimages {
		if img == "" {
			continue
		}
		if !strings.HasPrefix(img, "http") {
			img = m.BaseUrl() + img
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i),
			URL:    img,
		})
	}

	return chapter, nil
}

// slug extracts the manga slug from the URL path
func (m *MangaBuddy) slug() string {
	parts := strings.Split(strings.TrimRight(m.URL, "/"), "/")
	return parts[len(parts)-1]
}
