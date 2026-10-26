// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// MangaHub is a grabber for the MangaHub platform (mangahub.io and the sites running its app).
// Its reader shows only ~6 pages, so the title, chapters and pages come from the GraphQL API.
type MangaHub struct {
	*Grabber
	// manga is the series from the API, fetched once for FetchTitle and FetchChapters
	manga *mangahubManga
}

// NewMangaHub returns a MangaHub grabber, matched by the markup of its pages
func NewMangaHub(g *Grabber) *MangaHub {
	return &MangaHub{Grabber: g}
}

// mangahubAPI is the GraphQL endpoint, a variable so the tests can serve a fake
var mangahubAPI = "https://api.mghcdn.com/graphql"

// mangahubCDN serves the page images, relative to it
const mangahubCDN = "https://imgx.mghcdn.com/"

// mangahubMarker is the API config every page of the platform carries
const mangahubMarker = `"apiUrl":"https:\u002F\u002Fapi.mghcdn.com"`

// mangahubPathRe is the path shape of a series or reader page, the only shape the markup check fetches
var mangahubPathRe = regexp.MustCompile(`(?i)^/(manga/[^/]+|chapter/[^/]+/chapter-[^/]+)/?$`)

// mangahubChapterPathRe captures the origin and slug of a reader URL (/chapter/{slug}/chapter-{n})
var mangahubChapterPathRe = regexp.MustCompile(`(?i)^(https?://[^/?#]+)/chapter/([^/?#]+)/chapter-[^/?#]*`)

// mangahubSlugRe captures the series slug of a series URL
var mangahubSlugRe = regexp.MustCompile(`/manga/([^/?#]+)`)

// mangahubClientRe captures the client bundle a page loads, the one with every site's config
var mangahubClientRe = regexp.MustCompile(`src="(/assets/client[^"]*)"`)

// mangahubNumberTitleRe matches a title that only names the chapter's number, which ChapterLabel names instead
var mangahubNumberTitleRe = regexp.MustCompile(`(?i)^chapter\s*[\d.]+$`)

// mangahubManga is the series as the API returns it. source and slug are the
// keys its chapters' pages are fetched by
type mangahubManga struct {
	Title    string `json:"title"`
	Chapters []struct {
		Number float64 `json:"number"`
		Title  string  `json:"title"`
	} `json:"chapters"`
	source string
	slug   string
	key    string
}

// MangaHubChapter is a chapter of the list, with what its pages are fetched by
type MangaHubChapter struct {
	Chapter
	source string
	slug   string
	key    string
}

// Test matches a series or reader page by the API config its markup carries,
// with no domain list: a clone of the platform needs no code of its own
func (m *MangaHub) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil || !mangahubPathRe.MatchString(u.Path) {
		return false, nil
	}

	return pageHasMarkup(m.Grabber, m.URL, mangahubMarker), nil
}

// resolveSeriesURL rewrites a reader URL to its series URL. Series URLs are unchanged.
func (m *MangaHub) resolveSeriesURL() {
	if match := mangahubChapterPathRe.FindStringSubmatch(m.URL); match != nil {
		m.URL = match[1] + "/manga/" + match[2]
	}
}

// FetchTitle returns the series title
func (m *MangaHub) FetchTitle() (string, error) {
	manga, err := m.fetchManga()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(manga.Title), nil
}

// FetchChapters returns the chapters of the series, from the API
func (m *MangaHub) FetchChapters() (Filterables, []error) {
	manga, err := m.fetchManga()
	if err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(manga.Chapters))
	for _, c := range manga.Chapters {
		title := sanitizeTitle(c.Title)
		if mangahubNumberTitleRe.MatchString(title) {
			title = ""
		}
		chapters = append(chapters, &MangaHubChapter{
			Chapter: Chapter{
				Title:  title,
				Number: c.Number,
				Label:  Label{Style: LabelNumbered},
			},
			source: manga.source,
			slug:   manga.slug,
			key:    manga.key,
		})
	}

	return chapters, nil
}

// FetchChapter fetches the pages of a chapter from the API
func (m *MangaHub) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaHubChapter)

	var data struct {
		Chapter *struct {
			Pages string `json:"pages"`
		} `json:"chapter"`
	}
	err := m.query(mchap.key, fmt.Sprintf(
		"{ chapter(x: %s, slug: %s, number: %s) { pages } }",
		mchap.source, strconv.Quote(mchap.slug), strconv.FormatFloat(mchap.Number, 'f', -1, 64),
	), &data)
	if err != nil {
		return nil, err
	}
	if data.Chapter == nil {
		return nil, errors.New("the API has no such chapter")
	}

	images, err := mangahubPages(data.Chapter.Pages)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      mchap.Title,
		Label:      mchap.Label,
		Number:     mchap.Number,
		PagesCount: int64(len(images)),
		Language:   "en",
	}
	for i, img := range images {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img})
	}

	return chapter, nil
}

// fetchManga returns the series from the API, once per grabber
func (m *MangaHub) fetchManga() (*mangahubManga, error) {
	if m.manga != nil {
		return m.manga, nil
	}

	m.resolveSeriesURL()
	slug, err := m.slug()
	if err != nil {
		return nil, err
	}
	source, key, err := m.siteCode()
	if err != nil {
		return nil, err
	}

	var data struct {
		Manga *mangahubManga `json:"manga"`
	}
	err = m.query(key, fmt.Sprintf("{ manga(x: %s, slug: %s) { title, chapters { number, title } } }", source, strconv.Quote(slug)), &data)
	if err != nil {
		return nil, err
	}
	if data.Manga == nil {
		return nil, fmt.Errorf("the API has no series %q", slug)
	}
	data.Manga.source = source
	data.Manga.slug = slug
	data.Manga.key = key
	m.manga = data.Manga

	return m.manga, nil
}

// siteCode returns the code the API keys this site's data by, the dataSourceKey
// of its entry in the client bundle the series page loads, and the access key
// that same page issued as the mhub_access cookie the API expects in a header
func (m *MangaHub) siteCode() (code, key string, err error) {
	page, err := http.GetText(http.RequestParams{URL: m.URL})
	if err != nil {
		return "", "", err
	}
	base, _ := url.Parse(m.URL)

	key = http.SessionCookie(base.Hostname(), "mhub_access")
	if key == "" {
		return "", "", errors.New("the series page issued no mhub_access cookie")
	}

	match := mangahubClientRe.FindStringSubmatch(page)
	if match == nil {
		return "", "", errors.New("could not find the client bundle in the series page")
	}
	ref, err := url.Parse(match[1])
	if err != nil {
		return "", "", err
	}
	bundle, err := http.GetText(http.RequestParams{URL: base.ResolveReference(ref).String(), Referer: m.URL})
	if err != nil {
		return "", "", err
	}

	host := strings.TrimPrefix(base.Hostname(), "www.")
	entry := regexp.MustCompile(`domain:"` + regexp.QuoteMeta(host) + `",[^}]*dataSourceKey:"([a-z0-9]+)"`)
	site := entry.FindStringSubmatch(bundle)
	if site == nil {
		return "", "", fmt.Errorf("no site code for %s in its client bundle", host)
	}

	return site[1], key, nil
}

// query runs one GraphQL query with the access key and decodes its data into out
func (m *MangaHub) query(key, q string, out any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     mangahubAPI + "?" + url.Values{"query": {q}}.Encode(),
		Accept:  "application/json",
		Headers: map[string]string{"x-mhub-access": key},
	})
	if err != nil {
		return err
	}

	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return fmt.Errorf("mangahub API: %s", resp.Errors[0].Message)
	}

	return json.Unmarshal(resp.Data, out)
}

// slug returns the series slug of the series URL
func (m *MangaHub) slug() (string, error) {
	match := mangahubSlugRe.FindStringSubmatch(m.URL)
	if match == nil {
		return "", fmt.Errorf("could not find the series slug in url %s", m.URL)
	}

	return match[1], nil
}

// mangahubPages returns the image URLs of a chapter from its page list, a JSON
// object holding a path prefix ("p") and the file names under it ("i")
func mangahubPages(raw string) ([]string, error) {
	if strings.HasPrefix(raw, "enc:") {
		return nil, errors.New("the site encrypts its pages, which isn't supported yet")
	}

	var list struct {
		Prefix string   `json:"p"`
		Images []string `json:"i"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("could not read the page list: %w", err)
	}
	if len(list.Images) == 0 {
		return nil, errors.New("the chapter has no pages")
	}

	pages := make([]string, 0, len(list.Images))
	for _, img := range list.Images {
		pages = append(pages, mangahubCDN+strings.TrimPrefix(list.Prefix, "/")+img)
	}

	return pages, nil
}
