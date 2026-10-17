// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// gigaviewerDomains are the hosts of GigaViewer, Hatena's reader platform.
// List another host here to serve it too: its episode and feed URLs are the
// same.
var gigaviewerDomains = []string{
	"shonenjumpplus.com",
	"tonarinoyj.jp",
	"comic-days.com",
	"kuragebunch.com",
	"magcomi.com",
	"comic-gardo.com",
	"comic-zenon.com",
	"comic-action.com",
	"comic-trail.com",
	"comic-earthstar.com",
	"comic-ogyaaa.com",
	"sunday-webry.com",
	"comic-seasons.com",
	"comic-y-ours.com",
	"ourfeel.jp",
	"feelweb.jp",
	"ichicomi.com",
	"mangatime-square.com",
}

// gvDigits, gvKanji and gvBreak are the number pieces of a title: digits (with
// a decimal), kanji, and a "＆13" that follows the number it leads with
const (
	gvDigits = `[0-9０-９]+(?:[.．][0-9０-９]+)?`
	gvKanji  = `[〇零一二三四五六七八九十百千]+`
	gvBreak  = `(?:[＆&][0-9０-９]+)?`
)

var (
	gigaviewerEpisodeRe = regexp.MustCompile(`/episode/(\d+)`)
	gigaviewerFeedRe    = regexp.MustCompile(`/(?:rss|atom)/series/(\d+)`)
	// gigaviewerBracketRe is a title's leading bracket, [], ［］ or 【】
	gigaviewerBracketRe = regexp.MustCompile(`^[\[［【]([^\]］】]*)[\]］】]`)
	// gigaviewerInnerRe is a number with its 話, 回 or 章: "第五十七話", "第12＆13話", "26.5話"
	gigaviewerInnerRe = regexp.MustCompile(`(?:第(` + gvKanji + `|` + gvDigits + `)|(` + gvDigits + `))` + gvBreak + `[話回章]`)
	// gigaviewerBareRe is gigaviewerInnerRe anchored at the start of a title
	gigaviewerBareRe = regexp.MustCompile(`^` + gigaviewerInnerRe.String())
	// gigaviewerSymbolRe is the "≪＃432≫" and "最終話《＃432》" markers
	gigaviewerSymbolRe = regexp.MustCompile(`^(?:最終話)?[《≪]＃([0-9０-９]+)[》≫]`)
	// gigaviewerHashRe is the "#8" marker
	gigaviewerHashRe = regexp.MustCompile(`^#([0-9０-９]+)`)
	// gigaviewerLatinRe is a "Op.7-1" or "Season01" marker; the part is the "-1"
	gigaviewerLatinRe = regexp.MustCompile(`^(?:Op|Season|Case|Episode|Profile)[ .]?([0-9０-９]+)(?:-([0-9０-９]+))?`)
	// gigaviewerWholeRe is a title that is only its number
	gigaviewerWholeRe = regexp.MustCompile(`^([0-9０-９]+)$`)
	// gigaviewerNoticeRe matches the feed's announcements and ads, which aren't episodes
	// (only for a title without a number: "第43話 告知" is a story episode)
	gigaviewerNoticeRe = regexp.MustCompile(`お知らせ|休載|告知`)
	// gigaviewerHalfRe is the 前編/中編/後編 that follows a Latin marker, "Season26 前編"
	gigaviewerHalfRe = regexp.MustCompile(`^[` + gigaviewerSeparators + `]*([前中後]編)`)
	// gigaviewerPartRe is a leading "(2)" or "（２）" after the number
	gigaviewerPartRe = regexp.MustCompile(`^[（(]([0-9０-９]+)[）)]`)
	// gigaviewerSeparators are what sits between a number and a part, or a part and nothing
	gigaviewerSeparators = " 　～~-・"
	// gigaviewerDigits makes the full-width digits of a title ASCII
	gigaviewerDigits = strings.NewReplacer("．", ".", "０", "0", "１", "1", "２", "2", "３", "3", "４", "4",
		"５", "5", "６", "6", "７", "7", "８", "8", "９", "9")
)

// Gigaviewer is a grabber for GigaViewer sites: episode pages embed their JSON
// (pages, series, whether public), and the RSS feed lists the series' episodes.
// Pages scrambled in tiles ("baku") are undone in Transform.
type Gigaviewer struct {
	*Grabber
	seriesID   string
	feedLoaded bool
	title      string
	chapters   Filterables
}

// NewGigaviewer returns a new Gigaviewer grabber.
func NewGigaviewer(g *Grabber) *Gigaviewer {
	return &Gigaviewer{Grabber: g}
}

// GigaviewerChapter is a chapter of a GigaViewer site, identified by its episode ID
type GigaviewerChapter struct {
	Chapter
}

// gigaviewerEpisode is the episode JSON embedded in an episode page
type gigaviewerEpisode struct {
	ID       string `json:"id"`
	IsPublic bool   `json:"isPublic"`
	// PageStructure is null for an episode the reader can't open
	PageStructure *struct {
		ChoJuGiga string `json:"choJuGiga"`
		Pages     []struct {
			Type string `json:"type"`
			Src  string `json:"src"`
		} `json:"pages"`
	} `json:"pageStructure"`
	Series struct {
		ID string `json:"id"`
	} `json:"series"`
}

// gigaviewerFeed is a series' RSS feed, newest episode first
type gigaviewerFeed struct {
	Channel struct {
		Title string `xml:"title"`
		Items []struct {
			Title string `xml:"title"`
			Link  string `xml:"link"`
		} `xml:"item"`
	} `xml:"channel"`
}

// Test returns true if the URL is on one of the GigaViewer hosts (host check, no fetch).
func (g *Gigaviewer) Test() (bool, error) {
	for _, domain := range gigaviewerDomains {
		if hostMatchesDomain(g.URL, domain) {
			return true, nil
		}
	}
	return false, nil
}

// seriesIdentifier returns the series of the URL: a feed URL names it, and
// an episode page's JSON names it otherwise
func (g *Gigaviewer) seriesIdentifier() (string, error) {
	if g.seriesID != "" {
		return g.seriesID, nil
	}
	if m := gigaviewerFeedRe.FindStringSubmatch(g.URL); m != nil {
		g.seriesID = m[1]
		return g.seriesID, nil
	}
	m := gigaviewerEpisodeRe.FindStringSubmatch(g.URL)
	if m == nil {
		return "", fmt.Errorf("gigaviewer: %s is neither an episode nor a series URL", g.URL)
	}
	episode, err := g.fetchEpisode(m[1])
	if err != nil {
		return "", err
	}
	g.seriesID = episode.Series.ID
	return g.seriesID, nil
}

// fetchEpisode fetches an episode page and returns the JSON it embeds
func (g *Gigaviewer) fetchEpisode(id string) (*gigaviewerEpisode, error) {
	body, err := http.GetText(http.RequestParams{URL: g.BaseUrl() + "/episode/" + id, Referer: g.BaseUrl() + "/"})
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	raw := doc.Find("#episode-json").AttrOr("data-value", "")
	if raw == "" {
		return nil, fmt.Errorf("gigaviewer: no episode data on episode %s", id)
	}

	var data struct {
		ReadableProduct gigaviewerEpisode `json:"readableProduct"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}

	return &data.ReadableProduct, nil
}

// fetchFeed reads the series' episodes from its feed and which of them are
// free, once. Its title is the feed's channel title without the site's name.
func (g *Gigaviewer) fetchFeed() error {
	if g.feedLoaded {
		return nil
	}

	seriesID, err := g.seriesIdentifier()
	if err != nil {
		return err
	}

	all, err := g.readFeed(seriesID, "")
	if err != nil {
		return err
	}
	free, err := g.readFeed(seriesID, "?free_only=1")
	if err != nil {
		return err
	}

	freeIDs := []string{}
	for _, item := range free.Channel.Items {
		freeIDs = append(freeIDs, gigaviewerEpisodeID(item.Link))
	}

	series := gigaviewerSeries(all.Channel.Title)

	// the feed lists the newest first, and core sorts by number anyway
	var chapters Filterables
	for i := len(all.Channel.Items) - 1; i >= 0; i-- {
		item := all.Channel.Items[i]
		id := gigaviewerEpisodeID(item.Link)
		if id == "" {
			continue
		}
		title, number, part := gigaviewerTitle(item.Title, series)
		if gigaviewerIsNotice(item.Title, number) {
			continue
		}
		chapters = append(chapters, &GigaviewerChapter{Chapter{
			Title:    title,
			Number:   number,
			ID:       id,
			Locked:   !slices.Contains(freeIDs, id),
			Language: "ja",
			Label:    Label{Style: LabelNumbered, Part: part},
		}})
	}

	g.title = series
	g.chapters = chapters
	g.feedLoaded = true

	return nil
}

// readFeed fetches the series' RSS feed, with query appended to its URL
func (g *Gigaviewer) readFeed(seriesID, query string) (*gigaviewerFeed, error) {
	body, err := http.GetText(http.RequestParams{URL: g.BaseUrl() + "/rss/series/" + seriesID + query, Referer: g.BaseUrl() + "/"})
	if err != nil {
		return nil, err
	}

	var feed gigaviewerFeed
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		return nil, err
	}

	return &feed, nil
}

// gigaviewerEpisodeID returns the episode ID at the end of an episode link, "" if none
func gigaviewerEpisodeID(link string) string {
	m := gigaviewerEpisodeRe.FindStringSubmatch(link)
	if m == nil {
		return ""
	}
	return m[1]
}

// gigaviewerSeries returns the series name of a feed's channel title, the
// parentheses it ends with. Matching from the closing bracket keeps a name
// with parentheses of its own whole.
func gigaviewerSeries(channel string) string {
	channel = strings.TrimSpace(channel)
	if !strings.HasSuffix(channel, "）") {
		return channel
	}
	depth := 0
	for i := len(channel); i > 0; {
		r, size := utf8.DecodeLastRuneInString(channel[:i])
		i -= size
		switch r {
		case '）':
			depth++
		case '（':
			depth--
			if depth == 0 {
				return channel[i+size : len(channel)-len("）")]
			}
		}
	}
	return channel
}

// gigaviewerIsNotice reports whether a feed item is an announcement or ad. A
// numbered title only is one when its bracket says so: "[休載イラスト＋第2回…]".
func gigaviewerIsNotice(raw string, number float64) bool {
	if number == 0 {
		return gigaviewerNoticeRe.MatchString(raw)
	}
	loc := gigaviewerBracketRe.FindStringSubmatch(raw)
	return loc != nil && gigaviewerNoticeRe.MatchString(loc[1])
}

// gigaviewerTitle splits a feed title into its title (empty when it's only the
// series name), number and part. The number must start the title, so a
// subtitle's numbers are never read; a title without one is numbered 0.
func gigaviewerTitle(raw, series string) (title string, number float64, part string) {
	raw = cleanTitle(raw)
	end, text, part, ok := gigaviewerMarker(raw)
	if !ok {
		return raw, 0, ""
	}
	number, ok = gigaviewerNumberOf(text)
	if !ok {
		return raw, 0, ""
	}

	title = cleanTitle(raw[end:])
	if part == "" {
		if p, rest, ok := gigaviewerSplitPart(title); ok {
			part, title = p, rest
		}
	}
	if title == series {
		title = ""
	}
	return title, number, part
}

// gigaviewerSplitPart reads the "①" or "（2）" that follows an episode number as
// the part of a split episode, and returns the rest of the title
func gigaviewerSplitPart(title string) (part, rest string, ok bool) {
	r, size := utf8.DecodeRuneInString(title)
	if p, ok := gigaviewerCircled(r); ok {
		return p, cleanTitle(title[size:]), true
	}
	if m := gigaviewerPartRe.FindStringSubmatch(title); m != nil {
		return gigaviewerDigits.Replace(m[1]), cleanTitle(title[len(m[0]):]), true
	}
	return "", title, false
}

// gigaviewerCircled returns the number a circled digit ("①" to "⑳") stands for
func gigaviewerCircled(r rune) (string, bool) {
	if r < '①' || r > '⑳' {
		return "", false
	}
	return strconv.Itoa(int(r-'①') + 1), true
}

// gigaviewerMarker reads the number a title starts with: where it ends, its
// text and the part after it ("前編" in 【第112話前編】, "1" in Op.7-1)
func gigaviewerMarker(raw string) (end int, text, part string, ok bool) {
	if loc := gigaviewerBracketRe.FindStringSubmatchIndex(raw); loc != nil {
		inner := gigaviewerDigits.Replace(raw[loc[2]:loc[3]])
		_, text, ok := gigaviewerGroup(gigaviewerInnerRe, inner)
		if !ok {
			return 0, "", "", false
		}
		part = strings.Trim(gigaviewerInnerRe.ReplaceAllString(inner, ""), gigaviewerSeparators)
		if r, size := utf8.DecodeRuneInString(part); size == len(part) {
			if p, ok := gigaviewerCircled(r); ok {
				part = p
			}
		}
		return loc[1], text, part, true
	}
	for _, re := range []*regexp.Regexp{gigaviewerBareRe, gigaviewerSymbolRe, gigaviewerHashRe, gigaviewerWholeRe} {
		if end, text, ok := gigaviewerGroup(re, raw); ok {
			return end, text, "", true
		}
	}
	if loc := gigaviewerLatinRe.FindStringSubmatchIndex(raw); loc != nil {
		part := ""
		if loc[4] >= 0 {
			part = gigaviewerDigits.Replace(raw[loc[4]:loc[5]])
		}
		end = loc[1]
		if part == "" {
			if h := gigaviewerHalfRe.FindStringSubmatchIndex(raw[end:]); h != nil {
				part, end = raw[end+h[2]:end+h[3]], end+h[1]
			}
		}
		return end, raw[loc[2]:loc[3]], part, true
	}
	return 0, "", "", false
}

// gigaviewerGroup returns where re's match at the start of s ends, and the
// text of its first group that matched
func gigaviewerGroup(re *regexp.Regexp, s string) (end int, text string, ok bool) {
	loc := re.FindStringSubmatchIndex(s)
	if loc == nil {
		return 0, "", false
	}
	for k := 2; k+1 < len(loc); k += 2 {
		if loc[k] >= 0 {
			return loc[1], s[loc[k]:loc[k+1]], true
		}
	}
	return loc[1], "", true
}

// gigaviewerNumberOf reads a number written in digits, decimals or kanji
func gigaviewerNumberOf(text string) (float64, bool) {
	text = gigaviewerDigits.Replace(text)
	if n, err := strconv.ParseFloat(text, 64); err == nil {
		return n, true
	}
	n, ok := gigaviewerKanjiNumber(text)
	return float64(n), ok
}

var gigaviewerKanjiDigits = map[rune]int{'〇': 0, '零': 0, '一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
var gigaviewerKanjiUnits = map[rune]int{'十': 10, '百': 100, '千': 1000}

// gigaviewerKanjiNumber reads a number written in kanji, "五十七" as 57
func gigaviewerKanjiNumber(text string) (int, bool) {
	total, cur := 0, 0
	for _, r := range text {
		if d, ok := gigaviewerKanjiDigits[r]; ok {
			cur = d
			continue
		}
		unit, ok := gigaviewerKanjiUnits[r]
		if !ok {
			return 0, false
		}
		if cur == 0 {
			cur = 1
		}
		total += cur * unit
		cur = 0
	}
	return total + cur, text != ""
}

// FetchTitle returns the series title.
func (g *Gigaviewer) FetchTitle() (string, error) {
	if err := g.fetchFeed(); err != nil {
		return "", err
	}
	return g.title, nil
}

// FetchChapters returns the series' episodes, locked where the free view
// doesn't list them.
func (g *Gigaviewer) FetchChapters() (Filterables, []error) {
	if err := g.fetchFeed(); err != nil {
		return nil, []error{err}
	}
	return g.chapters, nil
}

// FetchChapter fetches an episode's pages. Each page is listed once, in reading order.
func (g *Gigaviewer) FetchChapter(f Filterable) (*Chapter, error) {
	cchap := f.(*GigaviewerChapter)

	episode, err := g.fetchEpisode(cchap.ID)
	if err != nil {
		return nil, err
	}
	if !episode.IsPublic || episode.PageStructure == nil {
		return nil, fmt.Errorf("%s is paid, only free episodes can be downloaded", ChapterLabel(f))
	}

	scramble := episode.PageStructure.ChoJuGiga
	if scramble != "" && scramble != "none" && scramble != "baku" {
		return nil, fmt.Errorf("gigaviewer: unsupported image scrambling %q", scramble)
	}

	var pages []Page
	for _, p := range episode.PageStructure.Pages {
		if p.Type != "main" {
			continue
		}
		page := Page{
			Number: int64(len(pages) + 1),
			URL:    p.Src,
		}
		if scramble == "baku" {
			page.Transform = gigaviewerUnscramble
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

// gigaviewerUnscramble undoes "baku" scrambling: the 4x4 grid of tiles, each
// floor(size/32)*8 pixels, is transposed and the strips past it stay put.
func gigaviewerUnscramble(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gigaviewer: decoding scrambled image: %w", err)
	}

	bounds := src.Bounds()
	cellW := bounds.Dx() / 32 * 8
	cellH := bounds.Dy() / 32 * 8

	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
	for row := range 4 {
		for col := range 4 {
			from := image.Pt(bounds.Min.X+col*cellW, bounds.Min.Y+row*cellH)
			to := image.Pt(bounds.Min.X+row*cellW, bounds.Min.Y+col*cellH)
			draw.Draw(dst, image.Rectangle{Min: to, Max: to.Add(image.Pt(cellW, cellH))}, src, from, draw.Src)
		}
	}

	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, dst, &jpeg.Options{Quality: 95}); err != nil {
		return nil, fmt.Errorf("gigaviewer: encoding descrambled image: %w", err)
	}

	return buf.Bytes(), nil
}
