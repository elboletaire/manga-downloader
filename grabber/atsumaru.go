// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"

	"github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
)

// atsumaruCDN hosts the page images. The api's image paths are the same ones
// the reader requests, but atsu.moe/static answers 410 Gone for them now.
const atsumaruCDN = "https://cdn.atsu.moe"

// Atsumaru is a grabber for atsu.moe: a react SPA whose JSON api is completely
// open to plain HTTP, no cookies/auth needed. The html isn't a bare shell
// either: the series page ships a server-rendered window.mangaPage blob, which
// is the only place the scanlation groups are actually named (see
// fetchScanlators)
type Atsumaru struct {
	*Grabber
	title string
	// chapters caches the raw api chapter list, keyed by manga id, so
	// FetchTitle and FetchChapters can share a single api call
	info *atsumaruMangaInfo
	// scanlators caches the scanId -> name mapping, which lives only in the
	// series page html; nil once fetched means the page shape changed
	scanlators     []atsumaruScanlator
	scanlatorsDone bool
}

func NewAtsumaru(g *Grabber) *Atsumaru {
	return &Atsumaru{Grabber: g}
}

// AtsumaruChapter represents an Atsumaru Chapter
type AtsumaruChapter struct {
	Chapter
	Id string
}

// atsumaruChapterRe matches reader URLs like /read/{mangaId}/{chapterId}
var atsumaruChapterRe = regexp.MustCompile(`(?i)^https?://(?:www\.)?atsu\.moe/read/([^/?#]+)/`)

// Test returns true if the URL is an atsu.moe series or reader URL
func (a *Atsumaru) Test() (bool, error) {
	re := regexp.MustCompile(`atsu\.moe/(manga|read)/`)
	return re.MatchString(a.URL), nil
}

// resolveSeriesURL rewrites a reader URL (/read/{id}/...) to the series URL
// (/manga/{id}). No-op for series URLs.
func (a *Atsumaru) resolveSeriesURL() {
	m := atsumaruChapterRe.FindStringSubmatch(a.URL)
	if m == nil {
		return
	}
	u, err := url.Parse(a.URL)
	if err != nil {
		return
	}
	u.Path = "/manga/" + m[1]
	u.RawQuery = ""
	u.Fragment = ""
	a.URL = u.String()
}

// FetchTitle fetches and returns the manga title
func (a *Atsumaru) FetchTitle() (string, error) {
	a.resolveSeriesURL()
	if a.title != "" {
		return a.title, nil
	}

	info, err := a.mangaInfo()
	if err != nil {
		return "", err
	}

	a.title = sanitizeTitle(info.Title)

	return a.title, nil
}

// FetchChapters returns the chapters of the manga: every group's release of
// each chapter, the group in Chapter.Group, so --scanlator picks among them
// (see grabber.SelectGroup)
func (a *Atsumaru) FetchChapters() (chapters Filterables, errs []error) {
	a.resolveSeriesURL()
	info, err := a.mangaInfo()
	if err != nil {
		return nil, []error{err}
	}

	names := map[string]string{}
	for _, s := range a.fetchScanlators() {
		names[s.Id] = s.displayName()
	}

	for _, c := range info.Chapters {
		// a group the series page doesn't name is listed by its id
		group := names[c.ScanId]
		if group == "" {
			group = c.ScanId
		}
		listed := &AtsumaruChapter{
			Chapter{
				Number: c.Number,
				Title:  c.Title,
				ID:     c.Id,
				Label:  Label{Style: LabelNumbered},
			},
			c.Id,
		}
		listed.SetGroups(group)
		chapters = append(chapters, listed)
	}

	return chapters, errs
}

// Refilterable implements grabber.Refilterable: the listing holds every group's
// releases and is cached on the site, so a group change is a filter on it
func (*Atsumaru) Refilterable() {}

// fetchScanlators returns the series' scanlation groups with their names. They
// aren't exposed by any api endpoint (the chapter list only carries opaque
// scanId cuids, and the site itself filters client-side off a localStorage
// key), but the server-rendered series page embeds them in a window.mangaPage
// blob. Failing to read it is not fatal: groups stay selectable by id.
func (a *Atsumaru) fetchScanlators() []atsumaruScanlator {
	if a.scanlatorsDone {
		return a.scanlators
	}
	a.scanlatorsDone = true

	body, err := http.Get(http.RequestParams{URL: a.URL})
	if err != nil {
		notice.Warn("could not fetch the series page to name the scanlation groups: %s", err.Error())
		return nil
	}
	defer body.Close()

	html, err := io.ReadAll(body)
	if err != nil {
		notice.Warn("could not read the series page to name the scanlation groups: %s", err.Error())
		return nil
	}

	raw, err := extractBalancedJSON(string(html), `"scanlators":`)
	if err != nil {
		notice.Warn("could not find the scanlation groups in the series page: %s", err.Error())
		return nil
	}

	scanlators := []atsumaruScanlator{}
	if err = json.Unmarshal([]byte(raw), &scanlators); err != nil {
		notice.Warn("could not parse the scanlation groups from the series page: %s", err.Error())
		return nil
	}

	a.scanlators = scanlators

	return a.scanlators
}

// displayName returns the group name, falling back to its id when the series
// page didn't name it
func (g atsumaruScanlator) displayName() string {
	if g.Name != "" {
		return g.Name
	}
	return g.Id
}

// FetchChapter fetches a chapter and its pages
func (a *Atsumaru) FetchChapter(f Filterable) (*Chapter, error) {
	achap := f.(*AtsumaruChapter)

	mangaId, err := a.mangaId()
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("https://atsu.moe/api/read/chapter?mangaId=%s&chapterId=%s", mangaId, achap.Id)
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: a.URL,
	})
	if err != nil {
		return nil, err
	}

	return atsumaruChapter(achap, body)
}

// atsumaruChapter builds the chapter of one release from its reader feed
func atsumaruChapter(achap *AtsumaruChapter, body string) (*Chapter, error) {
	feed := struct {
		ReadChapter struct {
			Pages []struct {
				Image string `json:"image"`
			} `json:"pages"`
		} `json:"readChapter"`
	}{}
	if err := json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      achap.GetTitle(),
		Label:      labelOf(achap),
		Number:     achap.GetNumber(),
		Language:   "en",
		ID:         achap.ID,
		PagesCount: int64(len(feed.ReadChapter.Pages)),
	}
	chapter.SetGroups(achap.GetGroups()...)
	for i, p := range feed.ReadChapter.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    atsumaruCDN + p.Image,
		})
	}

	return chapter, nil
}

// mangaId returns the manga id from the URL, e.g. "2VgNt" for
// https://atsu.moe/manga/2VgNt
func (a Atsumaru) mangaId() (string, error) {
	re := regexp.MustCompile(`/manga/([^/?#]+)`)
	matches := re.FindStringSubmatch(a.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find manga id in url %s", a.URL)
	}
	return matches[1], nil
}

// mangaInfo fetches (and caches) the manga info, which includes the title
// and the full chapters list (across every scanlation group) in one request
func (a *Atsumaru) mangaInfo() (*atsumaruMangaInfo, error) {
	if a.info != nil {
		return a.info, nil
	}

	mangaId, err := a.mangaId()
	if err != nil {
		return nil, err
	}

	body, err := http.GetText(http.RequestParams{
		URL:     "https://atsu.moe/api/manga/info?mangaId=" + mangaId,
		Referer: a.URL,
	})
	if err != nil {
		return nil, err
	}

	info := &atsumaruMangaInfo{}
	if err = json.Unmarshal([]byte(body), info); err != nil {
		return nil, err
	}

	a.info = info

	return info, nil
}

// atsumaruScanlator is a scanlation group as embedded in the series page
type atsumaruScanlator struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// atsumaruMangaInfo is the JSON feed for the manga info/chapters api
type atsumaruMangaInfo struct {
	Title    string                `json:"title"`
	Chapters []atsumaruInfoChapter `json:"chapters"`
}

// atsumaruInfoChapter is a single chapter in the manga info feed, which mixes
// every scanlation group's uploads together
type atsumaruInfoChapter struct {
	Id     string  `json:"id"`
	Title  string  `json:"title"`
	Number float64 `json:"number"`
	ScanId string  `json:"scanId"`
}
