// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// HeavenManga is a grabber for heavenmanga.com (Spanish). The series page has
// no chapters: its Laravel DataTables list is served as JSON by the same URL,
// one row per release, each with its own reader ID and uploader label.
type HeavenManga struct {
	*Grabber
}

// heavenMangaSlugRe captures the slug of a series URL (/manga/{slug}, with an
// optional numeric suffix). Reader URLs (/manga/leer/{id}) match too, see heavenMangaSlug
var heavenMangaSlugRe = regexp.MustCompile(`^https?://[^/]+/manga/([^/?#]+)(?:/\d+)?/?(?:[?#].*)?$`)

// heavenMangaPageRe matches each page of the reader, which the server ships
// as a JSON list of {"npage": N, "imgURL": "..."} in the reader's script
var heavenMangaPageRe = regexp.MustCompile(`"npage":\s*\d+,\s*"imgURL":\s*"((?:[^"\\]|\\.)*)"`)

// heavenMangaNoGroup is the label on releases that name no uploader
const heavenMangaNoGroup = "Solo Subida"

func NewHeavenManga(g *Grabber) *HeavenManga {
	return &HeavenManga{Grabber: g}
}

// HeavenMangaChapter represents a HeavenManga chapter release
type HeavenMangaChapter struct {
	Chapter
	URL string
}

// heavenMangaSlug returns the series slug of a series or reader URL, "" for
// anything else. The reader lives under /manga/leer/, which is no series.
func heavenMangaSlug(rawURL string) string {
	match := heavenMangaSlugRe.FindStringSubmatch(rawURL)
	if len(match) < 2 || match[1] == "leer" {
		return ""
	}
	return match[1]
}

// Test returns true if the URL is a heavenmanga.com URL
func (m *HeavenManga) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil {
		return false, err
	}
	return strings.TrimPrefix(u.Hostname(), "www.") == "heavenmanga.com", nil
}

// resolveSeriesURL turns a reader URL into its series URL. Readers link back
// to the series, so that's one fetch, made only when the URL isn't a series.
func (m *HeavenManga) resolveSeriesURL() error {
	if slug := heavenMangaSlug(m.URL); slug != "" {
		m.URL = m.BaseUrl() + "/manga/" + slug
		return nil
	}

	body, err := http.Get(http.RequestParams{URL: m.URL})
	if err != nil {
		return err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}
	for _, href := range doc.Find("a[href]").Map(func(_ int, s *goquery.Selection) string {
		return s.AttrOr("href", "")
	}) {
		if slug := heavenMangaSlug(href); slug != "" {
			m.URL = m.BaseUrl() + "/manga/" + slug
			return nil
		}
	}

	return errSeriesNotFound(m.URL)
}

// FetchTitle returns the series title
func (m *HeavenManga) FetchTitle() (string, error) {
	if err := m.resolveSeriesURL(); err != nil {
		return "", err
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

	return sanitizeTitle(doc.Find("h3.manga-title").Text()), nil
}

// heavenMangaRow is one row of the chapter table: a release, its ID being the
// reader's, and the HTML of its panel
type heavenMangaRow struct {
	ID     int64  `json:"id"`
	Number string `json:"number"`
}

// heavenMangaTable is the DataTables JSON the series page answers with
type heavenMangaTable struct {
	Data []heavenMangaRow `json:"data"`
}

// FetchChapters returns one chapter per release, with its uploader's group
func (m *HeavenManga) FetchChapters() (chapters Filterables, errs []error) {
	if err := m.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	// length=-1 returns every row in one request: paging would reorder the
	// releases of a number between pages
	query := url.Values{}
	query.Set("draw", "1")
	query.Set("columns[0][data]", "number")
	query.Set("columns[1][data]", "created_at")
	query.Set("start", "0")
	query.Set("length", "-1")
	query.Set("search[value]", "")

	body, err := http.Get(http.RequestParams{
		URL:     m.URL + "?" + query.Encode(),
		Accept:  "application/json",
		Headers: map[string]string{"X-Requested-With": "XMLHttpRequest"},
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	var table heavenMangaTable
	if err := json.NewDecoder(body).Decode(&table); err != nil {
		return nil, []error{err}
	}

	for _, row := range table.Data {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(row.Number))
		if err != nil {
			continue
		}
		number, ok := parseChapterNumber(doc.Find("h3.h-inherit").Text())
		if !ok {
			continue
		}

		id := strconv.FormatInt(row.ID, 10)
		chapter := &HeavenMangaChapter{
			Chapter{
				Number: number,
				ID:     id,
				Label:  Label{Style: LabelNumbered},
			},
			m.BaseUrl() + "/manga/leer/" + id,
		}
		if group := sanitizeTitle(doc.Find("span.greenSpan").Text()); group != "" && group != heavenMangaNoGroup {
			chapter.SetGroups(group)
		}

		chapters = append(chapters, chapter)
	}

	return chapters, nil
}

// FetchChapter fetches the reader page, which lists every page of the release
func (m *HeavenManga) FetchChapter(f Filterable) (*Chapter, error) {
	listed := f.(*HeavenMangaChapter)

	body, err := http.Get(http.RequestParams{
		URL: listed.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "es",
		ID:       listed.ID,
	}
	chapter.SetGroups(listed.GetGroups()...)

	for i, match := range heavenMangaPageRe.FindAllStringSubmatch(string(raw), -1) {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    strings.ReplaceAll(match[1], `\/`, "/"),
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
