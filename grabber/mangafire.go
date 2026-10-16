// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/browser"
)

const mangafireBase = "https://mangafire.to"

// mangafireMaxChapterPages caps how many chapter-list pages the pager is
// clicked through. The list loads 20 chapters per page, so this covers series
// with a few thousand chapters (e.g. One Piece needs ~90) while still bounding
// a runaway loop if the end-of-list detection ever fails.
const mangafireMaxChapterPages = 400

// mangafireNextPageSelector advances the chapter-list pager one page. The
// "Next page" arrow only renders when there are more than ~5 pages, so short
// pagers (numbered buttons only) need the numbered button right after the
// active one instead; on long pagers the arrow covers the case where the
// active number is the last of its window. querySelector picks whichever
// matches first in document order — both advance exactly one page. On the
// last page neither matches (no numbered sibling, arrow gone/disabled), which
// ends the pagination loop.
const mangafireNextPageSelector = `.npager__num.is-active + .npager__num, .npager__nav[aria-label="Next page"]`

// Mangafire is a grabber for mangafire.to. The site is a react SPA whose JSON
// API signs every request with a per-session, Cloudflare-challenge-gated `vrf`
// token generated client-side, so the endpoints can no longer be called with
// plain HTTP (they answer 403 "Missing token"). Instead we drive a real
// (headless) browser: it renders the series/reader pages, makes the signed API
// calls itself, and we intercept the JSON responses. Page images then download
// over plain HTTP (a Referer header is enough, no cookies) reusing the harvested
// browser session.
type Mangafire struct {
	*Grabber
	// cached by load() after the first series render
	loaded     bool
	loadErr    error
	title      string
	readerBase string // canonical reader-URL base, e.g. https://mangafire.to/title/dkw-one-piece
	chapters   Filterables
	languages  FilterOptions // the title's languages, read from the dropdown by load
}

func NewMangafire(g *Grabber) *Mangafire {
	return &Mangafire{Grabber: g}
}

// MangafireChapter represents a Mangafire chapter
type MangafireChapter struct {
	Chapter
	// URL is the chapter reader page URL, rendered in a browser to capture the
	// signed pages-API response
	URL string
	// Id is the numeric chapter id
	Id int64
}

// mangafireChapterRe matches chapter reader URLs (/title/{slug}/chapter/{id}
// and the legacy /manga/{slug}/chapter/{id}). The capturing group is the
// series URL prefix.
var mangafireChapterRe = regexp.MustCompile(`(?i)^(https?://(?:www\.)?mangafire\.to/(?:title|manga)/[^/?#]+)/chapter/`)

// resolveSeriesURL rewrites m.URL to the series URL when it is a chapter
// reader URL. Series URLs are unchanged.
func (m *Mangafire) resolveSeriesURL() {
	if match := mangafireChapterRe.FindStringSubmatch(m.URL); match != nil {
		m.URL = match[1]
	}
}

// Test returns true if the URL is a mangafire.to series URL
func (m *Mangafire) Test() (bool, error) {
	re := regexp.MustCompile(`mangafire\.to/(title|manga)/`)
	return re.MatchString(m.URL), nil
}

// FetchTitle returns the manga title
func (m *Mangafire) FetchTitle() (string, error) {
	if err := m.load(); err != nil {
		return "", err
	}
	return m.title, nil
}

// LanguageOptions returns the languages the title offers, as load read them
// from its chapter list's dropdown
func (m *Mangafire) LanguageOptions() FilterOptions {
	return m.languages
}

// FetchChapters returns the chapters of the manga
func (m *Mangafire) FetchChapters() (Filterables, []error) {
	if err := m.load(); err != nil {
		return nil, []error{err}
	}
	return m.chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (m *Mangafire) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangafireChapter)

	// rendering the reader page makes the SPA call /api/chapters/{id} with a
	// valid vrf; we intercept that single response (which carries the full
	// pages list) instead of trying to sign the call ourselves
	responses, _, err := browser.GetAPIResponses(
		mchap.URL,
		".reader-img",
		fmt.Sprintf("/api/chapters/%d", mchap.Id),
		"",
		0,
		0,
		"",
	)
	if err != nil {
		return nil, err
	}

	var pages []mangafirePage
	for _, r := range responses {
		feed := struct {
			Data struct {
				Pages []mangafirePage `json:"pages"`
			} `json:"data"`
		}{}
		if err := json.Unmarshal([]byte(r.Body), &feed); err != nil {
			continue
		}
		if len(feed.Data.Pages) > 0 {
			pages = feed.Data.Pages
			break
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for chapter %s (it may be premium/locked)", ChapterLabel(f))
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		Language:   mchap.Language,
		ID:         mchap.ID,
		PagesCount: int64(len(pages)),
	}
	chapter.SetGroups(mchap.GetGroups()...)
	for i, p := range pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    p.Url,
		})
	}

	return chapter, nil
}

// load renders the series page once, paginating through the whole chapter list,
// and caches the title, the canonical reader-URL base and the chapters. Both
// FetchTitle and FetchChapters go through it, so the (slow) browser render only
// happens once per run.
func (m *Mangafire) load() error {
	if m.loaded {
		return m.loadErr
	}
	m.loaded = true
	m.resolveSeriesURL()

	hid, err := m.hid()
	if err != nil {
		m.loadErr = err
		return err
	}

	// the language dropdown is only in the DOM once it's opened, so it's read
	// in this same render, which also picks the requested language. An unknown
	// language is only an error once the title's own list is known.
	lang, langErr := mangafireLanguage(m.Settings.Language)

	// urlSubstr matches the title (/api/titles/{hid}?...), the volumes list and
	// every chapters page (/api/titles/{hid}/chapters?...); they're told apart
	// below by their exact URL
	responses, dropdownJSON, err := browser.GetAPIResponses(
		m.URL,
		".title-detail__row-link",
		"/api/titles/"+hid,
		mangafireNextPageSelector,
		mangafireMaxChapterPages,
		0,
		mangafireDropdownJS(lang.label),
	)
	if err != nil {
		m.loadErr = fmt.Errorf("error fetching chapters: %w", err)
		return m.loadErr
	}

	dropdown, err := parseMangafireDropdown(dropdownJSON)
	if err != nil {
		m.loadErr = err
		return err
	}
	m.languages = mangafireOptions(dropdown)
	if langErr != nil || (lang.code != "" && !dropdown.offers(lang.label)) {
		m.loadErr = m.unavailableLanguage(m.Settings.Language)
		return m.loadErr
	}

	m.readerBase = mangafireBase + "/title/" + hid // fallback if the title call is missed
	titlePrefix := "/api/titles/" + hid + "?"
	chaptersPrefix := "/api/titles/" + hid + "/chapters"

	// first pass: the title-info response gives the clean title and the
	// canonical reader-URL base
	for _, r := range responses {
		if !strings.Contains(r.URL, titlePrefix) {
			continue
		}
		info := struct {
			Data struct {
				Title string `json:"title"`
				URL   string `json:"url"`
			} `json:"data"`
		}{}
		if err := json.Unmarshal([]byte(r.Body), &info); err != nil {
			continue
		}
		if info.Data.Title != "" {
			m.title = info.Data.Title
		}
		if info.Data.URL != "" {
			m.readerBase = mangafireBase + info.Data.URL
		}
		break
	}

	// second pass: collect chapters from every captured feed page
	// the site's default language list is also captured before switching, so
	// only the requested one is kept
	feedPrefix := chaptersPrefix
	if lang.code != "" {
		feedPrefix += "?language=" + lang.code + "&"
	}
	for _, c := range parseMangafireChapters(responses, feedPrefix) {
		listed := &MangafireChapter{
			Chapter: Chapter{
				Number:   c.Number,
				Title:    c.Name,
				Language: c.Language,
				ID:       strconv.FormatInt(c.Id, 10),
				Label:    Label{Style: LabelNumbered},
			},
			URL: fmt.Sprintf("%s/chapter/%d", m.readerBase, c.Id),
			Id:  c.Id,
		}
		listed.SetGroups(mangafireGroup(c.Type))
		m.chapters = append(m.chapters, listed)
	}

	if len(m.chapters) == 0 {
		if lang.code != "" {
			m.loadErr = fmt.Errorf("no chapters found for %s in language %q", m.URL, m.Settings.Language)
		} else {
			m.loadErr = fmt.Errorf("no chapters found for %s", m.URL)
		}
	}

	return m.loadErr
}

// hid returns the title id from the URL, e.g. "dkw" for both the current
// https://mangafire.to/title/dkw-one-piece format and the legacy
// https://mangafire.to/manga/one-piecee.dkw one
func (m *Mangafire) hid() (string, error) {
	re := regexp.MustCompile(`/title/([^/-]+)-`)
	if matches := re.FindStringSubmatch(m.URL); len(matches) == 2 {
		return matches[1], nil
	}
	re = regexp.MustCompile(`/manga/[^/]+\.([^/.]+)`)
	if matches := re.FindStringSubmatch(m.URL); len(matches) == 2 {
		return matches[1], nil
	}
	return "", fmt.Errorf("could not find title id in url %s", m.URL)
}

// parseMangafireChapters collects the chapter items out of the captured API
// responses whose URL matches chaptersPrefix (one response per chapter-list
// page). The api returns official and unofficial uploads of the same number as
// separate items, and every one is kept: each is a release of its own. Items
// keep their first-seen order, which is the site's own list order.
func parseMangafireChapters(responses []browser.APIResponse, chaptersPrefix string) []mangafireChapterItem {
	var out []mangafireChapterItem
	for _, r := range responses {
		if !strings.Contains(r.URL, chaptersPrefix) {
			continue
		}
		feed := struct {
			Items []mangafireChapterItem `json:"items"`
		}{}
		if err := json.Unmarshal([]byte(r.Body), &feed); err != nil {
			continue
		}
		out = append(out, feed.Items...)
	}
	return out
}

// mangafireGroup returns the group a chapter item names for its upload type.
// The listing has no group of its own, so the two kinds of release are the
// groups --scanlator picks between.
func mangafireGroup(uploadType string) string {
	switch uploadType {
	case "official":
		return "Official"
	case "unofficial":
		return "Unofficial"
	}
	return ""
}

// mangafireChapterItem is one entry of the chapters-list JSON feed
type mangafireChapterItem struct {
	Id       int64   `json:"id"`
	Number   float64 `json:"number"`
	Name     string  `json:"name"`
	Language string  `json:"language"`
	Type     string  `json:"type"` // "official" or "unofficial"
}

// mangafirePage is one entry of the reader's pages list
type mangafirePage struct {
	Url string `json:"url"`
}

// mangafireLang is a language of the chapter-list dropdown: the code the API
// takes as `language=` and the label (minus its flag emoji) the UI shows
type mangafireLang struct {
	code, label string
}

var mangafireLangs = []mangafireLang{
	{"en", "English"},
	{"es-la", "Spanish (LATAM)"},
	{"es", "Spanish"},
	{"fr", "French"},
	{"ja", "Japanese"},
	{"pt-br", "Portuguese (Br)"},
	{"pt", "Portuguese"},
}

// mangafireLangAliases maps what users type for --language to a site code:
// ISO-ish codes and the country-style ones (br, mx, jp) the other grabbers'
// help text hints at
var mangafireLangAliases = map[string]string{
	"br": "pt-br", "pt_br": "pt-br", "ptbr": "pt-br",
	"mx": "es-la", "es-mx": "es-la", "es_la": "es-la", "latam": "es-la",
	"jp": "ja",
}

// mangafireLanguage resolves the --language value to a site language. An empty
// value, or "all", yields the zero value (leave the site's default list alone).
func mangafireLanguage(v string) (mangafireLang, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == FilterAll {
		return mangafireLang{}, nil
	}
	if a, ok := mangafireLangAliases[v]; ok {
		v = a
	}
	codes := make([]string, 0, len(mangafireLangs))
	for _, l := range mangafireLangs {
		if l.code == v {
			return l, nil
		}
		codes = append(codes, l.code)
	}
	return mangafireLang{}, fmt.Errorf("unsupported language %q for mangafire (use one of: %s)", v, strings.Join(codes, ", "))
}

// mangafireDropdown is what the chapter list's language dropdown resolves to:
// the label shown before any pick, every label it offers, and the error when
// the selector isn't on the page
type mangafireDropdown struct {
	Error    string   `json:"error"`
	Selected string   `json:"selected"`
	Labels   []string `json:"labels"`
}

// offers reports whether the dropdown lists the label
func (d mangafireDropdown) offers(label string) bool {
	for _, l := range d.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// mangafireDropdownJS returns an async script that opens the chapter list's
// "Lang" dropdown and reads what it offers. If want is set and offered, it
// picks it, which makes the SPA refetch the list with a freshly signed call,
// and the script says so (wait), so the browser awaits that refetch. It
// resolves to the JSON browser.GetAPIResponses reads, with the dropdown as
// result.
func mangafireDropdownJS(want string) string {
	label, _ := json.Marshal(want)
	return `(async function(){
var want=` + string(label) + `;
var norm=function(s){return s.replace(/^[^A-Za-z]+/,"").trim()};
var btn=document.querySelector(".title-detail__toolbar .select");
if(!btn)return JSON.stringify({wait:false,result:{error:"language selector not found"}});
var selected=norm(btn.querySelector(".select__value").textContent);
btn.click();
await new Promise(function(r){setTimeout(r,500)});
var items=Array.from(document.querySelectorAll(".dropdown__item"));
var labels=items.map(function(e){return norm(e.textContent)});
var it=want&&want!==selected?items.find(function(e){return norm(e.textContent)===want}):null;
if(it)it.click();else btn.click();
return JSON.stringify({wait:it!==null,result:{selected:selected,labels:labels}});
})()`
}

// parseMangafireDropdown reads the dropdown browser.GetAPIResponses returned
func parseMangafireDropdown(raw json.RawMessage) (mangafireDropdown, error) {
	var d mangafireDropdown
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("could not read the language list: %w", err)
	}
	if d.Error != "" {
		return d, errors.New(d.Error)
	}
	return d, nil
}

// mangafireOptions turns the dropdown into the language options: the codes
// --language takes, as the site lists them, with the label shown by default as
// Current. A label without a code in mangafireLangs can't be typed, so it's
// left out.
func mangafireOptions(d mangafireDropdown) FilterOptions {
	var options FilterOptions
	for _, label := range d.Labels {
		if code := mangafireCodeOf(label); code != "" {
			options.Values = append(options.Values, code)
		}
	}
	options.Current = mangafireCodeOf(d.Selected)

	return options
}

// mangafireCodeOf returns the code of a dropdown label, "" when unknown
func mangafireCodeOf(label string) string {
	for _, l := range mangafireLangs {
		if l.label == label {
			return l.code
		}
	}
	return ""
}

// unavailableLanguage is the error for a --language the title doesn't offer,
// listing the codes it does
func (m *Mangafire) unavailableLanguage(v string) error {
	return &UnavailableFilterError{Filter: "language", Value: v, Available: m.languages.Values}
}
