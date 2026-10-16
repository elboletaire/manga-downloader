// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

// mangadexAPI is the API's base URL (a var so tests can point it at a local
// server)
var mangadexAPI = "https://api.mangadex.org"

var (
	// mangadexChapterURLRe matches chapter URLs like
	// https://mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79
	// www. and UUID case are handled inline via (?:www\.)? and (?i).
	mangadexChapterURLRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?mangadex\.org/chapter/([a-f0-9-]+)`)
)

// Mangadex is a grabber for mangadex.org
type Mangadex struct {
	*Grabber
	title string
	// manga caches the manga endpoint, read for the title and the languages
	manga *mangadexManga
	// readable and external track, per language, the chapters the feed listed
	// that can be downloaded and the first external URL of those that can't
	readable map[string]bool
	external map[string]string
	// unfiltered is set once the feed was read for every language
	unfiltered bool
	// rateLimiter rate limiter for the FetchChapter method. This call uses the '/at-home' endpoint which has a rate limit
	// of 40 calls per minute, if we exceed this limit we get a 429, and the consequent chapters fail. This may eventually
	// lead to an IP ban.
	rateLimiter <-chan time.Time
}

func NewMangadex(g *Grabber) *Mangadex {
	// we set the rate limit at 39 calls per minute instead of 40 to make sure the rate limit is under the threshold,
	// otherwise we occasionally get hit by the rate limiter.
	return &Mangadex{Grabber: g, rateLimiter: time.Tick(time.Minute / 39)}
}

// mangadexPagesPerSecond stays well under the image servers' ~1000 requests
// a minute per-IP limit (Cloudflare 1015), leaving room for retries and a
// reader open on the same network.
const mangadexPagesPerSecond = 8

// mangadexMaxConcurrency caps the user's concurrency: the pacer already
// keeps the pace, so more parallel downloads only queue bursts on it.
var mangadexMaxConcurrency = MaxConcurrency{Chapters: 2, Pages: 5}

// mangadexPagePacer spaces page requests evenly. It's shared by every
// Mangadex instance because the limit is per IP, not per download.
var mangadexPagePacer = &pacer{interval: time.Second / mangadexPagesPerSecond}

// pacer hands out request slots at most one per interval
type pacer struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// wait blocks until the caller's slot comes up
func (p *pacer) wait() {
	p.mu.Lock()
	now := time.Now()
	slot := p.next
	if slot.Before(now) {
		slot = now
	}
	p.next = slot.Add(p.interval)
	p.mu.Unlock()
	time.Sleep(time.Until(slot))
}

// GetMaxConcurrency caps the configured concurrency at what the image
// servers tolerate. Lower settings are kept; higher ones are clamped, since
// the limit is the server's and going over it only gets pages refused.
func (m *Mangadex) GetMaxConcurrency() MaxConcurrency {
	mc := m.Grabber.GetMaxConcurrency()
	mc.Chapters = min(mc.Chapters, mangadexMaxConcurrency.Chapters)
	mc.Pages = min(mc.Pages, mangadexMaxConcurrency.Pages)
	return mc
}

// WaitPage paces page downloads to the image servers' rate limit
// (PagePacer)
func (m *Mangadex) WaitPage() {
	mangadexPagePacer.wait()
}

// mangadexChapterURL checks if the URL is a chapter URL (not a series URL).
func mangadexChapterURL(url string) bool {
	return mangadexChapterURLRe.MatchString(url)
}

// mangadexSeriesURL extracts the series URL from a chapter UUID by fetching
// the chapter endpoint and extracting the manga ID from relationships.
func mangadexSeriesURL(chapterUUID, chapterURL string) (string, error) {
	rbody, err := http.Get(http.RequestParams{
		URL: mangadexAPI + "/chapter/" + chapterUUID,
	})
	if err != nil {
		return "", err
	}
	defer rbody.Close()

	var resp mangadexChapterResponse
	if err = json.NewDecoder(rbody).Decode(&resp); err != nil {
		return "", err
	}

	// Find the manga relationship
	for _, rel := range resp.Data.Relationships {
		if rel.Type == "manga" {
			return fmt.Sprintf("https://mangadex.org/title/%s", rel.Id), nil
		}
	}

	return "", errSeriesNotFound(chapterURL)
}

// resolveSeriesURL resolves a chapter URL to the series URL if needed.
func (m *Mangadex) resolveSeriesURL() error {
	if !mangadexChapterURL(m.URL) {
		return nil
	}

	matches := mangadexChapterURLRe.FindStringSubmatch(m.URL)
	if len(matches) < 2 {
		return fmt.Errorf("could not extract chapter UUID from %s", m.URL)
	}
	chapterUUID := strings.ToLower(matches[1])

	seriesURL, err := mangadexSeriesURL(chapterUUID, m.URL)
	if err != nil {
		return err
	}

	m.URL = seriesURL
	return nil
}

// MangadexChapter represents a MangaDex Chapter
type MangadexChapter struct {
	Chapter
	Id string
}

// Test checks if the site is MangaDex
func (m *Mangadex) Test() (bool, error) {
	re := regexp.MustCompile(`mangadex\.org`)
	return re.MatchString(m.URL), nil
}

// FetchTitle returns the title of the manga
func (m *Mangadex) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	if err := m.resolveSeriesURL(); err != nil {
		return "", err
	}

	body, err := m.fetchManga()
	if err != nil {
		return "", err
	}

	// fetch the title in the requested language
	if m.Settings.Language != "" {
		trans := body.Data.Attributes.AltTitles.GetTitleByLang(m.Settings.Language)

		if trans != "" {
			m.title = trans
			return m.title, nil
		}
	}

	// fallback to english or, if not present, any other title
	if title, ok := body.Data.Attributes.Title["en"]; ok {
		m.title = title
	} else {
		for _, title := range body.Data.Attributes.Title {
			m.title = title
			break
		}
	}

	return m.title, nil
}

// fetchManga returns the manga endpoint's data, fetched once: FetchTitle and
// LanguageOptions both read it
func (m *Mangadex) fetchManga() (*mangadexManga, error) {
	if m.manga != nil {
		return m.manga, nil
	}

	// the API only knows lowercase ids
	id := strings.ToLower(getUuid(m.URL))

	rbody, err := http.Get(http.RequestParams{
		URL:     mangadexAPI + "/manga/" + id,
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()

	// decode json response
	body := &mangadexManga{}
	if err = json.NewDecoder(rbody).Decode(body); err != nil {
		return nil, err
	}
	m.manga = body

	return body, nil
}

// LanguageOptions lists the languages the series is translated into, minus
// those whose chapters are all external. Only a complete all-languages listing
// can vouch for the rest; a filtered one only rules out its own language.
func (m *Mangadex) LanguageOptions() FilterOptions {
	body, err := m.fetchManga()
	if err != nil {
		return FilterOptions{}
	}

	var values []string
	for _, lang := range uniqueValues(body.Data.Attributes.AvailableTranslatedLanguages) {
		if _, ext := m.external[lang]; ext && !m.readable[lang] {
			continue
		}
		if m.unfiltered && !m.readable[lang] {
			continue
		}
		values = append(values, lang)
	}

	return FilterOptions{Values: values, All: true}
}

// mangadexExternalError reports a language whose chapters all live on another
// site, which there is nothing to download from here.
type mangadexExternalError struct {
	Language string
	URL      string
}

func (e *mangadexExternalError) Error() string {
	what := "this title's"
	if e.Language != FilterAll {
		what = fmt.Sprintf("the %q chapters of this title are", e.Language)
	} else {
		what += " chapters are"
	}
	if strings.Contains(e.URL, "mangaplus.shueisha.co.jp") {
		return fmt.Sprintf("%s hosted on MANGA Plus, which is supported: try %s", what, e.URL)
	}

	return fmt.Sprintf("%s hosted elsewhere: %s", what, e.URL)
}

// FetchChapters returns the chapters of the manga
func (m *Mangadex) FetchChapters() (chapters Filterables, errs []error) {
	if err := m.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	// the API only knows lowercase ids
	id := strings.ToLower(getUuid(m.URL))

	filter := m.Settings.Language
	if filter == FilterAll {
		filter = ""
	}
	m.unfiltered = false
	m.readable, m.external = map[string]bool{}, map[string]string{}
	var firstExternal string

	baseOffset := 500
	var fetchChaps func(int)

	fetchChaps = func(offset int) {
		uri := fmt.Sprintf("%s/manga/%s/feed", mangadexAPI, id)
		params := url.Values{}
		params.Add("limit", fmt.Sprint(baseOffset))
		params.Add("order[volume]", "asc")
		params.Add("order[chapter]", "asc")
		params.Add("offset", fmt.Sprint(offset))
		// the groups come with the chapters, as relationships
		params.Add("includes[]", "scanlation_group")
		if lang := m.Settings.Language; lang != "" && lang != FilterAll {
			params.Add("translatedLanguage[]", lang)
		}
		uri = fmt.Sprintf("%s?%s", uri, params.Encode())

		rbody, err := http.Get(http.RequestParams{URL: uri})
		if err != nil {
			errs = append(errs, err)
			return
		}
		defer rbody.Close()
		// parse json body
		body := mangadexFeed{}
		if err = json.NewDecoder(rbody).Decode(&body); err != nil {
			errs = append(errs, err)
			return
		}

		for _, c := range body.Data {
			// skip chapters hosted outside mangadex (official publisher
			// sites): there's nothing to download from them. They usually
			// report 0 pages, but some stubs still claim a page count, so
			// check the external url too.
			if c.Attributes.Pages == 0 || c.Attributes.ExternalUrl != "" {
				lang := c.Attributes.TranslatedLanguage
				if _, ok := m.external[lang]; !ok && c.Attributes.ExternalUrl != "" {
					m.external[lang] = c.Attributes.ExternalUrl
				}
				continue
			}
			m.readable[c.Attributes.TranslatedLanguage] = true
			num, _ := strconv.ParseFloat(c.Attributes.Chapter, 64)
			listed := &MangadexChapter{
				Chapter{
					Number:     num,
					Title:      c.Attributes.Title,
					Language:   c.Attributes.TranslatedLanguage,
					PagesCount: c.Attributes.Pages,
					Volume:     parseVolume(c.Attributes.Volume),
					ID:         c.Id,
				},
				c.Id,
			}
			listed.SetGroups(mangadexGroups(c.Relationships)...)
			chapters = append(chapters, listed)
		}

		if len(body.Data) > 0 {
			fetchChaps(offset + baseOffset)
		}
	}
	// initial call
	fetchChaps(0)
	// a partial listing can't vouch for which languages have nothing readable
	m.unfiltered = filter == "" && len(errs) == 0

	if len(chapters) == 0 && len(errs) == 0 {
		if filter != "" {
			firstExternal = m.external[filter]
		} else {
			for _, u := range m.external {
				firstExternal = u
				break
			}
		}
		if firstExternal != "" {
			lang := filter
			if lang == "" {
				lang = FilterAll
			}
			errs = append(errs, &mangadexExternalError{Language: lang, URL: firstExternal})
		}
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (m *Mangadex) FetchChapter(f Filterable) (*Chapter, error) {
	<-m.rateLimiter
	chap := f.(*MangadexChapter)
	// download json
	rbody, err := http.Get(http.RequestParams{
		URL: mangadexAPI + "/at-home/server/" + chap.Id,
	})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()
	// parse json body
	body := mangadexPagesFeed{}
	if err = json.NewDecoder(rbody).Decode(&body); err != nil {
		return nil, err
	}

	pcount := len(body.Chapter.Data)

	chapter := &Chapter{
		Title:      chap.Title,
		Label:      Label{Style: LabelNumberFirst},
		Number:     f.GetNumber(),
		PagesCount: int64(pcount),
		Language:   chap.Language,
		ID:         chap.ID,
	}
	chapter.SetGroups(chap.GetGroups()...)

	// create pages
	for i, p := range body.Chapter.Data {
		num := i + 1
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(num),
			URL:    body.BaseUrl + path.Join("/data", body.Chapter.Hash, p),
		})
	}

	return chapter, nil
}

// mangadexManga represents the Manga json object
type mangadexManga struct {
	Id   string
	Data struct {
		Attributes struct {
			Title                        map[string]string
			AltTitles                    altTitles
			AvailableTranslatedLanguages []string
		}
	}
}

// altTitles is a slice of maps with the language as key and the title as value
type altTitles []map[string]string

// GetTitleByLang returns the title in the given language (or empty if string is not found)
func (a altTitles) GetTitleByLang(lang string) string {
	for _, t := range a {
		val, ok := t[lang]
		if ok {
			return val
		}
	}
	return ""
}

// mangadexFeed represents the json object returned by the feed endpoint
type mangadexFeed struct {
	Data []struct {
		Id         string
		Attributes struct {
			Volume             string
			Chapter            string
			Title              string
			TranslatedLanguage string
			Pages              int64
			ExternalUrl        string
		}
		Relationships []mangadexRelationship
	}
}

// mangadexRelationship is a linked resource of a chapter; the feed includes
// the scanlation groups (includes[]=scanlation_group) and the uploader
type mangadexRelationship struct {
	Type       string
	Attributes struct {
		Name string
	}
}

// mangadexGroups names the scanlation groups of a release, from its
// relationships. An upload by a user alone names none, so it names no group
// rather than the uploader.
func mangadexGroups(relationships []mangadexRelationship) []string {
	var names []string
	for _, rel := range relationships {
		if rel.Type == "scanlation_group" {
			names = append(names, rel.Attributes.Name)
		}
	}

	return names
}

// mangadexPagesFeed represents the json object returned by the pages endpoint
type mangadexPagesFeed struct {
	BaseUrl string
	Chapter struct {
		Hash      string
		Data      []string
		DataSaver []string
	}
}

// mangadexChapterResponse represents the json object returned by the chapter endpoint
type mangadexChapterResponse struct {
	Data struct {
		Id            string `json:"id"`
		Relationships []struct {
			Id   string `json:"id"`
			Type string `json:"type"`
		} `json:"relationships"`
	} `json:"data"`
}
