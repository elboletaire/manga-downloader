// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// comiciDomains are the hosts of Comici, the viewer of several Japanese
// publishers: a fast path that needs no fetch. Any other host is detected by
// its markup (comiciFingerprint), so listing one is optional.
var comiciDomains = []string{
	"comic-growl.com",
	"heros-web.com",
	"championcross.jp",
	"comicride.jp",
	"takecomic.jp",
	"comic-ryu.jp",
	"younganimal.com",
	"bigcomics.jp",
	"kansai.mag-garden.co.jp",
}

// comiciMarkers are what a Comici series or episode page carries: the vendor
// link, which a footer alone can also be, and the episode list's items
var comiciMarkers = []string{"comici.co.jp", `data-e2e="eli"`}

var (
	// comiciEpisodeRe and comiciSeriesRe match the 13-character IDs of
	// episodes and series
	comiciEpisodeRe = regexp.MustCompile(`/episodes/([0-9a-f]{13})`)
	comiciSeriesRe  = regexp.MustCompile(`/series/([0-9a-f]{13})`)
	// comiciNumberRe is the number a title starts with: "第34話", "第一服" (a
	// counter), "第七十一話", "EPISODE.01", "0話", "2-①" (a split episode), "#9.5",
	// "＃1" or a bare "1"
	comiciNumberRe = regexp.MustCompile(`^(?:第(` + gvKanji + `|` + gvDigits + `)[話服]|(` + gvDigits + `)話|(` + gvDigits + `)-|[#＃](` + gvDigits + `)|EPISODE[.．](` + gvDigits + `)|(` + gvDigits + `)$)`)
	// comiciWordRe is a part written as a word: "前編" in "第36話前編①"
	comiciWordRe = regexp.MustCompile(`^[前中後]編`)
)

// Comici is a grabber for Comici, the viewer of several Japanese publishers.
// Its listing marks the free episodes, and each one's pages come from the API.
type Comici struct {
	*Grabber
	// byMarkup also detects the hosts not in comiciDomains (NewComiciByMarkup)
	byMarkup   bool
	seriesID   string
	seriesLoad bool
	title      string
	chapters   Filterables
}

// NewComici returns a new Comici grabber, matching the listed hosts.
func NewComici(g *Grabber) *Comici {
	return &Comici{Grabber: g}
}

// NewComiciByMarkup returns a Comici grabber that also matches an episode or
// series URL on any host whose page has the Comici markup.
func NewComiciByMarkup(g *Grabber) *Comici {
	return &Comici{Grabber: g, byMarkup: true}
}

// ComiciChapter is a chapter of a Comici site, identified by its episode ID
type ComiciChapter struct {
	Chapter
}

// comiciPages is the contentsInfo API's reply: the pages and their scrambling
type comiciPages struct {
	TotalPages int `json:"totalPages"`
	Result     []struct {
		ImageURL string `json:"imageUrl"`
		Scramble string `json:"scramble"`
	} `json:"result"`
}

// Test returns true if the URL is on one of the Comici hosts (host check, no
// fetch). The NewComiciByMarkup instance also fetches an episode or series URL
// on another host, once, and checks its markup.
func (c *Comici) Test() (bool, error) {
	for _, domain := range comiciDomains {
		if hostMatchesDomain(c.URL, domain) {
			return true, nil
		}
	}
	if !c.byMarkup || !comiciURLShape(c.URL) {
		return false, nil
	}
	return pageHasMarkup(c.Grabber, c.URL, comiciMarkers...), nil
}

// comiciURLShape reports whether rawURL's path is an episode or a series, the
// two shapes the grabber reads
func comiciURLShape(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (comiciEpisodeRe.MatchString(u.Path) || comiciSeriesRe.MatchString(u.Path))
}

// seriesIdentifier returns the series of the URL: a series URL names it, and an
// episode page names it otherwise
func (c *Comici) seriesIdentifier() (string, error) {
	if c.seriesID != "" {
		return c.seriesID, nil
	}
	if m := comiciSeriesRe.FindStringSubmatch(c.URL); m != nil {
		c.seriesID = m[1]
		return c.seriesID, nil
	}
	m := comiciEpisodeRe.FindStringSubmatch(c.URL)
	if m == nil {
		return "", fmt.Errorf("comici: %s is neither an episode nor a series URL", c.URL)
	}
	doc, err := c.fetchDocument("/episodes/" + m[1])
	if err != nil {
		return "", err
	}
	c.seriesID = doc.Find("[data-series-id]").AttrOr("data-series-id", "")
	if c.seriesID == "" {
		return "", fmt.Errorf("comici: no series on episode %s", m[1])
	}
	return c.seriesID, nil
}

// fetchDocument fetches a page of the site and parses it
func (c *Comici) fetchDocument(path string) (*goquery.Document, error) {
	body, err := http.GetText(http.RequestParams{URL: c.BaseUrl() + path, Referer: c.BaseUrl() + "/"})
	if err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(strings.NewReader(body))
}

// comiciEpisode is an episode as the listing shows it, before its title is split
type comiciEpisode struct {
	raw, id  string
	locked   bool
	number   float64
	numbered bool
}

// fetchSeries reads the series' title and episodes from its listing, once
func (c *Comici) fetchSeries() error {
	if c.seriesLoad {
		return nil
	}

	seriesID, err := c.seriesIdentifier()
	if err != nil {
		return err
	}

	// the listing is paginated, and a page past the last one is a 404
	var episodes []comiciEpisode
	var title string
	for page := 1; ; page++ {
		doc, err := c.fetchDocument(fmt.Sprintf("/series/%s/%d", seriesID, page))
		var statusErr *http.StatusError
		if errors.As(err, &statusErr) && statusErr.Code == 404 {
			break
		}
		if err != nil {
			return err
		}
		if title == "" {
			title = comiciSeries(doc.Find("title").First().Text())
		}
		items := doc.Find(`[data-e2e="eli"]`)
		if items.Length() == 0 {
			break
		}
		items.Each(func(_ int, item *goquery.Selection) {
			link := item.Find(`a[href^="/episodes/"]`).First()
			id := comiciEpisodeRe.FindStringSubmatch(link.AttrOr("href", ""))
			if id == nil {
				return
			}
			episodes = append(episodes, comiciEpisode{
				raw:    item.Find(`[data-e2e="eliTitle"]`).First().Text(),
				id:     id[1],
				locked: !comiciIsFree(item),
			})
		})
	}

	// a trailing part is only a part when another episode shares its number:
	// a lone "25話 サマーキャンプ①" is an arc, not one of two parts
	counts := map[float64]int{}
	for i, e := range episodes {
		_, episodes[i].number, _, episodes[i].numbered = comiciTitle(e.raw, false)
		if episodes[i].numbered {
			counts[episodes[i].number]++
		}
	}
	var chapters Filterables
	for _, e := range episodes {
		name, number, part, numbered := comiciTitle(e.raw, counts[e.number] > 1)
		style := LabelNumbered
		if !numbered {
			style = LabelUnnumbered
		}
		chapters = append(chapters, &ComiciChapter{Chapter{
			Title:    name,
			Number:   number,
			ID:       e.id,
			Locked:   e.locked,
			Language: "ja",
			Label:    Label{Style: style, Part: part},
		}})
	}

	c.title = title
	c.chapters = chapters
	c.seriesLoad = true

	return nil
}

// comiciIsFree reports whether a listed episode is free to read now, by its
// eliFreeBadge: a plain "今なら無料" span is not free (no viewer anonymously).
func comiciIsFree(item *goquery.Selection) bool {
	return item.Find(`[data-e2e="eliFreeBadge"]`).Length() > 0
}

// comiciSeries returns the series name of a page's title, "Name - Authors | Site".
// The site's name and the authors are cut, and an entity is decoded.
func comiciSeries(title string) string {
	title = html.UnescapeString(title)
	if i := strings.LastIndex(title, " | "); i >= 0 {
		title = title[:i]
	}
	if i := strings.LastIndex(title, " - "); i >= 0 {
		title = title[:i]
	}
	return strings.TrimSpace(title)
}

// comiciTitle splits an episode's listed title into its title, number and part,
// and reports whether it found a number. A title without one is kept whole. A
// trailing part ("①") only counts when shared: another episode has the number.
func comiciTitle(raw string, shared bool) (title string, number float64, part string, numbered bool) {
	raw = cleanTitle(raw)
	end, text, ok := gigaviewerGroup(comiciNumberRe, raw)
	if !ok {
		return raw, 0, "", false
	}
	number, ok = gigaviewerNumberOf(text)
	if !ok {
		return raw, 0, "", false
	}

	rest := raw[end:]
	word := comiciWordRe.FindString(rest)
	rest = rest[len(word):]
	if p, r, ok := gigaviewerSplitPart(rest); ok {
		part, rest = word+p, r
	} else {
		part = word
	}
	if part == "" && shared {
		part, rest = gigaviewerTrailingPart(rest)
	}
	return cleanTitle(rest), number, part, true
}

// FetchTitle returns the series title.
func (c *Comici) FetchTitle() (string, error) {
	if err := c.fetchSeries(); err != nil {
		return "", err
	}
	return c.title, nil
}

// FetchChapters returns the series' episodes, locked where the listing doesn't
// show them free.
func (c *Comici) FetchChapters() (Filterables, []error) {
	if err := c.fetchSeries(); err != nil {
		return nil, []error{err}
	}
	return c.chapters, nil
}

// FetchChapter fetches an episode's pages. A locked episode has no viewer on
// its page, so it's reported instead of fetched.
func (c *Comici) FetchChapter(f Filterable) (*Chapter, error) {
	cchap := f.(*ComiciChapter)

	doc, err := c.fetchDocument("/episodes/" + cchap.ID)
	if err != nil {
		return nil, err
	}
	viewer := doc.Find("#comici-viewer")
	viewerID := viewer.AttrOr("data-comici-viewer-id", "")
	if viewerID == "" {
		return nil, fmt.Errorf("%s is paid, only free episodes can be downloaded", ChapterLabel(f))
	}

	info, err := c.fetchPages(viewerID, 1)
	if err != nil {
		return nil, err
	}
	info, err = c.fetchPages(viewerID, info.TotalPages)
	if err != nil {
		return nil, err
	}

	var pages []Page
	for _, p := range info.Result {
		page := Page{
			Number: int64(len(pages) + 1),
			URL:    p.ImageURL,
		}
		if p.Scramble != "" {
			perm, err := comiciPermutation(p.Scramble)
			if err != nil {
				return nil, err
			}
			page.Transform = comiciUnscramble(perm)
		}
		pages = append(pages, page)
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", ChapterLabel(f))
	}

	return &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "ja",
		Pages:      pages,
		ID:         cchap.ID,
	}, nil
}

// fetchPages asks the contentsInfo API for the pages of a viewer, up to page-to
func (c *Comici) fetchPages(viewerID string, pageTo int) (*comiciPages, error) {
	body, err := http.GetText(http.RequestParams{
		URL:     fmt.Sprintf("%s/api/book/contentsInfo?comici-viewer-id=%s&user-id=&page-from=0&page-to=%d", c.BaseUrl(), viewerID, pageTo),
		Referer: c.BaseUrl() + "/",
	})
	if err != nil {
		return nil, err
	}

	var info comiciPages
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		return nil, fmt.Errorf("comici: reading pages: %w", err)
	}
	return &info, nil
}

// comiciPermutation reads a page's scramble, the 16 tiles of its 4x4 grid
func comiciPermutation(scramble string) ([]int, error) {
	var perm []int
	if err := json.Unmarshal([]byte(scramble), &perm); err != nil {
		return nil, fmt.Errorf("comici: reading scramble %q: %w", scramble, err)
	}
	if len(perm) != 16 {
		return nil, fmt.Errorf("comici: scramble %q has %d tiles, not 16", scramble, len(perm))
	}
	return perm, nil
}

// comiciUnscramble undoes a page's scrambling. Tile i of the 4x4 grid, numbered
// column by column, is drawn from tile perm[i] of the scrambled image. The
// strips past the last whole tile stay put.
func comiciUnscramble(perm []int) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		src, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("comici: decoding scrambled image: %w", err)
		}

		bounds := src.Bounds()
		tileW := bounds.Dx() / 4
		tileH := bounds.Dy() / 4

		dst := image.NewRGBA(bounds)
		draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
		for i, from := range perm {
			to := image.Pt(bounds.Min.X+i/4*tileW, bounds.Min.Y+i%4*tileH)
			origin := image.Pt(bounds.Min.X+from/4*tileW, bounds.Min.Y+from%4*tileH)
			draw.Draw(dst, image.Rectangle{Min: to, Max: to.Add(image.Pt(tileW, tileH))}, src, origin, draw.Src)
		}

		buf := &bytes.Buffer{}
		if err := jpeg.Encode(buf, dst, &jpeg.Options{Quality: 95}); err != nil {
			return nil, fmt.Errorf("comici: encoding descrambled image: %w", err)
		}

		return buf.Bytes(), nil
	}
}
