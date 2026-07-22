// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/elboletaire/manga-downloader/http"
)

// Atsumaru is a grabber for atsu.moe: a react SPA (near-empty HTML shell),
// but its JSON api is completely open to plain HTTP, no cookies/auth needed
type Atsumaru struct {
	*Grabber
	title string
	// chapters caches the raw api chapter list, keyed by manga id, so
	// FetchTitle and FetchChapters can share a single api call
	info *atsumaruMangaInfo
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

	// atsu.moe lets multiple scanlation groups (scanId) upload their own
	// version of the same chapter numbers; the api always returns every
	// group's chapters mixed together (there's no per-group endpoint), so
	// we pick the group with the most chapters (the most complete
	// translation) and ignore the rest, to keep chapter numbers unique
	counts := map[string]int{}
	for _, c := range info.Chapters {
		counts[c.ScanId]++
	}
	best := ""
	for id, count := range counts {
		if count > counts[best] {
			best = id
		}
	}

	for _, c := range info.Chapters {
		if c.ScanId != best {
			continue
		}
		title := c.Title
		if title == "" {
			title = "Chapter " + strconv.FormatFloat(c.Number, 'f', -1, 64)
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

// atsumaruMangaInfo is the JSON feed for the manga info/chapters api
type atsumaruMangaInfo struct {
	Title    string `json:"title"`
	Chapters []struct {
		Id     string  `json:"id"`
		Title  string  `json:"title"`
		Number float64 `json:"number"`
		ScanId string  `json:"scanId"`
	} `json:"chapters"`
}
