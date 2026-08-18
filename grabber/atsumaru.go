// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

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

// Test returns true if the URL is an atsu.moe series URL
func (a *Atsumaru) Test() (bool, error) {
	re := regexp.MustCompile(`atsu\.moe/manga/`)
	return re.MatchString(a.URL), nil
}

// FetchTitle fetches and returns the manga title
func (a *Atsumaru) FetchTitle() (string, error) {
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

// FetchChapters returns the chapters of the manga
func (a *Atsumaru) FetchChapters() (chapters Filterables, errs []error) {
	info, err := a.mangaInfo()
	if err != nil {
		return nil, []error{err}
	}

	selected, err := a.selectGroups(a.groups(info))
	if err != nil {
		return nil, []error{err}
	}

	names := map[string]string{}
	for _, g := range selected {
		names[g.Id] = g.Name
	}
	// with more than one group in play the same chapter number appears once
	// per group, so tag the titles to keep them apart in the progress bars
	// and, more importantly, in the resulting filenames
	tag := len(selected) > 1

	for _, c := range info.Chapters {
		name, ok := names[c.ScanId]
		if !ok {
			continue
		}
		title := c.Title
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(c.Number, 'f', -1, 64)
		}
		if tag && name != "" {
			title += " [" + name + "]"
		}
		chapters = append(chapters, &AtsumaruChapter{
			Chapter{
				Number: c.Number,
				Title:  title,
			},
			c.Id,
		})
	}

	return chapters, errs
}

// atsumaruGroup is a scanlation group together with the number of chapters it
// uploaded for the series being fetched
type atsumaruGroup struct {
	atsumaruScanlator
	Count int
}

// groups returns the scanlation groups that uploaded chapters for this series,
// named where the series page could be read, in the order the site lists them
// (falling back to first-seen order for ids the page didn't name, so the
// default pick stays deterministic either way)
func (a *Atsumaru) groups(info *atsumaruMangaInfo) []atsumaruGroup {
	counts := map[string]int{}
	order := []string{}
	for _, c := range info.Chapters {
		if counts[c.ScanId] == 0 {
			order = append(order, c.ScanId)
		}
		counts[c.ScanId]++
	}

	groups := []atsumaruGroup{}
	seen := map[string]bool{}
	for _, s := range a.fetchScanlators() {
		if counts[s.Id] == 0 {
			// listed for the series but with nothing uploaded (yet)
			continue
		}
		seen[s.Id] = true
		groups = append(groups, atsumaruGroup{s, counts[s.Id]})
	}
	for _, id := range order {
		if !seen[id] {
			groups = append(groups, atsumaruGroup{atsumaruScanlator{Id: id}, counts[id]})
		}
	}

	return groups
}

// selectGroups narrows the available groups down to the ones to download,
// honouring --scanlator ("all" keeps every group). Without the flag it keeps
// the group with the most chapters, i.e. the most complete translation, and
// says so when there was actually a choice to make: the pick is otherwise
// invisible, and "most chapters" is no indication of quality (see #164).
func (a *Atsumaru) selectGroups(groups []atsumaruGroup) ([]atsumaruGroup, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	pref := strings.TrimSpace(a.Settings.Scanlator)

	if strings.EqualFold(pref, "all") {
		return groups, nil
	}

	if pref != "" {
		for _, g := range groups {
			if strings.EqualFold(g.Name, pref) || g.Id == pref {
				return []atsumaruGroup{g}, nil
			}
		}
		return nil, fmt.Errorf(
			"no scanlation group %q for this series, available: %s",
			pref, strings.Join(groupNames(groups), ", "),
		)
	}

	best := groups[0]
	for _, g := range groups[1:] {
		if g.Count > best.Count {
			best = g
		}
	}

	if len(groups) > 1 {
		color.Yellow(
			"this series has %d scanlation groups (%s); downloading %q (%d chapters) — pick another with --scanlator, or all of them with --scanlator all",
			len(groups), strings.Join(groupNames(groups), ", "), best.displayName(), best.Count,
		)
	}

	return []atsumaruGroup{best}, nil
}

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
		color.Yellow("could not fetch the series page to name the scanlation groups: %s", err.Error())
		return nil
	}
	defer body.Close()

	html, err := io.ReadAll(body)
	if err != nil {
		color.Yellow("could not read the series page to name the scanlation groups: %s", err.Error())
		return nil
	}

	raw, err := extractBalancedJSON(string(html), `"scanlators":`)
	if err != nil {
		color.Yellow("could not find the scanlation groups in the series page: %s", err.Error())
		return nil
	}

	scanlators := []atsumaruScanlator{}
	if err = json.Unmarshal([]byte(raw), &scanlators); err != nil {
		color.Yellow("could not parse the scanlation groups from the series page: %s", err.Error())
		return nil
	}

	a.scanlators = scanlators

	return a.scanlators
}

// displayName returns the group name, falling back to its id when the series
// page couldn't be read
func (g atsumaruGroup) displayName() string {
	if g.Name != "" {
		return g.Name
	}
	return g.Id
}

// groupNames returns the display names of the passed groups
func groupNames(groups []atsumaruGroup) []string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.displayName())
	}

	return names
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

	feed := struct {
		ReadChapter struct {
			Pages []struct {
				Image string `json:"image"`
			} `json:"pages"`
		} `json:"readChapter"`
	}{}
	if err = json.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		Language:   "en",
		PagesCount: int64(len(feed.ReadChapter.Pages)),
	}
	for i, p := range feed.ReadChapter.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    a.BaseUrl() + p.Image,
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
