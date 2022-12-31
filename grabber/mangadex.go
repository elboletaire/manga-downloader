package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/models"
	"github.com/elgs/gojq"
)

type MangaDex struct {
	URL   string
	title string
}

func (m MangaDex) Test() bool {
	re := regexp.MustCompile(`mangadex\.org`)
	return re.MatchString(m.URL)
}

func (m MangaDex) Title() string {
	id := GetUUID(m.URL)

	rbody, err := downloader.Get("https://api.mangadex.org/manga/" + id)
	if err != nil {
		panic(err)
	}
	body := new(strings.Builder)
	io.Copy(body, rbody)

	parser, err := gojq.NewStringQuery(body.String())
	if err != nil {
		panic(err)
	}
	title, _ := parser.QueryToString("data.attributes.title.en")

	return title
}

func (m MangaDex) FetchChapters() models.Filterables {
	id := GetUUID(m.URL)

	// TODO needs to be recursive in order to search for all coincidences
	// (although forced to spanish as rn probably never gets to the limit)
	rbody, err := downloader.Get("https://api.mangadex.org/manga/" + id + "/feed?limit=500&order[volume]=asc&order[chapter]=asc&translatedLanguage[]=es")
	if err != nil {
		panic(err)
	}
	body := MangaDexFeed{}
	err = json.NewDecoder(rbody).Decode(&body)
	if err != nil {
		panic(err)
	}

	var chapters models.Filterables
	for _, c := range body.Data {
		num, _ := strconv.ParseInt(c.Attributes.Chapter, 10, 64)
		chapters = append(chapters, &MangaDexChapter{
			ID:     c.ID,
			Number: num,
			Title:  c.Attributes.Title,
		})
	}

	return chapters
}

func (m MangaDex) FetchChapter(f models.Filterable) models.Chapter {
	mchap := f.(*MangaDexChapter)
	rbody, err := downloader.Get("https://api.mangadex.org/at-home/server/" + mchap.ID)
	if err != nil {
		panic(err)
	}
	body := MangaDexPagesFeed{}
	err = json.NewDecoder(rbody).Decode(&body)
	if err != nil {
		panic(err)
	}
	chapter := models.Chapter{
		Title:      fmt.Sprintf("Chapter %04d %s", int64(f.GetNumber()), mchap.Title),
		Number:     f.GetNumber(),
		PagesCount: int64(len(body.Chapter.Data)),
	}
	for i, p := range body.Chapter.Data {
		chapter.Pages = append(chapter.Pages, models.Page{
			Number: int64(i + 1),
			URL:    body.BaseUrl + path.Join("/data", body.Chapter.Hash, p),
		})
	}

	return chapter
}

type MangaDexChapter struct {
	ID     string
	Number int64
	Title  string
}

type MangaDexFeedChapter struct {
	ID         string
	Attributes MangaDexFeedChapterAttributes
}

type MangaDexFeedChapterAttributes struct {
	Volume  string
	Chapter string
	Title   string
}

type MangaDexFeed struct {
	Data []MangaDexFeedChapter
}

type MangaDexPagesFeedChapterData []string

type MangaDexPagesFeedChapter struct {
	Hash      string
	Data      MangaDexPagesFeedChapterData
	DataSaver MangaDexPagesFeedChapterData
}

type MangaDexPagesFeed struct {
	BaseUrl string
	Chapter MangaDexPagesFeedChapter
}

func (m *MangaDexChapter) GetTitle() string {
	return m.Title
}

func (m *MangaDexChapter) GetNumber() float64 {
	return float64(m.Number)
}
