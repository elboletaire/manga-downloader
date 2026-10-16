// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// fmteamChapterRe matches chapter reader URLs like
// https://fmteam.fr/read/batuque/fr/ch/157 and decimal variants like ch/12.5.
// www. is accepted; trailing slash, query and fragment are all tolerated.
var fmteamChapterRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?fmteam\.fr/read/([^/]+)/([^/]+)/ch/[\d.]+/?(?:[?#].*)?$`)

// Fmteam is a grabber for fmteam.fr: a French scanlation site whose Vue SPA
// exposes a wide-open JSON API (const API_BASE_URL = BASE_URL + 'api' in the
// homepage HTML) for both the chapters list and the reader pages, so no
// browser is needed and images download over plain HTTP
type Fmteam struct {
	*Grabber
	title string
	// languages are the languages of every chapter the series lists
	languages []string
}

func NewFmteam(g *Grabber) *Fmteam {
	return &Fmteam{Grabber: g}
}

// LanguageOptions lists the languages the series has chapters in. The chapters
// are only fetched once FetchChapters has read them, so this is valid after it.
func (f *Fmteam) LanguageOptions() FilterOptions {
	return FilterOptions{
		Values: uniqueValues(f.languages),
		All:    true,
	}
}

// FmteamChapter represents a Fmteam chapter
type FmteamChapter struct {
	Chapter
	// ReadUrl is the chapter's relative reader path (e.g.
	// "/read/batuque/fr/ch/157"), reused as-is under /api to fetch pages
	ReadUrl string
}

// Test returns true if the URL is a fmteam.fr series or chapter URL.
// Chapter URLs are resolved to series URLs lazily in FetchTitle/FetchChapters.
func (f *Fmteam) Test() (bool, error) {
	re := regexp.MustCompile(`(?i)fmteam\.fr/(?:comics/|read/)`)
	return re.MatchString(f.URL), nil
}

// FetchTitle fetches and returns the manga title
func (f *Fmteam) FetchTitle() (string, error) {
	if f.title != "" {
		return f.title, nil
	}

	f.resolveSeriesURL()

	data, err := f.seriesData()
	if err != nil {
		return "", err
	}

	f.title = sanitizeTitle(data.Comic.Title)

	return f.title, nil
}

// FetchChapters returns the chapters of the manga
func (f *Fmteam) FetchChapters() (chapters Filterables, errs []error) {
	f.resolveSeriesURL()

	data, err := f.seriesData()
	if err != nil {
		return nil, []error{err}
	}

	for _, c := range data.Comic.Chapters {
		f.languages = append(f.languages, c.Language)
		number, err := fmteamChapterNumber(c)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		fchap := &FmteamChapter{
			Chapter{
				Number:   number,
				Title:    strings.TrimSpace(c.Title),
				Label:    Label{Style: LabelNumbered, Lang: "fr"},
				Language: c.Language,
				ID:       c.Url,
			},
			c.Url,
		}
		fchap.SetGroups(teamNames(c.Teams)...)
		chapters = append(chapters, fchap)
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (f Fmteam) FetchChapter(fl Filterable) (*Chapter, error) {
	fchap := fl.(*FmteamChapter)

	uri := f.BaseUrl() + "/api" + fchap.ReadUrl
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: f.URL,
	})
	if err != nil {
		return nil, err
	}

	page := fmteamReadPage{}
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      fl.GetTitle(),
		Label:      labelOf(fl),
		Number:     fl.GetNumber(),
		Language:   fchap.Language,
		ID:         fchap.ID,
		PagesCount: int64(len(page.Chapter.Pages)),
	}
	chapter.SetGroups(fchap.GetGroups()...)
	for i, url := range page.Chapter.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    url,
		})
	}

	return chapter, nil
}

// resolveSeriesURL converts a chapter URL to a series URL if needed.
func (f *Fmteam) resolveSeriesURL() {
	if !fmteamChapterRe.MatchString(f.URL) {
		return
	}
	matches := fmteamChapterRe.FindStringSubmatch(f.URL)
	if len(matches) >= 2 {
		slug := matches[1]
		f.URL = fmt.Sprintf("https://fmteam.fr/comics/%s", slug)
	}
}

// seriesSlug returns the series slug from the URL (i.e. "batuque" for
// https://fmteam.fr/comics/batuque)
func (f Fmteam) seriesSlug() (string, error) {
	re := regexp.MustCompile(`/comics/([^/?#]+)`)
	matches := re.FindStringSubmatch(f.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series slug in url %s", f.URL)
	}
	return matches[1], nil
}

// seriesData fetches the series JSON API (chapters list + metadata), which
// unlike the visible SPA HTML already contains everything server-side
func (f Fmteam) seriesData() (*fmteamComicPage, error) {
	slug, err := f.seriesSlug()
	if err != nil {
		return nil, err
	}

	uri := f.BaseUrl() + "/api/comics/" + slug
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: f.URL,
	})
	if err != nil {
		return nil, err
	}

	data := &fmteamComicPage{}
	if err := json.Unmarshal([]byte(body), data); err != nil {
		return nil, err
	}

	return data, nil
}

// fmteamChapterNumber builds the chapter's float number from its "chapter"
// and (rarely populated) "subchapter" fields, i.e. chapter=10 subchapter="5"
// becomes 10.5
func fmteamChapterNumber(c fmteamChapterJson) (float64, error) {
	numberStr := strconv.FormatFloat(c.Number, 'f', -1, 64)
	if c.Subchapter != nil && *c.Subchapter != "" && *c.Subchapter != "0" {
		numberStr += "." + *c.Subchapter
	}

	number, err := strconv.ParseFloat(numberStr, 64)
	if err != nil {
		return 0, fmt.Errorf("could not parse chapter number %q: %w", numberStr, err)
	}

	return number, nil
}

// fmteamComicPage is the relevant subset of the /api/comics/{slug} JSON
type fmteamComicPage struct {
	Comic struct {
		Title    string              `json:"title"`
		Chapters []fmteamChapterJson `json:"chapters"`
	} `json:"comic"`
}

// fmteamChapterJson is a chapter entry as returned by the chapters list API
type fmteamChapterJson struct {
	Number     float64 `json:"chapter"`
	Subchapter *string `json:"subchapter"`
	Title      string  `json:"title"`
	Language   string  `json:"language"`
	Url        string  `json:"url"`
	// Teams are the groups that made the chapter; the list can hold nulls
	Teams []*feedTeam `json:"teams"`
}

// fmteamReadPage is the relevant subset of the /api/read/{slug}/{lang}/ch/{n}
// JSON: "pages" is a plain, already-ordered list of direct image URLs
type fmteamReadPage struct {
	Chapter struct {
		Pages []string `json:"pages"`
	} `json:"chapter"`
}
