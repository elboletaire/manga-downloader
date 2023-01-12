package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// InManga is a grabber for inmanga.com
type InManga struct {
	Grabber
	title string
}

// InMangaChapter is a chapter representation from InManga
type InMangaChapter struct {
	Chapter
	Id string
}

// Test checks if the site is InManga
func (i *InManga) Test() bool {
	re := regexp.MustCompile(`inmanga\.com`)
	return re.MatchString(i.URL)
}

// FetchChapters returns the chapters of the manga
func (i InManga) FetchChapters() Filterables {
	id := GetUUID(i.URL)

	// retrieve chapters json list
	body, err := http.GetText(http.RequestParams{
		URL: "https://inmanga.com/chapter/getall?mangaIdentification=" + id,
	})
	if err != nil {
		panic(err)
	}

	raw := struct {
		Data string
	}{}

	if err = json.Unmarshal([]byte(body), &raw); err != nil {
		panic(err)
	}

	feed := InMangaChapterFeed{}
	err = json.Unmarshal([]byte(raw.Data), &feed)
	if err != nil {
		panic(err)
	}

	return NewInMangaChaptersSlice(feed.Result)
}

// GetTitle fetches the manga title
func (i *InManga) GetTitle() string {
	if i.title != "" {
		return i.title
	}

	body, err := http.Get(http.RequestParams{
		URL: i.URL,
	})
	if err != nil {
		panic(err)
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		panic(err)
	}

	i.title = doc.Find("h1").Text()
	return i.title
}

// FetchChapter fetches the chapter with its pages
func (i InManga) FetchChapter(chap Filterable) Chapter {
	ichap := chap.(*InMangaChapter)
	body, err := http.Get(http.RequestParams{
		URL: "https://inmanga.com/chapter/chapterIndexControls?identification=" + ichap.Id,
	})
	if err != nil {
		panic(err)
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		panic(err)
	}

	chapter := Chapter{
		Title:      chap.GetTitle(),
		Number:     chap.GetNumber(),
		PagesCount: int64(ichap.PagesCount),
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

	return chapter
}

// NewInMangaChapter creates an InMangaChapter from an InMangaChapterFeedResult
func NewInMangaChapter(c InMangaChapterFeedResult) *InMangaChapter {
	return &InMangaChapter{
		Chapter{
			Number:     c.Number,
			PagesCount: int64(c.PagesCount),
			Title:      fmt.Sprintf("Capítulo %04d", int64(c.Number)),
		},
		c.Id,
	}
}

// NewInMangaChaptersSlice creates a slice of Filterables from a slice of InMangaChapterFeedResult
func NewInMangaChaptersSlice(s []InMangaChapterFeedResult) Filterables {
	chapters := make(Filterables, 0, len(s))
	for _, c := range s {
		chapters = append(chapters, NewInMangaChapter(c))
	}

	return chapters
}

// InMangaChapterFeed is the JSON feed for the chapters list
type InMangaChapterFeed struct {
	Result []InMangaChapterFeedResult
}

// InMangaChapterFeedResult is the JSON feed for a single chapter result
type InMangaChapterFeedResult struct {
	Id         string `json:"identification"`
	Number     float64
	PagesCount float64
}
