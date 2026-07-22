// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Guya is a grabber for guya.moe (and its guya.cubari.moe host, the
// underlying domain guya.moe redirects to): a guyamoe/cubari instance with a
// wide-open JSON API (`/api/series/{slug}/`) that returns the full
// chapter/page tree in a single call - no browser, no per-chapter fetch
// needed, since the page filenames for every group are already in that JSON
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

// Test returns true if the URL is a guya.moe (or guya.cubari.moe) URL
func (g *Guya) Test() (bool, error) {
	re := regexp.MustCompile(`guya\.(moe|cubari\.moe)`)
	return re.MatchString(g.URL), nil
}

// FetchTitle fetches and returns the manga title
func (g *Guya) FetchTitle() (string, error) {
	if g.title != "" {
		return g.title, nil
	}

	feed, err := g.seriesData()
	if err != nil {
		return "", err
	}

	g.title = sanitizeTitle(feed.Title)

	return g.title, nil
}

// FetchChapters returns the chapters of the manga
func (g Guya) FetchChapters() (Filterables, []error) {
	feed, err := g.seriesData()
	if err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(feed.Chapters))
	for key, c := range feed.Chapters {
		number, err := strconv.ParseFloat(key, 64)
		if err != nil {
			continue
		}

		groupId := preferredGroup(c.Groups, feed.PreferredSort)
		if groupId == "" {
			continue
		}

		title := c.Title
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(number, 'f', -1, 64)
		}

		chapters = append(chapters, &GuyaChapter{
			Chapter{
				Number: number,
				Title:  title,
			},
			feed.Slug,
			c.Folder,
			groupId,
			c.Groups[groupId],
		})
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
		Number:     f.GetNumber(),
		PagesCount: int64(len(gchap.Files)),
		Language:   "en",
	}

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

// seriesData fetches and unmarshals the series API feed
func (g Guya) seriesData() (*guyaSeriesFeed, error) {
	slug, err := g.slug()
	if err != nil {
		return nil, err
	}

	uri := g.BaseUrl() + "/api/series/" + slug + "/"
	body, err := http.GetText(http.RequestParams{
		URL:     uri,
		Referer: g.URL,
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

// preferredGroup returns the first group id (out of the ones that scanlated
// this chapter) found in the series' preferred_sort list, falling back to
// any available group if none of the preferred ones scanlated this chapter
func preferredGroup(groups map[string][]string, preferredSort []string) string {
	for _, id := range preferredSort {
		if _, ok := groups[id]; ok {
			return id
		}
	}
	for id := range groups {
		return id
	}
	return ""
}

// guyaSeriesFeed is the JSON feed for a guya series (`/api/series/{slug}/`)
type guyaSeriesFeed struct {
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	PreferredSort []string `json:"preferred_sort"`
	Chapters      map[string]struct {
		Title  string              `json:"title"`
		Folder string              `json:"folder"`
		Groups map[string][]string `json:"groups"`
	} `json:"chapters"`
}
