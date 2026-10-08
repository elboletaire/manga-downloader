// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// a var so the tests can point it at a local server
var sacachispaAPI = "https://api.sacachispa.site/api"

// Sacachispa is a grabber for sacachispa.site, an English scanlation site.
// The site is a SvelteKit SPA over an open JSON API (api.sacachispa.site): the
// series page ships no data of its own (and renders just a slice of the chapter
// list), so everything is read from the API, which needs no authentication for
// free chapters. A chapter ("chapter") has one or more releases (one per
// group/language), and a release owns the page images.
type Sacachispa struct {
	*Grabber
	mangaID string
}

func NewSacachispa(g *Grabber) *Sacachispa {
	return &Sacachispa{Grabber: g}
}

// SacachispaChapter represents a sacachispa.site chapter
type SacachispaChapter struct {
	Chapter
	// ID is the API's chapter id, whose releases hold the pages
	ID string
	// PatreonOnly marks chapters the API refuses to serve (403) until they
	// become free
	PatreonOnly bool
}

// the site's URLs carry the manga's UUID: /manga/{uuid}/{slug} for a series
// and /read/{release uuid} for the reader
var (
	sacachispaMangaRe = regexp.MustCompile(`/manga/([0-9a-fA-F-]{36})`)
	sacachispaReadRe  = regexp.MustCompile(`/read/([0-9a-fA-F-]{36})`)
)

// Test returns true if the URL is a sacachispa.site URL
func (s *Sacachispa) Test() (bool, error) {
	re := regexp.MustCompile(`sacachispa\.site`)
	return re.MatchString(s.URL), nil
}

// FetchTitle fetches and returns the manga title
func (s *Sacachispa) FetchTitle() (string, error) {
	id, err := s.resolveMangaID()
	if err != nil {
		return "", err
	}

	var res struct {
		Data struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := sacachispaGet("/manga/"+id, &res); err != nil {
		return "", err
	}
	if res.Data.Title == "" {
		return "", errors.New("no title found for the series")
	}

	return sanitizeTitle(res.Data.Title), nil
}

// FetchChapters returns the chapters of the manga. Their pages are fetched
// later, in FetchChapter, so only the requested chapters cost API calls.
func (s *Sacachispa) FetchChapters() (Filterables, []error) {
	id, err := s.resolveMangaID()
	if err != nil {
		return nil, []error{err}
	}

	chapters := Filterables{}
	for page, pages := 1, 1; page <= pages; page++ {
		var res struct {
			Data []struct {
				ID          string `json:"id"`
				Chapter     string `json:"chapter"`
				Title       string `json:"title"`
				PatreonOnly bool   `json:"patreonOnly"`
			} `json:"data"`
			Pagination struct {
				Pages int `json:"pages"`
			} `json:"pagination"`
		}
		path := fmt.Sprintf("/chapters?mangaId=%s&limit=100&page=%d", id, page)
		if err := sacachispaGet(path, &res); err != nil {
			return nil, []error{err}
		}
		pages = res.Pagination.Pages

		for _, c := range res.Data {
			// decimals like 22.5 exist, so this must stay a float
			number, err := strconv.ParseFloat(strings.TrimSpace(c.Chapter), 64)
			if err != nil {
				continue
			}

			chapters = append(chapters, &SacachispaChapter{
				Chapter: Chapter{
					Number:   number,
					Title:    chapterTitleOrDefault(c.Title, number),
					Language: "en",
				},
				ID:          c.ID,
				PatreonOnly: c.PatreonOnly,
			})
		}
	}

	return chapters, nil
}

// FetchChapter returns the chapter and its pages
func (s Sacachispa) FetchChapter(f Filterable) (*Chapter, error) {
	schap, ok := f.(*SacachispaChapter)
	if !ok {
		return nil, errors.New("invalid chapter type")
	}

	var rels struct {
		Data []struct {
			ID       string `json:"id"`
			Language string `json:"language"`
		} `json:"data"`
	}
	if err := sacachispaGet("/chapters/"+schap.ID+"/releases", &rels); err != nil {
		return nil, err
	}
	if len(rels.Data) == 0 {
		return nil, fmt.Errorf("no releases found for chapter %s", formatChapterNumber(f.GetNumber()))
	}

	// a chapter may have several releases; prefer the requested language
	release := rels.Data[0]
	if lang := s.GetPreferredLanguage(); lang != "" {
		for _, r := range rels.Data {
			if r.Language == lang {
				release = r
				break
			}
		}
	}

	var res struct {
		Data struct {
			Items []struct {
				Page int    `json:"page"`
				URL  string `json:"url"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := sacachispaGet("/releases/"+release.ID+"/pages", &res); err != nil {
		if schap.PatreonOnly {
			return nil, fmt.Errorf(
				"chapter %s is Patreon-exclusive and not available yet: %w",
				formatChapterNumber(f.GetNumber()), err,
			)
		}
		return nil, err
	}

	pages := make([]Page, 0, len(res.Data.Items))
	for i, p := range res.Data.Items {
		if strings.TrimSpace(p.URL) == "" {
			continue
		}
		pages = append(pages, Page{Number: int64(i + 1), URL: p.URL})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for chapter %s", formatChapterNumber(f.GetNumber()))
	}

	chapter := schap.Chapter
	chapter.Pages = pages
	chapter.PagesCount = int64(len(pages))

	return &chapter, nil
}

// resolveMangaID returns the manga's UUID out of the series URL, or, for a
// reader URL, looks it up through the release (so, as everywhere else, a
// chapter URL downloads the whole series)
func (s *Sacachispa) resolveMangaID() (string, error) {
	if s.mangaID != "" {
		return s.mangaID, nil
	}

	if m := sacachispaMangaRe.FindStringSubmatch(s.URL); m != nil {
		s.mangaID = m[1]
		return s.mangaID, nil
	}

	if m := sacachispaReadRe.FindStringSubmatch(s.URL); m != nil {
		var res struct {
			Data struct {
				Manga struct {
					ID string `json:"id"`
				} `json:"manga"`
			} `json:"data"`
		}
		if err := sacachispaGet("/releases/"+m[1], &res); err != nil {
			return "", err
		}
		if res.Data.Manga.ID != "" {
			s.mangaID = res.Data.Manga.ID
			return s.mangaID, nil
		}
	}

	return "", errors.New("unsupported URL: expected https://sacachispa.site/manga/{id}/{slug} (the older /series/{slug} URLs no longer exist)")
}

// sacachispaGet fetches an API path and decodes its JSON response into v
func sacachispaGet(path string, v any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     sacachispaAPI + path,
		Referer: "https://sacachispa.site",
		Headers: map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return fmt.Errorf("%s: %w", strings.SplitN(path, "?", 2)[0], err)
	}

	return json.Unmarshal([]byte(body), v)
}
