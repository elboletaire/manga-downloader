package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Flamecomics is a grabber for flamecomics.xyz: the app is a Next.js SSR
// site, so both the series (chapters list) and chapter (pages) pages embed
// the full data as JSON in a `__NEXT_DATA__` script tag, no browser needed
type Flamecomics struct {
	*Grabber
	title string
}

func NewFlamecomics(g *Grabber) *Flamecomics {
	return &Flamecomics{Grabber: g}
}

// FlamecomicsChapter represents a Flame Comics chapter
type FlamecomicsChapter struct {
	Chapter
	SeriesId int64
	Token    string
}

// Test returns true if the URL is a flamecomics.xyz series URL
func (f *Flamecomics) Test() (bool, error) {
	re := regexp.MustCompile(`flamecomics\.xyz/series/`)
	return re.MatchString(f.URL), nil
}

// FetchTitle fetches and returns the manga title
func (f *Flamecomics) FetchTitle() (string, error) {
	if f.title != "" {
		return f.title, nil
	}

	data, err := f.seriesData()
	if err != nil {
		return "", err
	}

	f.title = sanitizeTitle(data.Props.PageProps.Series.Title)

	return f.title, nil
}

// FetchChapters returns the chapters of the manga
func (f Flamecomics) FetchChapters() (chapters Filterables, errs []error) {
	data, err := f.seriesData()
	if err != nil {
		return nil, []error{err}
	}

	for _, c := range data.Props.PageProps.Chapters {
		number, err := strconv.ParseFloat(c.Chapter, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not parse chapter number %q: %w", c.Chapter, err))
			continue
		}

		title := ""
		if c.Title != nil {
			title = *c.Title
		}
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(number, 'f', -1, 64)
		}

		chapters = append(chapters, &FlamecomicsChapter{
			Chapter{
				Number: number,
				Title:  title,
			},
			c.SeriesId,
			c.Token,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (f Flamecomics) FetchChapter(fl Filterable) (*Chapter, error) {
	fchap := fl.(*FlamecomicsChapter)

	uri := fmt.Sprintf("%s/series/%d/%s", f.BaseUrl(), fchap.SeriesId, fchap.Token)
	body, err := http.Get(http.RequestParams{
		URL: uri,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	nextData := doc.Find("script#__NEXT_DATA__").First().Text()
	if nextData == "" {
		return nil, fmt.Errorf("could not find __NEXT_DATA__ in %s", uri)
	}

	page := flamecomicsChapterPage{}
	if err := json.Unmarshal([]byte(nextData), &page); err != nil {
		return nil, err
	}
	cp := page.Props.PageProps.Chapter

	// images is a map keyed by numeric string ("0", "1", ...), sort it
	// numerically to get the actual page order
	indexes := make([]int, 0, len(cp.Images))
	for k := range cp.Images {
		idx, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)

	chapter := &Chapter{
		Title:      fl.GetTitle(),
		Number:     fl.GetNumber(),
		Language:   "en",
		PagesCount: int64(len(indexes)),
	}
	for i, idx := range indexes {
		img := cp.Images[strconv.Itoa(idx)]
		pageUrl := fmt.Sprintf(
			"https://cdn.flamecomics.xyz/uploads/images/series/%d/%s/%s?%d",
			cp.SeriesId, cp.Token, img.Name, cp.ReleaseDate,
		)
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    pageUrl,
		})
	}

	return chapter, nil
}

// seriesData fetches the series page and extracts the embedded
// `__NEXT_DATA__` JSON, which contains both the series metadata and the
// full chapters list (unlike the visible HTML, which lazy-loads chapters)
func (f Flamecomics) seriesData() (*flamecomicsSeriesPage, error) {
	body, err := http.Get(http.RequestParams{
		URL: f.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	nextData := doc.Find("script#__NEXT_DATA__").First().Text()
	if nextData == "" {
		return nil, fmt.Errorf("could not find __NEXT_DATA__ in %s", f.URL)
	}

	data := &flamecomicsSeriesPage{}
	if err := json.Unmarshal([]byte(nextData), data); err != nil {
		return nil, err
	}

	return data, nil
}

// flamecomicsSeriesPage is the relevant subset of the `__NEXT_DATA__` JSON
// embedded in a series page
type flamecomicsSeriesPage struct {
	Props struct {
		PageProps struct {
			Series struct {
				Title string `json:"title"`
			} `json:"series"`
			Chapters []struct {
				SeriesId int64   `json:"series_id"`
				Chapter  string  `json:"chapter"`
				Title    *string `json:"title"`
				Token    string  `json:"token"`
			} `json:"chapters"`
		} `json:"pageProps"`
	} `json:"props"`
}

// flamecomicsChapterPage is the relevant subset of the `__NEXT_DATA__` JSON
// embedded in a chapter (reader) page
type flamecomicsChapterPage struct {
	Props struct {
		PageProps struct {
			Chapter struct {
				SeriesId    int64  `json:"series_id"`
				Token       string `json:"token"`
				ReleaseDate int64  `json:"release_date"`
				Images      map[string]struct {
					Name string `json:"name"`
				} `json:"images"`
			} `json:"chapter"`
		} `json:"pageProps"`
	} `json:"props"`
}
