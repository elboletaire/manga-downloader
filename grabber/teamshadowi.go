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

// Teamshadowi is a grabber for team-shadowi.com, a French scanlation site
// built on the Next.js App Router. It doesn't use the classic __NEXT_DATA__
// script tag; instead the whole rendered page (including every fetched prop)
// is streamed as escaped JSON string chunks inside a series of
// `self.__next_f.push([1,"..."])` calls. The series page's payload embeds
// not only the chapter list but every chapter's full page-image URLs
// already resolved, so no separate reader-page fetch is ever needed.
type Teamshadowi struct {
	*Grabber
	data *teamshadowiPublicData
}

func NewTeamshadowi(g *Grabber) *Teamshadowi {
	return &Teamshadowi{Grabber: g}
}

// TeamshadowiChapter represents a team-shadowi.com chapter. Its pages are
// already known from the series page fetch, so FetchChapter needs no
// further network call.
type TeamshadowiChapter struct {
	Chapter
}

// Test returns true if the URL is a team-shadowi.com URL
func (t *Teamshadowi) Test() (bool, error) {
	re := regexp.MustCompile(`team-shadowi\.com`)
	return re.MatchString(t.URL), nil
}

// FetchTitle fetches and returns the manga title
func (t *Teamshadowi) FetchTitle() (string, error) {
	data, err := t.fetchData()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(data.Series.Title), nil
}

// FetchChapters returns the chapters of the manga, including their page
// images (already embedded in the series page payload)
func (t *Teamshadowi) FetchChapters() (Filterables, []error) {
	data, err := t.fetchData()
	if err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(data.Chapters))
	for _, c := range data.Chapters {
		number, err := strconv.ParseFloat(c.Number, 64)
		if err != nil {
			continue
		}

		title := fmt.Sprintf("Chapter %s", c.Number)
		if strings.TrimSpace(c.Title) != "" {
			title += " - " + c.Title
		}

		pages := make([]Page, 0, len(c.ImagePaths))
		for i, url := range c.ImagePaths {
			pages = append(pages, Page{
				Number: int64(i + 1),
				URL:    url,
			})
		}

		chapters = append(chapters, &TeamshadowiChapter{
			Chapter{
				Number:     number,
				Title:      title,
				PagesCount: int64(len(pages)),
				Pages:      pages,
				Language:   "en",
			},
		})
	}

	return chapters, nil
}

// FetchChapter returns the chapter and its pages. The series page fetch
// (FetchChapters) already carried every chapter's full image list, so
// there's nothing left to fetch here.
func (t Teamshadowi) FetchChapter(f Filterable) (*Chapter, error) {
	tchap, ok := f.(*TeamshadowiChapter)
	if !ok {
		return nil, errors.New("invalid chapter type")
	}

	chapter := tchap.Chapter

	return &chapter, nil
}

// fetchData fetches and caches the series page's embedded JSON payload
// (title, chapters and each chapter's page images)
func (t *Teamshadowi) fetchData() (*teamshadowiPublicData, error) {
	if t.data != nil {
		return t.data, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:     t.URL,
		Referer: t.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	data, err := parseTeamshadowiPublicData(body)
	if err != nil {
		return nil, err
	}

	t.data = data

	return t.data, nil
}

// nextFPushRe matches the string literal argument of every
// `self.__next_f.push([1,"..."])` call in a Next.js App Router page: the
// site streams its rendered page (including all fetched data) as a series
// of these escaped JSON string chunks. Since inner quotes are always
// backslash-escaped, the first unescaped `"])` is guaranteed to be the
// real end of the string literal.
var nextFPushRe = regexp.MustCompile(`(?s)self\.__next_f\.push\(\[1,"(.*?)"\]\)`)

// parseTeamshadowiPublicData extracts the `"publicData":{...}` JSON object
// embedded in the page's Next.js RSC stream and unmarshals it
func parseTeamshadowiPublicData(html string) (*teamshadowiPublicData, error) {
	matches := nextFPushRe.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return nil, errors.New("no __next_f payload found in the page (is the URL a series page?)")
	}

	var stream strings.Builder
	for _, m := range matches {
		var chunk string
		// each captured group is a JSON-escaped string (Next.js serializes
		// it with JSON.stringify), so wrapping it back in quotes and
		// decoding it as a JSON string unescapes \", \\, \/, \n, \uXXXX...
		if err := json.Unmarshal([]byte(`"`+m[1]+`"`), &chunk); err != nil {
			continue
		}
		stream.WriteString(chunk)
	}

	raw, err := extractBalancedJSON(stream.String(), `"publicData":`)
	if err != nil {
		return nil, err
	}

	data := &teamshadowiPublicData{}
	if err := json.Unmarshal([]byte(raw), data); err != nil {
		return nil, err
	}

	return data, nil
}

// extractBalancedJSON finds the given key marker in s and returns the JSON
// object that follows it, matching braces while respecting quoted strings
// (the marker sits inside a much larger, non-JSON RSC stream, so a naive
// "find the last }" wouldn't work)
func extractBalancedJSON(s, marker string) (string, error) {
	idx := strings.Index(s, marker)
	if idx == -1 {
		return "", fmt.Errorf("marker %q not found in the page data", marker)
	}

	start := idx + len(marker)
	for start < len(s) && s[start] != '{' {
		start++
	}
	if start >= len(s) {
		return "", fmt.Errorf("no JSON object found after marker %q", marker)
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}

	return "", fmt.Errorf("unbalanced JSON object after marker %q", marker)
}

// teamshadowiPublicData is the JSON payload embedded in team-shadowi.com
// series pages
type teamshadowiPublicData struct {
	Series struct {
		Title string `json:"title"`
	} `json:"series"`
	Chapters []struct {
		Number     string   `json:"number"`
		Title      string   `json:"title"`
		ImagePaths []string `json:"image_paths"`
	} `json:"chapters"`
}
