// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// globalcomixAPI is a var so tests can point it at a local server
var globalcomixAPI = "https://api.globalcomix.com"

const (
	// globalcomixAPIKey is the fallback for the public key the site's own web
	// reader sends in X-Gc-Client; the API answers CLIENT_HEADER_MISSING without it
	globalcomixAPIKey = "gck_b4d492261ec541eda44ce41de79da424"
	// globalcomixAuthCookie is the per-release cookie readV3 sets, which the
	// page CDN demands (401 without it)
	globalcomixAuthCookie = "gc_reader_auth"
	// globalcomixMaxResponseSize bounds an API response (a long series' listing
	// is a few MB)
	globalcomixMaxResponseSize = 64 << 20
)

var (
	globalcomixHostRe    = regexp.MustCompile(`(?i)^https?://(?:[a-z0-9-]+\.)*globalcomix\.com(?:[/?#]|$)`)
	globalcomixSeriesRe  = regexp.MustCompile(`(?i)^/c/([^/?#]+)`)
	globalcomixReaderRe  = regexp.MustCompile(`(?i)^/read/([0-9a-f-]{36})`)
	globalcomixCDNHostRe = regexp.MustCompile(`^https://[a-z0-9.-]*globalcomix\.com(?:/|$)`)
	globalcomixKeyRe     = regexp.MustCompile(`"api_key":"(gck_[A-Za-z0-9]+)"`)
	errGlobalcomixSeries = errors.New("could not find the series in the URL")
)

// Globalcomix is a grabber for globalcomix.com, a licensed comics reader: a SPA over a
// JSON API whose readV3 grants only the pages the viewer may read (a paid issue
// gets its preview).
type Globalcomix struct {
	*Grabber
	client *http.Client
	mu     sync.Mutex
	// key is the API key read off the page, fetched once under keyMu
	keyMu sync.Mutex
	key   string
	// comic is the series, fetched once for FetchTitle, FetchChapters and
	// LanguageOptions
	comic *globalcomixComic
}

func NewGlobalcomix(g *Grabber) *Globalcomix {
	return &Globalcomix{Grabber: g, client: &http.Client{Timeout: 60 * time.Second}}
}

// GlobalcomixChapter represents a Globalcomix issue
type GlobalcomixChapter struct {
	Chapter
	Key string
}

// globalcomixComic is the series as /v1/read/{slug} returns it
type globalcomixComic struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Lang  string `json:"lang"`
	Langs []struct {
		LangID string `json:"lang_id"`
		Name   string `json:"name"`
	} `json:"comic_langs"`
}

// globalcomixRelease is one entry of a series' release listing
type globalcomixRelease struct {
	Key         string `json:"key"`
	LangID      string `json:"lang_id"`
	ChapterType string `json:"chapter_type"`
	Chapter     string `json:"chapter"`
	Order       int    `json:"order"`
	Title       string `json:"title"`
	PageCount   int    `json:"page_count"`
	IsFree      bool   `json:"is_free"`
}

// globalcomixCDN is the reader_cdn_access block of a readV3 response
type globalcomixCDN struct {
	BaseURL     string   `json:"base_url"`
	ReleaseKey  string   `json:"release_key"`
	URLTemplate string   `json:"url_template"`
	Ranges      [][2]int `json:"ranges"`
}

// Test returns true for globalcomix.com URLs
func (g *Globalcomix) Test() (bool, error) {
	return globalcomixHostRe.MatchString(g.URL), nil
}

// apiKey returns the key the page embeds as window.gc.global.api_key, so a
// rotation doesn't break the grabber; the constant stands in if it can't be read.
func (g *Globalcomix) apiKey() string {
	g.keyMu.Lock()
	defer g.keyMu.Unlock()

	if g.key != "" {
		return g.key
	}
	g.key = globalcomixAPIKey

	resp, err := g.client.Get(g.URL)
	if err != nil {
		return g.key
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return g.key
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, globalcomixMaxResponseSize))
	if err != nil {
		return g.key
	}
	if m := globalcomixKeyRe.FindSubmatch(body); m != nil {
		g.key = string(m[1])
	}

	return g.key
}

// get fetches an API path and returns the payload's results. The reader's
// auth cookie, when the response sets one, is returned apart: it belongs to
// that release alone, so it can't live in the shared session.
func (g *Globalcomix) get(path string) (results json.RawMessage, cookie string, err error) {
	req, err := http.NewRequest("GET", globalcomixAPI+path, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	req.Header.Set("X-Gc-Client", g.apiKey())
	req.Header.Set("X-Gc-Identmode", "cookie")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, globalcomixMaxResponseSize))
	if err != nil {
		return nil, "", err
	}

	envelope := struct {
		Payload struct {
			Results json.RawMessage `json:"results"`
		} `json:"payload"`
		Error string `json:"error"`
	}{}
	if err = json.Unmarshal(body, &envelope); err != nil {
		return nil, "", fmt.Errorf("received %d response code", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || envelope.Error != "" {
		return nil, "", fmt.Errorf("the Globalcomix API answered %d %s for %s", resp.StatusCode, envelope.Error, path)
	}

	for _, c := range resp.Cookies() {
		if c.Name == globalcomixAuthCookie && c.Value != "" {
			cookie = c.Value
		}
	}

	return envelope.Payload.Results, cookie, nil
}

// seriesSlug derives the series slug from the URL: a series, issue or chapter
// link carries it, a /read/{key} link only after asking the API.
func (g *Globalcomix) seriesSlug() (string, error) {
	u, err := url.Parse(g.URL)
	if err != nil {
		return "", err
	}

	if m := globalcomixSeriesRe.FindStringSubmatch(u.Path); m != nil {
		slug, err := url.PathUnescape(m[1])
		if err != nil {
			return "", err
		}
		return slug, nil
	}

	if m := globalcomixReaderRe.FindStringSubmatch(u.Path); m != nil {
		results, _, err := g.get("/v1/readV3/" + m[1])
		if err != nil {
			return "", err
		}
		release := struct {
			ComicSlug string `json:"comic_slug"`
		}{}
		if err = json.Unmarshal(results, &release); err != nil || release.ComicSlug == "" {
			return "", errGlobalcomixSeries
		}
		return release.ComicSlug, nil
	}

	return "", errGlobalcomixSeries
}

// fetchComic returns the series, fetched once
func (g *Globalcomix) fetchComic() (*globalcomixComic, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.comic != nil {
		return g.comic, nil
	}

	slug, err := g.seriesSlug()
	if err != nil {
		return nil, err
	}
	results, _, err := g.get("/v1/read/" + url.PathEscape(slug))
	if err != nil {
		return nil, err
	}
	comic := &globalcomixComic{}
	if err = json.Unmarshal(results, comic); err != nil {
		return nil, err
	}
	if comic.ID == 0 {
		return nil, errGlobalcomixSeries
	}
	g.comic = comic

	return comic, nil
}

// FetchTitle fetches and returns the series title
func (g *Globalcomix) FetchTitle() (string, error) {
	comic, err := g.fetchComic()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(comic.Name), nil
}

// LanguageOptions lists the languages the series is published in
func (g *Globalcomix) LanguageOptions() FilterOptions {
	comic, err := g.fetchComic()
	if err != nil {
		return FilterOptions{}
	}

	var values []string
	for _, l := range comic.Langs {
		values = append(values, l.LangID)
	}

	options := FilterOptions{Values: uniqueValues(values), All: true}
	if langs := g.languages(comic); len(langs) == 1 {
		options.Current = langs[0]
	}

	return options
}

// requestedLanguage is the --language one, else the ?lang_id= of the URL
func (g *Globalcomix) requestedLanguage() string {
	if lang := g.Settings.Language; lang != "" {
		return lang
	}
	if u, err := url.Parse(g.URL); err == nil {
		return u.Query().Get("lang_id")
	}

	return ""
}

// languages returns the lang_ids to list: the requested one, else the series' own
func (g *Globalcomix) languages(comic *globalcomixComic) []string {
	lang := g.requestedLanguage()
	switch lang {
	case "":
		return []string{comic.Lang}
	case FilterAll:
		var all []string
		for _, l := range comic.Langs {
			all = append(all, l.LangID)
		}
		return uniqueValues(all)
	}

	return []string{lang}
}

// FetchChapters returns the series' issues; the ones the viewer can't read in
// full (paid) are listed as Locked
func (g *Globalcomix) FetchChapters() (chapters Filterables, errs []error) {
	comic, err := g.fetchComic()
	if err != nil {
		return nil, []error{err}
	}

	if lang := g.requestedLanguage(); lang != "" && lang != FilterAll {
		known := false
		var available []string
		for _, l := range comic.Langs {
			available = append(available, l.LangID)
			known = known || l.LangID == lang
		}
		if !known {
			return nil, []error{&UnavailableFilterError{Filter: "language", Value: lang, Available: uniqueValues(available)}}
		}
	}

	for _, lang := range g.languages(comic) {
		results, _, err := g.get(fmt.Sprintf("/v1/comics/%d/releases?lang_id=%s", comic.ID, url.QueryEscape(lang)))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		releases := []globalcomixRelease{}
		if err = json.Unmarshal(results, &releases); err != nil {
			errs = append(errs, err)
			continue
		}
		chapters = append(chapters, globalcomixChapters(releases, lang)...)
	}

	return chapters, errs
}

// globalcomixChapters turns a listing into chapters. The API only says a
// release's type and number: a numbered collected edition ("v", "o") is taken
// to repeat the issues, so it's dropped when the series has issues ("c"),
// whereas an unnumbered one (a one-shot, a bonus) can't be told apart and stays.
// Unnumbered releases get the previous number plus a hundredth, so they never
// collide with a real issue.
func globalcomixChapters(releases []globalcomixRelease, lang string) (chapters Filterables) {
	sort.SliceStable(releases, func(i, j int) bool { return releases[i].Order < releases[j].Order })

	hasIssues := false
	for _, r := range releases {
		hasIssues = hasIssues || r.ChapterType == "c"
	}

	var previous float64
	var unnumbered int
	for _, r := range releases {
		number, err := strconv.ParseFloat(strings.TrimSpace(r.Chapter), 64)
		numbered := err == nil
		if r.Key == "" || (hasIssues && r.ChapterType != "c" && numbered) {
			continue
		}
		if numbered {
			previous, unnumbered = number, 0
		} else {
			unnumbered++
			number = previous + float64(unnumbered)/100
		}
		chapters = append(chapters, &GlobalcomixChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    sanitizeTitle(r.Title),
				Language: lang,
				ID:       r.Key,
				Locked:   !r.IsFree,
			},
			Key: r.Key,
		})
	}

	return chapters
}

// FetchChapter fetches an issue and its pages
func (g *Globalcomix) FetchChapter(f Filterable) (*Chapter, error) {
	gchap := f.(*GlobalcomixChapter)

	results, cookie, err := g.get("/v1/readV3/" + url.PathEscape(gchap.Key))
	if err != nil {
		return nil, err
	}
	release := struct {
		IsFree    bool            `json:"is_free"`
		PageCount int             `json:"page_count"`
		CDN       *globalcomixCDN `json:"reader_cdn_access"`
	}{}
	if err = json.Unmarshal(results, &release); err != nil {
		return nil, err
	}

	chapter, err := globalcomixPages(gchap, release.PageCount, release.CDN, cookie)
	if err != nil {
		return nil, err
	}
	if !release.IsFree {
		return nil, globalcomixLockedError(gchap)
	}

	return chapter, nil
}

func globalcomixLockedError(c *GlobalcomixChapter) error {
	return fmt.Errorf("issue %s is paid, only its preview is readable", strconv.FormatFloat(c.Number, 'f', -1, 64))
}

// globalcomixPages builds the chapter from the CDN grant. The grant's ranges
// are the pages the viewer may read: fewer than the issue has means a preview,
// which is never returned as if it were the issue.
func globalcomixPages(gchap *GlobalcomixChapter, pageCount int, cdn *globalcomixCDN, cookie string) (*Chapter, error) {
	if cdn == nil || cdn.URLTemplate == "" || !globalcomixCDNHostRe.MatchString(cdn.BaseURL) {
		return nil, errors.New("the reader returned no page CDN")
	}

	chapter := &Chapter{
		Title:    gchap.GetTitle(),
		Number:   gchap.GetNumber(),
		Language: gchap.Language,
		ID:       gchap.ID,
	}

	base := strings.TrimRight(cdn.BaseURL, "/")
	for order := 1; order <= pageCount; order++ {
		if !globalcomixReadable(order, cdn.Ranges) {
			continue
		}
		pageURL := strings.NewReplacer(
			"{base_url}", base,
			"{release_key}", url.PathEscape(cdn.ReleaseKey),
			"{order}", strconv.Itoa(order),
			"{quality}", "desktop",
		).Replace(cdn.URLTemplate)
		page := Page{Number: int64(order), URL: pageURL}
		if cookie != "" {
			page.Headers = map[string]string{"Cookie": globalcomixAuthCookie + "=" + cookie}
		}
		chapter.Pages = append(chapter.Pages, page)
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	if pageCount == 0 || len(chapter.Pages) < pageCount {
		return nil, globalcomixLockedError(gchap)
	}

	return chapter, nil
}

func globalcomixReadable(order int, ranges [][2]int) bool {
	for _, r := range ranges {
		if order >= r[0] && order <= r[1] {
			return true
		}
	}

	return false
}
