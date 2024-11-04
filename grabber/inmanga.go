package grabber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
)

// Inmanga is a grabber for inmanga.com
type Inmanga struct {
	*Grabber
	title string
}

// InmangaChapter is a chapter representation from InManga
type InmangaChapter struct {
	Chapter
	Id string
}

// ValidateURL checks if the site is InManga
func (i *Inmanga) ValidateURL() (bool, error) {
	re := regexp.MustCompile(`inmanga\.com`)
	return re.MatchString(i.URL), nil
}

// FetchTitle fetches the manga title
func (i *Inmanga) FetchTitle() (string, error) {
	if i.title != "" {
		return i.title, nil
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, i.URL, http.NoBody)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	i.title = doc.Find("h1").Text()

	return i.title, nil
}

// FetchChapters returns the chapters of the manga
func (i Inmanga) FetchChapters() (Filterables, error) {
	id, err := getUUID(i.URL)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://inmanga.com/chapter/getall?mangaIdentification="+id.String(), http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	buff := new(bytes.Buffer)
	io.Copy(buff, resp.Body)

	raw := struct {
		Data string
	}{}

	err = json.Unmarshal(buff.Bytes(), &raw)
	if err != nil {
		return nil, err
	}

	feed := inmangaChapterFeed{}
	err = json.Unmarshal([]byte(raw.Data), &feed)
	if err != nil {
		return nil, err
	}

	chapters := make(Filterables, 0, len(feed.Result))
	for _, c := range feed.Result {
		chapters = append(chapters, newInmangaChapter(c))
	}

	return chapters, nil
}

// FetchChapter fetches the chapter with its pages
func (i Inmanga) FetchChapter(chap Filterable) (*Chapter, error) {
	ichap, _ := chap.(*InmangaChapter)

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://inmanga.com/chapter/chapterIndexControls?identification="+ichap.Id, http.NoBody)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      chap.GetTitle(),
		Number:     chap.GetNumber(),
		PagesCount: ichap.PagesCount,
		// Inmanga only hosts spanish mangas
		Language: "es",
	}

	// get pages from select, but discard one, since it's duplicated
	doc.Find("select.PageListClass").First().Children().Each(func(i int, s *goquery.Selection) {
		num, _ := strconv.ParseInt(s.Text(), 10, 64)
		chapter.Pages = append(chapter.Pages, Page{
			Number: num,
			URL:    "https://pack-yak.intomanga.com/images/manga/ms/chapter/ch/page/p/" + s.AttrOr("value", ""),
		})
	})

	return chapter, nil
}

// newInmangaChapter creates an InMangaChapter from an InMangaChapterFeedResult
func newInmangaChapter(c inmangaChapterFeedResult) *InmangaChapter {
	return &InmangaChapter{
		Chapter{
			Number:     c.Number,
			PagesCount: int64(c.PagesCount),
			Title:      fmt.Sprintf("Capítulo %04d", int64(c.Number)),
		},
		c.ID,
	}
}

// inmangaChapterFeed is the JSON feed for the chapters list
type inmangaChapterFeed struct {
	Result []inmangaChapterFeedResult
}

// inmangaChapterFeedResult is the JSON feed for a single chapter result
type inmangaChapterFeedResult struct {
	ID         string `json:"identification"`
	Number     float64
	PagesCount float64
}
