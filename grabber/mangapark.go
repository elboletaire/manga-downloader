// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Mangapark is a grabber for mangapark.cc. Series pages live at /manga/{slug},
// chapters are listed via GET /manga/{slug}/chapters?lang=LANG (paginated with
// ?offset=N until data-has-more is false), and readers at
// /read/{slug}/{lang}/chapter-N. Old hosts (mangapark1.com/.to/.io/.page) are
// rewritten to .cc without any request. Images are served from an external CDN.
type Mangapark struct {
	*Grabber
	title     string
	slug      string
	languages []string // uppercase codes from the series page's language tabs
}

// NewMangapark creates a new Mangapark grabber.
func NewMangapark(g *Grabber) *Mangapark {
	return &Mangapark{Grabber: g}
}

// MangaparkChapter represents a chapter listed by the MangaPark API.
type MangaparkChapter struct {
	Chapter
	Link string // reader path: /read/{slug}/{lang}/chapter-N
}

// mangaparkBase is the canonical origin for all mangapark.cc requests.
const mangaparkBase = "https://mangapark.cc"

// mangaparkHostRe matches mangapark.cc and the old dead/hijacked hosts.
var mangaparkHostRe = regexp.MustCompile(`mangapark(?:\.cc|1\.com|\.(?:to|io|page))`)

// mangaparkPathRe extracts the series slug from a path. It handles
// /manga/{slug}, /read/{slug}/..., /series/{slug} (old hosts), or a bare
// /{slug} (old hosts only, captured in group 2).
var mangaparkPathRe = regexp.MustCompile(`^/(?:manga|read|series)/([^/]+)|^/([^/]+)/?$`)

// mangaparkTitlePrefixRe matches the "Chapter N" or "Chapter N: " prefix the
// site embeds in every chapter span title, stripping it to expose the subtitle.
var mangaparkTitlePrefixRe = regexp.MustCompile(`^Chapter \d+(\.\d+)?[: ]*`)

// mangaparkLang normalises a language setting to the uppercase code the
// chapters API expects (e.g. "en" → "EN"), defaulting to "EN".
func mangaparkLang(lang string) string {
	if lang == "" {
		return "EN"
	}
	return strings.ToUpper(lang)
}

// mangaparkChapterTitle strips the "Chapter N" or "Chapter N: " prefix from a
// span title and returns the subtitle, empty when nothing remains
func mangaparkChapterTitle(title string) string {
	return strings.TrimSpace(mangaparkTitlePrefixRe.ReplaceAllString(title, ""))
}

// mangaparkSeriesURL returns the canonical series page URL for any link the
// grabber accepts, rewriting old hosts to mangapark.cc without fetching.
func mangaparkSeriesURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	matches := mangaparkPathRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return "", fmt.Errorf("mangapark: could not find the series slug in %q", raw)
	}
	slug := matches[1]
	if slug == "" {
		// bare /{slug} is only valid on the old hosts, not on .cc
		if u.Hostname() == "mangapark.cc" {
			return "", fmt.Errorf("mangapark: could not find the series slug in %q", raw)
		}
		slug = matches[2]
	}

	return mangaparkBase + "/manga/" + slug, nil
}

// LanguageOptions lists the languages the series is translated into, sourced
// from the series page's language-tab dropdown.
func (m *Mangapark) LanguageOptions() FilterOptions {
	return FilterOptions{
		Values: m.languages,
		All:    true,
	}
}

// Test returns true if the URL belongs to any MangaPark host.
func (m *Mangapark) Test() (bool, error) {
	return mangaparkHostRe.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the series title.
func (m *Mangapark) FetchTitle() (string, error) {
	if err := m.fetchSeriesData(); err != nil {
		return "", err
	}
	return m.title, nil
}

// FetchChapters returns the complete chapter list from the paginated API.
func (m *Mangapark) FetchChapters() (Filterables, []error) {
	if err := m.fetchSeriesData(); err != nil {
		return nil, []error{err}
	}

	chapters, err := m.fetchChaptersAPI()
	if err != nil {
		return nil, []error{err}
	}

	return chapters, nil
}

// fetchChaptersAPI pages through GET /manga/{slug}/chapters?lang=LANG&offset=N
// until data-has-more is false, collecting all MangaparkChapter entries.
func (m *Mangapark) fetchChaptersAPI() (Filterables, error) {
	lang := mangaparkLang(m.Settings.Language)
	chapters := Filterables{}
	offset := 0

	for {
		apiURL := fmt.Sprintf("%s/manga/%s/chapters?lang=%s&offset=%d", mangaparkBase, m.slug, lang, offset)
		body, err := http.GetText(http.RequestParams{URL: apiURL, Referer: m.URL})
		if err != nil {
			return nil, err
		}

		batch, nextOffset, hasMore, err := parseMangaparkChaptersBatch(body)
		if err != nil {
			return nil, err
		}

		chapters = append(chapters, batch...)

		if !hasMore {
			break
		}
		offset = nextOffset
	}

	if len(chapters) == 0 {
		if lang := m.Settings.Language; lang != "" {
			return nil, &UnavailableFilterError{
				Filter:    "language",
				Value:     lang,
				Available: m.languages,
			}
		}
		return nil, errors.New("mangapark: no chapters found")
	}

	return chapters, nil
}

// parseMangaparkChaptersBatch parses one HTML fragment from the chapters API,
// returning chapters, the next offset, whether more pages follow, and any error.
func parseMangaparkChaptersBatch(body string) (Filterables, int, bool, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, 0, false, err
	}

	meta := doc.Find("span.chapter-batch-meta")
	hasMore := meta.AttrOr("data-has-more", "") == "true"
	nextOffset, _ := strconv.Atoi(meta.AttrOr("data-next-offset", "0"))

	chapters := Filterables{}
	seqNum := 1.0

	doc.Find("li.item").Each(func(_ int, li *goquery.Selection) {
		link := li.Find("a")
		href, ok := link.Attr("href")
		if !ok || href == "" {
			return
		}

		// Full title: first span has the chapter name and optional subtitle;
		// the flag <i> has no text so it doesn't appear in span.Text().
		title := sanitizeTitle(link.Find("span").First().Text())
		if title == "" {
			title = sanitizeTitle(link.AttrOr("title", ""))
		}

		// Language code is the 4th slash-separated segment of the reader path:
		// /read/{slug}/{lang}/chapter-N → ["", "read", slug, lang, ...]
		// Uppercase so it matches the codes LanguageOptions() returns.
		var lang string
		if parts := strings.SplitN(href, "/", 5); len(parts) >= 4 {
			lang = strings.ToUpper(parts[3])
		}

		dataNum := li.AttrOr("data-number", "")
		var number float64
		style := LabelNumbered
		if dataNum == "" {
			// One-shot or unnumbered: assign the next sequential number.
			number = seqNum
			seqNum++
			style = LabelUnnumbered
		} else {
			var parseErr error
			number, parseErr = strconv.ParseFloat(dataNum, 64)
			if parseErr != nil {
				return // skip malformed entries silently
			}
			seqNum = number + 1
			// Strip "Chapter N: " prefix; keep only the subtitle.
			title = mangaparkChapterTitle(title)
		}

		chapters = append(chapters, &MangaparkChapter{
			Chapter{
				Number:   number,
				Title:    title,
				Language: lang,
				ID:       href,
				Label:    Label{Style: style},
			},
			href,
		})
	})

	return chapters, nextOffset, hasMore, nil
}

// FetchChapter fetches the page list for a chapter from its reader page.
// Images are found in section.chapter-reader img; the CDN does not require a
// Referer header.
func (m *Mangapark) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaparkChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     mangaparkBase + mchap.Link,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:    f.GetTitle(),
		Label:    labelOf(f),
		Number:   f.GetNumber(),
		Language: mchap.Language,
		ID:       mchap.Link,
	}

	doc.Find("section.chapter-reader img").Each(func(i int, s *goquery.Selection) {
		src := s.AttrOr("src", "")
		if !strings.HasPrefix(src, "http") {
			return
		}
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    src,
		})
	})
	chapter.PagesCount = int64(len(chapter.Pages))

	if chapter.PagesCount == 0 {
		return nil, errors.New("no pages found in the chapter page")
	}

	return chapter, nil
}

// fetchSeriesData resolves the series URL from any accepted link, caches the
// title from the series page's <h1>, and collects the available language codes
// from the language-tab dropdown. It is a no-op after the first call.
func (m *Mangapark) fetchSeriesData() error {
	if m.title != "" {
		return nil
	}

	seriesURL, err := mangaparkSeriesURL(m.URL)
	if err != nil {
		return err
	}
	m.URL = seriesURL
	m.slug = seriesURL[len(mangaparkBase+"/manga/"):]

	body, err := http.GetText(http.RequestParams{URL: m.URL})
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return err
	}

	title := sanitizeTitle(doc.Find("h1").First().Text())
	if title == "" {
		return errors.New("could not find series title in the series page")
	}

	m.title = title
	doc.Find("a.dropdown-item[data-code]").Each(func(_ int, s *goquery.Selection) {
		if code := s.AttrOr("data-code", ""); code != "" {
			m.languages = append(m.languages, code)
		}
	})

	return nil
}
