package grabber

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/html"
	"github.com/elboletaire/manga-downloader/models"
	"github.com/elgs/gojq"
)

type InManga struct {
	URL   string
	title string
}

func (i InManga) Test() bool {
	re := regexp.MustCompile(`inmanga\.com`)
	return re.MatchString(i.URL)
}

func (i InManga) FetchChapters() models.Filterables {
	id := GetUUID(i.URL)

	// retrieve chapters json from server
	rbody, err := downloader.Get("https://inmanga.com/chapter/getall?mangaIdentification=" + id)
	if err != nil {
		panic(err)
	}
	defer rbody.Close()
	body := new(strings.Builder)
	io.Copy(body, rbody)

	parser, err := gojq.NewStringQuery(body.String())
	if err != nil {
		panic(err)
	}
	data, _ := parser.QueryToString("data")
	ps, err := gojq.NewStringQuery(data)
	if err != nil {
		panic(err)
	}
	cps, err := ps.Query("result")
	if err != nil {
		panic(err)
	}

	chapters := NewInMangaSlice(cps.([]interface{}))
	return chapters
}

func (i InManga) Title() string {
	if i.title != "" {
		return i.title
	}

	rbody, err := downloader.Get(i.URL)
	if err != nil {
		panic(err)
	}
	defer rbody.Close()
	body := new(strings.Builder)
	io.Copy(body, rbody)

	doc := html.Reader(body.String())
	i.title = html.Query(doc, "h1").FirstChild.Data

	return i.title
}

func (i InManga) FetchChapter(chap models.Filterable) models.Chapter {
	ichap := chap.(*InMangaChapter)
	h, err := downloader.Get("https://inmanga.com/chapter/chapterIndexControls?identification=" + ichap.Identification)
	if err != nil {
		panic(err)
	}
	defer h.Close()
	strhtml := new(strings.Builder)
	io.Copy(strhtml, h)

	// fmt.Println(string(strhtml))
	doc := html.Reader(strhtml.String())
	chapter := models.Chapter{
		Number:     chap.GetNumber(),
		PagesCount: int64(ichap.PagesCount),
	}

	s := html.Query(doc, "select.PageListClass")
	for _, opt := range html.QueryAll(s, "option") {
		page, _ := strconv.ParseInt(opt.FirstChild.Data, 10, 64)
		chapter.Pages = append(chapter.Pages, models.Page{
			Number: page,
			URL:    "https://pack-yak.intomanga.com/images/manga/MANGA-SERIES/chapter/CHAPTER/page/PAGE/" + opt.Attr[0].Val,
		})
	}

	return chapter
}

type InMangaChapters []*InMangaChapter

type InMangaChapter struct {
	Number         float64
	Identification string
	PagesCount     float64
	Title          string
}

func (i *InMangaChapter) GetNumber() float64 {
	return i.Number
}

func (i *InMangaChapter) GetTitle() string {
	return i.Title
}

func NewInMangaChapter(c map[string]interface{}) *InMangaChapter {
	n := c["Number"].(float64)
	i := c["Identification"].(string)
	p := c["PagesCount"].(float64)
	t := fmt.Sprintf("Capítulo %04d", int64(n))

	return &InMangaChapter{
		Number:         n,
		Identification: i,
		PagesCount:     p,
		Title:          t,
	}
}

func NewInMangaSlice(s []interface{}) models.Filterables {
	chapters := make(models.Filterables, 0, len(s))
	for _, c := range s {
		chapters = append(chapters, NewInMangaChapter(c.(map[string]interface{})))
	}

	return chapters
}
