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

var (
	gigaviewerEpisodeRe = regexp.MustCompile(`/episode/(\d+)`)
	gigaviewerFeedRe    = regexp.MustCompile(`/(?:rss|atom)/series/(\d+)`)
	// gigaviewerMarkerRe is the number marker a title starts with: a bracket
	// "[139話 後編]", or a bare "第10話", "第655回", "≪＃432≫"
	gigaviewerMarkerRe = regexp.MustCompile(`^(?:\[([^\]]*)\]|(?:最終話)?[《≪]＃([0-9０-９]+)[》≫]|第([0-9０-９.]+)[話回章])`)
	// gigaviewerEpisodeNumRe reads the episode number out of a bracket
	gigaviewerEpisodeNumRe = regexp.MustCompile(`第?([0-9]+(?:\.[0-9]+)?)話`)
	// gigaviewerSeriesRe takes the series name out of the feed's channel
	// title, "少年ジャンプ＋（SERIES）": the last parenthesised part, as a
	// subtitle may hold parentheses of its own
	gigaviewerSeriesRe = regexp.MustCompile(`（([^（）]+)）$`)
	// gigaviewerDigits makes the full-width digits of a title ASCII
	gigaviewerDigits = strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4",
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
// whole title when it has no parenthesised name
func gigaviewerSeries(channel string) string {
	channel = strings.TrimSpace(channel)
	if m := gigaviewerSeriesRe.FindStringSubmatch(channel); m != nil {
		return m[1]
	}
	return channel
}

// gigaviewerTitle splits a feed title, "[139話 後編]SERIES" or "第2話 SUBTITLE",
// into the episode's title (empty when it's only the series name), its number
// and, for a bracket, its part (the 前編/後編 in it). A title without a number
// marker is kept as it is, numbered 0.
func gigaviewerTitle(raw, series string) (title string, number float64, part string) {
	raw = cleanTitle(raw)
	m := gigaviewerMarkerRe.FindStringSubmatchIndex(raw)
	if m == nil {
		return raw, 0, ""
	}

	var num string
	if m[2] >= 0 {
		bracket := gigaviewerDigits.Replace(raw[m[2]:m[3]])
		nm := gigaviewerEpisodeNumRe.FindStringSubmatch(bracket)
		if nm == nil {
			return raw, 0, ""
		}
		num = nm[1]
		part = strings.Trim(gigaviewerEpisodeNumRe.ReplaceAllString(bracket, ""), " 　～~-・")
	} else if m[4] >= 0 {
		num = gigaviewerDigits.Replace(raw[m[4]:m[5]])
	} else {
		num = gigaviewerDigits.Replace(raw[m[6]:m[7]])
	}
	number, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return raw, 0, ""
	}

	title = cleanTitle(raw[m[1]:])
	if title == series {
		title = ""
	}
	return title, number, part
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
