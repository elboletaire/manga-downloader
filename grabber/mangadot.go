// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// mangadotBase is the site's base URL, for its API and its images (a var so
// tests can point it at a local server)
var mangadotBase = "https://mangadot.net"

// mangadotSeriesRe matches series URLs like https://mangadot.net/manga/229
var mangadotSeriesRe = regexp.MustCompile(`(?i)^https?://[^/]+/manga/(\d+)`)

// mangadotChapterRe matches chapter and whole-volume URLs like
// https://mangadot.net/chapter/39688?source=user or /volume/1807006
var mangadotChapterRe = regexp.MustCompile(`(?i)^https?://[^/]+/(chapter|volume)/(\d+)`)

// Mangadot reads mangadot.net's JSON API: one unauthenticated call lists every
// release of a series (all languages and groups), and the reader's pages come
// from the same API, so no browser is needed
type Mangadot struct {
	*Grabber
	title string
	// languages are the languages of every release the series lists, before
	// the --language filter
	languages []string
}

// MangadotChapter is a release of a chapter. The site keeps two tables with
// overlapping ids, so the source tells which one UploadID belongs to.
type MangadotChapter struct {
	Chapter
	// Source is the release's table: "user" for group uploads, "scraper" for
	// the site's own chapters
	Source string
	// UploadID is the release's id within its source's table
	UploadID string
}

// mangadotListedChapter is a release as the chapter list API sends it
type mangadotListedChapter struct {
	ID            int64      `json:"id"`
	Number        float64    `json:"chapter_number"`
	Volume        jsonVolume `json:"volume_number"`
	Title         *string    `json:"chapter_title"`
	Language      string     `json:"language"`
	Source        string     `json:"source"`
	GroupName     *string    `json:"group_name"`
	ScanlatorName *string    `json:"scanlator_name"`
	Groups        []struct {
		Name string `json:"name"`
	} `json:"groups"`
}

// mangadotPages is the page list of a release. Image URLs are relative to the
// site.
type mangadotPages struct {
	Chapter struct {
		MangaID int64 `json:"manga_id"`
	} `json:"chapter"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

func NewMangadot(g *Grabber) *Mangadot {
	return &Mangadot{Grabber: g}
}

// LanguageOptions lists the languages the series has releases in. FetchChapters
// fills the list, so this is valid after it.
func (m *Mangadot) LanguageOptions() FilterOptions {
	return FilterOptions{
		Values: uniqueValues(m.languages),
		All:    true,
	}
}

// Test returns true if the URL is a mangadot.net series, chapter or volume URL
func (m *Mangadot) Test() (bool, error) {
	if !hostMatchesDomain(m.URL, "mangadot.net") {
		return false, nil
	}
	return mangadotSeriesRe.MatchString(m.URL) || mangadotChapterRe.MatchString(m.URL), nil
}

// resolveSeriesURL turns a chapter or volume URL into its series URL, which the
// API names through the release's own pages
func (m *Mangadot) resolveSeriesURL() error {
	if mangadotSeriesRe.MatchString(m.URL) {
		return nil
	}

	match := mangadotChapterRe.FindStringSubmatch(m.URL)
	if match == nil {
		return errSeriesNotFound(m.URL)
	}

	source := "scraper"
	if u, err := url.Parse(m.URL); err == nil && u.Query().Get("source") == "user" {
		source = "user"
	}
	if match[1] == "volume" {
		source = "user"
	}

	pages, err := m.pages(source, match[2])
	if err != nil || pages.Chapter.MangaID == 0 {
		return errSeriesNotFound(m.URL)
	}

	m.URL = mangadotBase + "/manga/" + strconv.FormatInt(pages.Chapter.MangaID, 10)
	return nil
}

// seriesID returns the series id of the (resolved) series URL
func (m *Mangadot) seriesID() (string, error) {
	match := mangadotSeriesRe.FindStringSubmatch(m.URL)
	if match == nil {
		return "", errSeriesNotFound(m.URL)
	}
	return match[1], nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangadot) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	if err := m.resolveSeriesURL(); err != nil {
		return "", err
	}

	id, err := m.seriesID()
	if err != nil {
		return "", err
	}

	res := struct {
		Manga struct {
			Title string `json:"title"`
		} `json:"manga"`
	}{}
	if err := mangadotGet(mangadotBase+"/api/manga/"+id, &res); err != nil {
		return "", err
	}
	if res.Manga.Title == "" {
		return "", errors.New("could not find the title in the API response")
	}
	m.title = sanitizeTitle(res.Manga.Title)

	return m.title, nil
}

// FetchChapters returns one chapter per release of the language filter
func (m *Mangadot) FetchChapters() (chapters Filterables, errs []error) {
	if err := m.resolveSeriesURL(); err != nil {
		errs = append(errs, err)
		return
	}

	id, err := m.seriesID()
	if err != nil {
		errs = append(errs, err)
		return
	}

	var rows []mangadotListedChapter
	if err := mangadotGet(mangadotBase+"/api/manga/"+id+"/chapters/list", &rows); err != nil {
		errs = append(errs, err)
		return
	}

	for _, row := range rows {
		m.languages = append(m.languages, row.Language)
	}

	// the default is English, or every language when the series has none
	language := m.Settings.Language
	if language == "" {
		language = "en"
		if !slices.Contains(m.languages, language) {
			language = FilterAll
		}
	}

	for _, row := range rows {
		if language != FilterAll && row.Language != language {
			continue
		}
		listedID := strconv.FormatInt(row.ID, 10)
		listed := &MangadotChapter{
			Chapter: Chapter{
				Number:   row.Number,
				Title:    strings.TrimSpace(derefString(row.Title)),
				Language: row.Language,
				ID:       row.Source + ":" + listedID,
				Volume:   int(row.Volume),
				Label:    Label{Style: LabelNumbered},
			},
			Source:   row.Source,
			UploadID: listedID,
		}
		listed.SetGroups(mangadotGroups(row)...)
		chapters = append(chapters, listed)
	}

	return
}

// FetchChapter fetches a release and its pages
func (m *Mangadot) FetchChapter(f Filterable) (*Chapter, error) {
	listed := f.(*MangadotChapter)

	pages, err := m.pages(listed.Source, listed.UploadID)
	if err != nil {
		return nil, err
	}
	if len(pages.Images) == 0 {
		return nil, fmt.Errorf("no pages found for chapter %s", listed.ID)
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   listed.Language,
		ID:         listed.ID,
		Volume:     listed.Volume,
		PagesCount: int64(len(pages.Images)),
		Label:      labelOf(f),
	}
	chapter.SetGroups(listed.GetGroups()...)
	for i, img := range pages.Images {
		imgURL := img.URL
		if strings.HasPrefix(imgURL, "/") {
			imgURL = mangadotBase + imgURL
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    imgURL,
		})
	}

	return chapter, nil
}

// pages reads the page list of a release: group uploads live in their own
// table, the site's chapters in the other one
func (m *Mangadot) pages(source, id string) (*mangadotPages, error) {
	endpoint := "chapters"
	if source == "user" {
		endpoint = "uploads"
	}

	pages := &mangadotPages{}
	if err := mangadotGet(mangadotBase+"/api/"+endpoint+"/"+id+"/images", pages); err != nil {
		return nil, err
	}

	return pages, nil
}

// mangadotGroups returns the groups of a release: its groups list, else the
// group name, else the scanlator name it was uploaded under
func mangadotGroups(row mangadotListedChapter) []string {
	names := make([]string, 0, len(row.Groups))
	for _, g := range row.Groups {
		names = append(names, g.Name)
	}
	if len(names) > 0 {
		return names
	}

	for _, name := range []*string{row.GroupName, row.ScanlatorName} {
		if name != nil && strings.TrimSpace(*name) != "" {
			return []string{strings.TrimSpace(*name)}
		}
	}

	return nil
}

// mangadotGet decodes the JSON at the URI into v
func mangadotGet(uri string, v any) error {
	rbody, err := http.Get(http.RequestParams{URL: uri})
	if err != nil {
		return err
	}
	defer rbody.Close()

	return json.NewDecoder(rbody).Decode(v)
}

// derefString returns the string a pointer holds, "" for nil
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
