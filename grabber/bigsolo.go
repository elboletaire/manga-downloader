// Copyright (C) 2023-2026 Òscar Casajuana Alonso

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

// Bigsolo is a grabber for bigsolo.org, a French scanlation aggregator. The
// series page is server-rendered and embeds the full series info (including
// every chapter) as JSON in a `#series-data-placeholder` script tag, so no
// browser is needed. Chapter pages themselves are not hosted on bigsolo.org:
// each chapter only stores a reference to an imgchest.com "post" (an
// image-hosting service), whose page in turn embeds the ordered image list
// as JSON in the `data-page` attribute of its Inertia.js `#app` root div.
type Bigsolo struct {
	*Grabber
	series *bigsoloSeries
}

func NewBigsolo(g *Grabber) *Bigsolo {
	return &Bigsolo{Grabber: g}
}

// BigsoloChapter represents a Bigsolo chapter
type BigsoloChapter struct {
	Chapter
	// ImgchestID is the imgchest.com post id hosting this chapter's pages
	ImgchestID string
}

// Test returns true if the URL is a bigsolo.org URL
func (b *Bigsolo) Test() (bool, error) {
	re := regexp.MustCompile(`bigsolo\.org`)
	return re.MatchString(b.URL), nil
}

// FetchTitle fetches and returns the manga title
func (b *Bigsolo) FetchTitle() (string, error) {
	series, err := b.fetchSeries()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(series.Title), nil
}

// FetchChapters returns the chapters of the manga
func (b *Bigsolo) FetchChapters() (chapters Filterables, errs []error) {
	series, err := b.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	for key, c := range series.Chapters {
		number, err := strconv.ParseFloat(key, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not parse chapter number %q: %w", key, err))
			continue
		}
		if c.Source.Service != "imgchest" || c.Source.Id == "" {
			errs = append(errs, fmt.Errorf("chapter %q has no supported source", key))
			continue
		}

		title := c.Title
		if title == "" {
			title = "Chapitre " + key
		}

		chapters = append(chapters, &BigsoloChapter{
			Chapter{
				Number: number,
				Title:  title,
			},
			c.Source.Id,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (b Bigsolo) FetchChapter(f Filterable) (*Chapter, error) {
	bchap := f.(*BigsoloChapter)

	uri := fmt.Sprintf("https://imgchest.com/p/%s", bchap.ImgchestID)
	body, err := http.Get(http.RequestParams{URL: uri})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	raw, ok := doc.Find("#app").Attr("data-page")
	if !ok || raw == "" {
		return nil, fmt.Errorf("could not find imgchest post data in %s", uri)
	}

	page := &imgchestPostPage{}
	if err := json.Unmarshal([]byte(raw), page); err != nil {
		return nil, err
	}

	files := page.Props.Post.Files
	sort.Slice(files, func(i, j int) bool {
		return files[i].Position < files[j].Position
	})

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(files)),
		Language:   "fr",
	}
	for i, file := range files {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    file.Link,
		})
	}

	return chapter, nil
}

// fetchSeries fetches and caches the series info from the series page
func (b *Bigsolo) fetchSeries() (*bigsoloSeries, error) {
	if b.series != nil {
		return b.series, nil
	}

	body, err := http.Get(http.RequestParams{URL: b.URL})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	raw := doc.Find("#series-data-placeholder").Text()
	if raw == "" {
		return nil, fmt.Errorf("could not find series data in %s (is the URL a series page?)", b.URL)
	}

	series := &bigsoloSeries{}
	if err := json.Unmarshal([]byte(raw), series); err != nil {
		return nil, err
	}

	b.series = series

	return series, nil
}

// bigsoloSeries is the series info embedded in the series page's
// `#series-data-placeholder` script tag
type bigsoloSeries struct {
	Title    string `json:"title"`
	Chapters map[string]struct {
		Title  string `json:"title"`
		Source struct {
			Service string `json:"service"`
			Id      string `json:"id"`
		} `json:"source"`
	} `json:"chapters"`
}

// imgchestPostPage is the relevant subset of the Inertia.js `data-page` JSON
// blob embedded in an imgchest.com post page
type imgchestPostPage struct {
	Props struct {
		Post struct {
			Files []struct {
				Link     string `json:"link"`
				Position int    `json:"position"`
			} `json:"files"`
		} `json:"post"`
	} `json:"props"`
}
