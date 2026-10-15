//go:build demo

// Package demo mocks the sites the VHS tapes in assets/demos download from, so
// they run offline and show the same output every time. A fake grabber serves
// the series in series/*.json and a transport serves their pages. Only the
// demo build (-tags demo, make build/demo) includes it; see README.md.
package demo

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
)

//go:embed series/*.json
var seriesFiles embed.FS

// series is one fake series, as series/*.json holds it
type series struct {
	// URLs are the series pages this data answers for; no other URL is claimed
	URLs  []string `json:"urls"`
	Title string   `json:"title"`
	// Languages are the ones the series is translated into. Set, they make the
	// site filter by --language and list them; unset, the site is single-language
	Languages []string `json:"languages"`
	// NameFormat, if set, is the title a fetched chapter gets, formatted with
	// its number truncated to a whole one, as MangaDex does ("Chapter %04d %s")
	NameFormat string    `json:"nameFormat"`
	Chapters   []chapter `json:"chapters"`
}

// chapter is one listed chapter of a series
type chapter struct {
	Number   float64  `json:"number"`
	Title    string   `json:"title"`
	Language string   `json:"language"`
	ID       string   `json:"id"`
	Pages    int      `json:"pages"`
	Volume   int      `json:"volume"`
	Groups   []string `json:"groups"`
	Locked   bool     `json:"locked"`
}

// Install registers the fake grabber ahead of the real ones and routes page
// requests to the placeholder transport. It's called from cmd's init.
func Install() {
	all, err := loadSeries(seriesFiles)
	if err != nil {
		panic(err)
	}
	scale, err := parseDelayScale(os.Getenv(delayEnv))
	if err != nil {
		panic(err)
	}

	http.SetRoundTripper(pages{scale: scale})
	for _, data := range all {
		grabber.RegisterEarly(func(g *grabber.Grabber) grabber.Site {
			return newSite(g, data)
		})
	}
}

// loadSeries reads every series file of the series folder in files
func loadSeries(files fs.FS) ([]*series, error) {
	names, err := fs.Glob(files, "series/*.json")
	if err != nil {
		return nil, err
	}

	all := make([]*series, 0, len(names))
	for _, name := range names {
		raw, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, err
		}
		s := &series{}
		if err := json.Unmarshal(raw, s); err != nil {
			return nil, fmt.Errorf("demo: reading %s: %w", name, err)
		}
		all = append(all, s)
	}
	return all, nil
}

// site is the fake grabber of one series: it answers only for that series' URLs
type site struct {
	*grabber.Grabber
	data *series
}

// languageSite is a site whose series lists languages, so it can be filtered
// by them like the real sites that do
type languageSite struct {
	*site
}

// newSite returns the grabber of one series
func newSite(g *grabber.Grabber, data *series) grabber.Site {
	s := &site{Grabber: g, data: data}
	if len(data.Languages) > 0 {
		return &languageSite{s}
	}
	return s
}

// Test claims the URL when the series lists it
func (s *site) Test() (bool, error) {
	url := strings.TrimRight(s.URL, "/")
	for _, u := range s.data.URLs {
		if strings.TrimRight(u, "/") == url {
			return true, nil
		}
	}
	return false, nil
}

// FetchTitle returns the series title
func (s *site) FetchTitle() (string, error) {
	return s.data.Title, nil
}

// FetchChapters lists the series' chapters, only those of --language when the
// series is translated into several. A language the series doesn't list is an
// UnavailableFilterError, as on the real sites that list their languages.
func (s *site) FetchChapters() (grabber.Filterables, []error) {
	lang := s.Settings.Language
	filter := len(s.data.Languages) > 0 && lang != "" && lang != grabber.FilterAll
	if filter && !slices.Contains(s.data.Languages, lang) {
		return nil, []error{&grabber.UnavailableFilterError{Filter: "language", Value: lang, Available: s.data.Languages}}
	}

	var chapters grabber.Filterables
	for _, c := range s.data.Chapters {
		if filter && c.Language != lang {
			continue
		}
		listed := &grabber.Chapter{
			Number:     c.Number,
			Title:      c.Title,
			Language:   c.Language,
			PagesCount: int64(c.Pages),
			Volume:     c.Volume,
			ID:         c.ID,
			Locked:     c.Locked,
		}
		listed.SetGroups(c.Groups...)
		chapters = append(chapters, listed)
	}
	return chapters, nil
}

// FetchChapter returns the chapter with its page URLs
func (s *site) FetchChapter(f grabber.Filterable) (*grabber.Chapter, error) {
	listed := f.(*grabber.Chapter)

	chapter := *listed
	if s.data.NameFormat != "" {
		chapter.Title = fmt.Sprintf(s.data.NameFormat, int64(listed.Number), listed.Title)
	}
	for n := 1; n <= int(listed.PagesCount); n++ {
		chapter.Pages = append(chapter.Pages, grabber.Page{Number: int64(n), URL: pageURL(listed.ID, n)})
	}
	return &chapter, nil
}

// LanguageOptions lists the languages the series is translated into
func (l *languageSite) LanguageOptions() grabber.FilterOptions {
	return grabber.FilterOptions{Values: l.data.Languages, All: true}
}
