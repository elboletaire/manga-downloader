// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// tanstackComic is the shared implementation for the TanStack Start comic
// platform behind en-hijala.com, hivetoons.org, vortexscans.org and
// azorafly.com. Their series pages only server-render the ~20 most recent
// chapters as DOM rows (the rest sits behind
// a client-side "Load more"), but the FULL chapter list ships in the page's
// seroval stream: an `initialChapters:$R[n]=[...]` literal followed by
// `,totalChapterCount:N`. Reader pages are fully server rendered too: page
// images sit in plain <img data-reader-page-image> tags, no browser needed.
// Recently released chapters can be paywalled (coins/early access); those
// render zero reader images.
type tanstackComic struct {
	*Grabber
	series *tanstackSeries
}

// TanstackChapter represents a chapter of a tanstackComic site
type TanstackChapter struct {
	Chapter
	Slug string
}

// tanstackSeries is the series title and the full chapter list embedded in
// its page
type tanstackSeries struct {
	Title             string
	Chapters          []tanstackChapter
	TotalChapterCount int
}

type tanstackChapter struct {
	Number float64 `json:"number"`
	Slug   string  `json:"slug"`
	Title  string  `json:"title"`
	// IsLocked is the site's own lock summary (coins, early access, short link)
	IsLocked          bool `json:"isLocked"`
	IsShortLinkLocked bool `json:"isShortLinkLocked"`
}

var (
	tanstackChaptersRe   = regexp.MustCompile(`initialChapters:\$R\[\d+\]=`)
	tanstackTitleRe      = regexp.MustCompile(`^postTitle:"((?:[^"\\]|\\.)*)"`)
	tanstackTotalRe      = regexp.MustCompile(`^,totalChapterCount:(\d+)`)
	serovalDefRe         = regexp.MustCompile(`^\$R\[\d+\]=`)
	tanstackChapterURLRe = regexp.MustCompile(`/series/([^/?#]+)/chapter-`)
)

// resolveSeriesURL converts a chapter URL to a series URL if needed.
func (t *tanstackComic) resolveSeriesURL() {
	if !tanstackChapterURLRe.MatchString(t.URL) {
		return
	}
	matches := tanstackChapterURLRe.FindStringSubmatch(t.URL)
	if len(matches) >= 2 {
		slug := matches[1]
		t.URL = fmt.Sprintf("%s/series/%s", t.BaseUrl(), slug)
	}
}

// FetchTitle fetches and returns the manga title
func (t *tanstackComic) FetchTitle() (string, error) {
	t.resolveSeriesURL()
	series, err := t.fetchSeries()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(series.Title), nil
}

// FetchChapters returns the chapters of the manga
func (t *tanstackComic) FetchChapters() (chapters Filterables, errs []error) {
	t.resolveSeriesURL()
	series, err := t.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	// the marker only proves the list was found, not that it has entries; the
	// whole point of parsing it is getting the full list, so if the site ever
	// starts paginating it fail loudly instead of silently truncating
	if len(series.Chapters) == 0 {
		return nil, []error{errors.New("could not find the chapter list in the series page")}
	}
	if len(series.Chapters) < series.TotalChapterCount {
		return nil, []error{fmt.Errorf(
			"the embedded chapter list is incomplete (%d of %d chapters): the site may have started paginating it",
			len(series.Chapters), series.TotalChapterCount,
		)}
	}

	for _, c := range series.Chapters {
		chapters = append(chapters, &TanstackChapter{
			Chapter{
				Number: c.Number,
				Title:  strings.TrimSpace(c.Title),
				ID:     c.Slug,
				Label:  Label{Style: LabelNumbered},
				// a short-link lock may still open to a reader (none is live to
				// check), so only the other locks fail a chapter unfetched
				Locked: c.IsLocked && !c.IsShortLinkLocked,
			},
			c.Slug,
		})
	}

	return
}

// FetchChapter fetches a chapter and its pages
func (t *tanstackComic) FetchChapter(f Filterable) (*Chapter, error) {
	tchap := f.(*TanstackChapter)

	slug, err := t.seriesSlug()
	if err != nil {
		return nil, err
	}
	uri, err := url.JoinPath(t.BaseUrl(), "series", slug, tchap.Slug)
	if err != nil {
		return nil, err
	}

	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: t.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: "en",
		ID:       tchap.Slug,
		Locked:   tchap.Locked,
	}

	doc.Find("img[data-reader-page-image]").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	if chapter.PagesCount == 0 {
		return nil, errors.New("no pages found for this chapter (it might be locked behind coins/paid early access)")
	}

	return chapter, nil
}

// seriesSlug returns the series slug from the URL (i.e. "some-manga" for
// https://vortexscans.org/series/some-manga)
func (t tanstackComic) seriesSlug() (string, error) {
	re := regexp.MustCompile(`/series/([^/?#]+)`)
	matches := re.FindStringSubmatch(t.URL)
	if len(matches) != 2 {
		return "", fmt.Errorf("could not find series slug in url %s", t.URL)
	}
	return matches[1], nil
}

// fetchSeries fetches and caches the series page (see parseTanstackSeries)
func (t *tanstackComic) fetchSeries() (*tanstackSeries, error) {
	if t.series != nil {
		return t.series, nil
	}

	body, err := http.GetText(http.RequestParams{
		URL:     t.URL,
		Referer: t.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}

	series, err := parseTanstackSeries(body)
	if err != nil {
		return nil, err
	}

	t.series = series

	return series, nil
}

// parseTanstackSeries extracts the series title and its full chapter list out
// of a series page's seroval stream. The chapter list must be followed by the
// site's own totalChapterCount, which is what the guards in FetchChapters
// compare against.
func parseTanstackSeries(body string) (*tanstackSeries, error) {
	loc := tanstackChaptersRe.FindStringIndex(body)
	if loc == nil {
		return nil, errors.New("could not find the chapter list in the series page")
	}

	// the series object is serialized right before its chapter list, so the
	// last postTitle before it is the series' own
	idx := strings.LastIndex(body[:loc[0]], `postTitle:"`)
	if idx == -1 {
		return nil, errors.New("could not find the series title in the series page")
	}
	title := tanstackTitleRe.FindStringSubmatch(body[idx:])
	if len(title) != 2 {
		return nil, errors.New("could not parse the series title in the series page")
	}
	var decoded string
	if err := json.Unmarshal([]byte(`"`+title[1]+`"`), &decoded); err != nil {
		return nil, err
	}

	// body[loc[1]:] starts right at the array's "["
	raw, err := extractBalancedJSON(body[loc[1]:], "")
	if err != nil {
		return nil, err
	}

	total := tanstackTotalRe.FindStringSubmatch(body[loc[1]+len(raw):])
	if len(total) != 2 {
		return nil, errors.New("could not find totalChapterCount after the chapter list")
	}
	totalCount, err := strconv.Atoi(total[1])
	if err != nil {
		return nil, err
	}

	js, err := serovalToJSON(raw)
	if err != nil {
		return nil, err
	}

	series := &tanstackSeries{Title: decoded, TotalChapterCount: totalCount}
	if err := json.Unmarshal([]byte(js), &series.Chapters); err != nil {
		return nil, err
	}

	return series, nil
}

// serovalToJSON rewrites a seroval-serialized JavaScript literal (the form
// TanStack Start embeds its page data in) as JSON. Seroval writes object keys
// unquoted, booleans as !0/!1, undefined as void 0, and prefixes every object
// or array it defines with $R[n]= (a reference index, not data). String
// contents are copied verbatim. A bare $R[n] reference would need the rest of
// the graph, so it fails instead of being guessed at.
func serovalToJSON(lit string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(lit); {
		c := lit[i]
		switch {
		case c == '"':
			end := i + 1
			for end < len(lit) && lit[end] != '"' {
				if lit[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(lit) {
				return "", errors.New("unterminated string in the page data")
			}
			b.WriteString(lit[i : end+1])
			i = end + 1
		case c == '$':
			def := serovalDefRe.FindString(lit[i:])
			if def == "" {
				return "", errors.New("unsupported reference in the page data")
			}
			i += len(def)
		case strings.HasPrefix(lit[i:], "void 0"):
			b.WriteString("null")
			i += len("void 0")
		case c == '!' && i+1 < len(lit) && (lit[i+1] == '0' || lit[i+1] == '1'):
			if lit[i+1] == '0' {
				b.WriteString("true")
			} else {
				b.WriteString("false")
			}
			i += 2
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			end := i
			for end < len(lit) && (lit[end] == '_' || (lit[end] >= 'a' && lit[end] <= 'z') || (lit[end] >= 'A' && lit[end] <= 'Z') || (lit[end] >= '0' && lit[end] <= '9')) {
				end++
			}
			word := lit[i:end]
			switch {
			case end < len(lit) && lit[end] == ':':
				b.WriteString(`"` + word + `"`)
			case word == "null" || word == "true" || word == "false":
				b.WriteString(word)
			default:
				return "", fmt.Errorf("unsupported identifier %q in the page data", word)
			}
			i = end
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), nil
}
