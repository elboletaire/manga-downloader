package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/models"
	"github.com/elgs/gojq"
)

type MangaDex struct {
	Grabber
	title string
}

func (m *MangaDex) Test() bool {
	re := regexp.MustCompile(`mangadex\.org`)
	return re.MatchString(m.URL)
}

func (m MangaDex) Title() string {
	if m.title != "" {
		return m.title
	}

	id := GetUUID(m.URL)

	rbody, err := downloader.Get(downloader.GetParams{
		URL: "https://api.mangadex.org/manga/" + id,
	})
	if err != nil {
		panic(err)
	}
	body := new(strings.Builder)
	io.Copy(body, rbody)

	parser, err := gojq.NewStringQuery(body.String())
	if err != nil {
		panic(err)
	}
	m.title, _ = parser.QueryToString("data.attributes.title.en")

	return m.title
}

func (m MangaDex) FetchChapters(language string) models.Filterables {
	id := GetUUID(m.URL)

	var chapters models.Filterables
	var fetchChaps func(int)

	baseOffset := 500

	fetchChaps = func(offset int) {
		uri, err := url.JoinPath("https://api.mangadex.org", "manga", id, "feed")
		if err != nil {
			panic(err)
		}
		params := url.Values{}
		params.Add("limit", fmt.Sprint(baseOffset))
		params.Add("order[volume]", "asc")
		params.Add("order[chapter]", "asc")
		params.Add("offset", fmt.Sprint(offset))
		if language != "" {
			params.Add("translatedLanguage[]", language)
		}
		uri = fmt.Sprintf("%s?%s", uri, params.Encode())

		rbody, err := downloader.Get(downloader.GetParams{URL: uri})
		if err != nil {
			panic(err)
		}
		body := MangaDexFeed{}
		err = json.NewDecoder(rbody).Decode(&body)
		if err != nil {
			panic(err)
		}

		for _, c := range body.Data {
			num, _ := strconv.ParseInt(c.Attributes.Chapter, 10, 64)
			chapters = append(chapters, &MangaDexChapter{
				ID:       c.ID,
				Number:   num,
				Title:    c.Attributes.Title,
				Language: c.Attributes.TranslatedLanguage,
			})
		}

		if len(body.Data) > 0 {
			fetchChaps(offset + baseOffset)
		}
	}
	fetchChaps(0)

	return chapters
}

func (m MangaDex) FetchChapter(f models.Filterable) models.Chapter {
	mchap := f.(*MangaDexChapter)
	rbody, err := downloader.Get(downloader.GetParams{
		URL: "https://api.mangadex.org/at-home/server/" + mchap.ID,
	})
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
		Language:   mchap.Language,
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
	ID       string
	Number   int64
	Title    string
	Language string
}

type MangaDexFeedChapter struct {
	ID         string
	Attributes MangaDexFeedChapterAttributes
}

type MangaDexFeedChapterAttributes struct {
	Volume             string
	Chapter            string
	Title              string
	TranslatedLanguage string
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
