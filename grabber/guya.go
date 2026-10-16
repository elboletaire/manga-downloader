// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Guya is a grabber for guya.moe (and its guya.cubari.moe host, the
// underlying domain guya.moe redirects to) and danke.moe (Danke fürs Lesen,
// the same open-source "guyamoe" software - see their /about/ page and
// footer "Source Code" link to github.com/kafkaien42/guyamoe): a guyamoe/
// cubari instance with a wide-open JSON API (`/api/series/{slug}/`) that
// returns the full chapter/page tree in a single call - no browser, no
// per-chapter fetch needed, since the page filenames for every group are
// already in that JSON
type Guya struct {
	*Grabber
	title string
}

func NewGuya(g *Grabber) *Guya {
	return &Guya{Grabber: g}
}

// GuyaChapter represents a Guya chapter
type GuyaChapter struct {
	Chapter
	Slug    string
	Folder  string
	GroupId string
	Files   []string
}

var (
	guyaHostRe = regexp.MustCompile(`(guya\.(moe|cubari\.moe)|danke\.moe)`)
	// guyaReadRe is the reader path every Guya instance shares
	guyaReadRe = regexp.MustCompile(`/read/manga/[^/]+`)
)

// Test returns true if the URL is a guya.moe/guya.cubari.moe or danke.moe URL,
// or a /read/manga/{slug} one of another instance, told by its series API.
func (g *Guya) Test() (bool, error) {
	if guyaHostRe.MatchString(g.URL) {
		return true, nil
	}
	if !guyaReadRe.MatchString(g.URL) {
		return false, nil
	}

	// an unreachable or non-Guya site just declines
	feed, err := g.seriesData(true)
	return err == nil && feed.Slug != "" && feed.Chapters != nil, nil
}

// FetchTitle fetches and returns the manga title
func (g *Guya) FetchTitle() (string, error) {
	if g.title != "" {
		return g.title, nil
	}

	feed, err := g.seriesData(false)
	if err != nil {
		return "", err
	}

	g.title = sanitizeTitle(feed.Title)

	return g.title, nil
}

// FetchChapters returns the chapters of the manga: one per group that
// scanlated each chapter, the group in Chapter.Group, so --scanlator picks
// among them (see grabber.SelectGroup)
func (g Guya) FetchChapters() (Filterables, []error) {
	feed, err := g.seriesData(false)
	if err != nil {
		return nil, []error{err}
	}

	chapters := Filterables{}
	for key, c := range feed.Chapters {
		number, err := strconv.ParseFloat(key, 64)
		if err != nil {
			continue
		}

		for _, groupId := range orderedGroups(c.Groups, feed.PreferredSort) {
			listed := &GuyaChapter{
				Chapter{
					Number: number,
					Title:  c.Title,
					ID:     c.Folder + "/" + groupId,
					Label:  Label{Style: LabelNumbered},
				},
				feed.Slug,
				c.Folder,
				groupId,
				c.Groups[groupId],
			}
			listed.SetGroups(feed.Groups[groupId])
			chapters = append(chapters, listed)
		}
	}

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages. All the data needed (folder,
// group and page filenames) was already fetched as part of FetchChapters, so
// no extra request is needed here: pages just follow the site's predictable
// media URL scheme (/media/manga/{slug}/chapters/{folder}/{group}/{file})
func (g Guya) FetchChapter(f Filterable) (*Chapter, error) {
	gchap := f.(*GuyaChapter)

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(gchap.Files)),
		Language:   "en",
		ID:         gchap.ID,
	}
	chapter.SetGroups(gchap.GetGroups()...)

	for i, file := range gchap.Files {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL: fmt.Sprintf(
				"%s/media/manga/%s/chapters/%s/%s/%s",
				g.BaseUrl(), gchap.Slug, gchap.Folder, gchap.GroupId, file,
			),
		})
	}

	return chapter, nil
}

// seriesData fetches and unmarshals the series API feed; probe skips the
// retries, for identifying a site.
func (g Guya) seriesData(probe bool) (*guyaSeriesFeed, error) {
	slug, err := g.slug()
	if err != nil {
		return nil, err
	}

	uri := g.BaseUrl() + "/api/series/" + slug + "/"
	body, err := http.GetText(http.RequestParams{
		URL:          uri,
		Referer:      g.URL,
		DisableRetry: probe,
	})
	if err != nil {
		return nil, err
	}

	feed := &guyaSeriesFeed{}
	if err = json.Unmarshal([]byte(body), feed); err != nil {
		return nil, err
	}

	return feed, nil
}

// slug returns the manga slug from the URL (i.e.
// "Kaguya-Wants-To-Be-Confessed-To" for
// https://guya.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/)
func (g Guya) slug() (string, error) {
	re := regexp.MustCompile(`/manga/([^/]+)`)
	matches := re.FindStringSubmatch(g.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find manga slug in url %s", g.URL)
	}
	return matches[1], nil
}

// orderedGroups returns the ids of the groups that scanlated a chapter: the
// ones in the series' preferred_sort list first, in that order, then the rest
// by numeric id
func orderedGroups(groups map[string][]string, preferredSort []string) []string {
	rank := make(map[string]int, len(preferredSort))
	for i, id := range preferredSort {
		rank[id] = i
	}

	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		ri, preferredI := rank[ids[i]]
		rj, preferredJ := rank[ids[j]]
		if preferredI != preferredJ {
			return preferredI
		}
		if preferredI && ri != rj {
			return ri < rj
		}
		ni, errI := strconv.Atoi(ids[i])
		nj, errJ := strconv.Atoi(ids[j])
		if errI == nil && errJ == nil && ni != nj {
			return ni < nj
		}
		return ids[i] < ids[j]
	})

	return ids
}

// guyaSeriesFeed is the JSON feed for a guya series (`/api/series/{slug}/`)
type guyaSeriesFeed struct {
	Slug          string            `json:"slug"`
	Title         string            `json:"title"`
	Groups        map[string]string `json:"groups"`
	PreferredSort []string          `json:"preferred_sort"`
	Chapters      map[string]struct {
		Title  string              `json:"title"`
		Folder string              `json:"folder"`
		Groups map[string][]string `json:"groups"`
	} `json:"chapters"`
}
