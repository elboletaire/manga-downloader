package grabber

import (
	"fmt"
	"sort"

	"github.com/elboletaire/manga-downloader/ranges"
)

type Page struct {
	Number int64
	URL    string
}

type Pages []Page

type Chapter struct {
	Title      string
	Number     float64
	PagesCount int64
	Pages      Pages
}

type Chapters []Chapter

type InMangaChapters []*InMangaChapter

type InMangaChapter struct {
	Number         float64
	Identification string
	PagesCount     float64
	Title          string
}

func New(c map[string]interface{}) *InMangaChapter {
	n := c["Number"].(float64)
	i := c["Identification"].(string)
	p := c["PagesCount"].(float64)
	t := fmt.Sprintf("Chapter %d", int64(n))

	return &InMangaChapter{
		Number:         n,
		Identification: i,
		PagesCount:     p,
		Title:          t,
	}
}

func NewSlice(s []interface{}) InMangaChapters {
	chapters := make(InMangaChapters, 0, len(s))
	for _, c := range s {
		chapters = append(chapters, New(c.(map[string]interface{})))
	}
	return chapters
}

func (c InMangaChapters) SortByNumber() InMangaChapters {
	sort.Slice(c, func(i, j int) bool {
		return c[i].Number < c[j].Number
	})

	return c
}

func (c InMangaChapters) Filter(cond func(*InMangaChapter) bool) InMangaChapters {
	var filtered InMangaChapters
	for _, chap := range c {
		if cond(chap) {
			filtered = append(filtered, chap)
		}
	}

	return filtered
}

func (c InMangaChapters) GetRanges(rngs ranges.Ranges) InMangaChapters {
	var chaps InMangaChapters
	for _, r := range rngs {
		chaps = append(chaps, c.Filter(func(c *InMangaChapter) bool {
			return c.Number >= float64(r.Begin) && c.Number <= float64(r.End)
		})...)
	}

	return chaps
}
