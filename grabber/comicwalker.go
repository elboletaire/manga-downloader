// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/elboletaire/manga-downloader/http"
)

// comicWalkerDomain is KADOKAWA's KadoComi reader (カドコミ)
const comicWalkerDomain = "comic-walker.com"

var (
	// comicWalkerWorkRe is the work code in a series or episode URL: "KC_000061_S"
	comicWalkerWorkRe = regexp.MustCompile(`/detail/([^/?#]+)`)
	// comicWalkerNumberRe is an episode title's number, "第3話" or "第１集" (a volume's first episode)
	comicWalkerNumberRe = regexp.MustCompile(`^第(` + gvKanji + `|` + gvDigits + `)[話集]`)
	// comicWalkerPartRe is the 前編, 中編 or 後編 that a split episode's number ends with
	comicWalkerPartRe = regexp.MustCompile(`^(?:前編|中編|後編)`)
)

// ComicWalker is a grabber for KadoComi: the work's JSON API lists its episodes
// and the viewer API gives each one's pages, XOR-scrambled with a key from the
// response (drmMode "xor"). The listing marks episodes that aren't open for free.
type ComicWalker struct {
	*Grabber
	loaded bool
	title  string
	// chapters are the episodes of the work, unlocked or not
	chapters Filterables
}

// NewComicWalker returns a new ComicWalker grabber.
func NewComicWalker(g *Grabber) *ComicWalker {
	return &ComicWalker{Grabber: g}
}

// ComicWalkerChapter is an episode of KadoComi, identified by its episode ID
type ComicWalkerChapter struct {
	Chapter
}

// comicWalkerWork is the work JSON: its title and the first and latest episodes
type comicWalkerWork struct {
	Work struct {
		Title string `json:"title"`
	} `json:"work"`
	FirstEpisodes  comicWalkerEpisodes `json:"firstEpisodes"`
	LatestEpisodes comicWalkerEpisodes `json:"latestEpisodes"`
}

type comicWalkerEpisodes struct {
	Result []comicWalkerEpisode `json:"result"`
}

// comicWalkerEpisode is an episode in the work JSON. IsActive is whether it's
// open for free right now.
type comicWalkerEpisode struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	ServiceID string `json:"serviceId"`
	IsActive  bool   `json:"isActive"`
}

// comicWalkerViewer is the viewer JSON: the pages of an episode, none when it isn't open
type comicWalkerViewer struct {
	Manuscripts []struct {
		DrmMode     string `json:"drmMode"`
		DrmHash     string `json:"drmHash"`
		DrmImageURL string `json:"drmImageUrl"`
	} `json:"manuscripts"`
}

// Test returns true if the URL is on comic-walker.com (host check, no fetch).
func (c *ComicWalker) Test() (bool, error) {
	return hostMatchesDomain(c.URL, comicWalkerDomain), nil
}

// workCode returns the work of a series or episode URL, "KC_000061_S"
func (c *ComicWalker) workCode() (string, error) {
	m := comicWalkerWorkRe.FindStringSubmatch(c.URL)
	if m == nil {
		return "", fmt.Errorf("comicwalker: %s is neither a series nor an episode URL", c.URL)
	}
	return m[1], nil
}

// fetchWork reads the work's title and episodes once. An episode can be
// reached from its own URL, so the work comes from the URL's path.
func (c *ComicWalker) fetchWork() error {
	if c.loaded {
		return nil
	}

	code, err := c.workCode()
	if err != nil {
		return err
	}

	body, err := http.GetText(http.RequestParams{URL: c.BaseUrl() + "/api/contents/details/work?workCode=" + code, Referer: c.BaseUrl() + "/"})
	if err != nil {
		return err
	}

	var work comicWalkerWork
	if err := json.Unmarshal([]byte(body), &work); err != nil {
		return err
	}

	// the episode lists overlap, so each episode is kept once
	seen := map[string]bool{}
	var chapters Filterables
	for _, e := range append(work.FirstEpisodes.Result, work.LatestEpisodes.Result...) {
		// the trial copies (web_trial) repeat the first episodes under other numbers,
		// and the promotions (type pr) aren't episodes
		if seen[e.ID] || e.Type != "normal" || e.ServiceID == "web_trial" {
			continue
		}
		seen[e.ID] = true

		title, number, part := comicWalkerTitle(e.Title)
		chapters = append(chapters, &ComicWalkerChapter{Chapter{
			Title:    title,
			Number:   number,
			ID:       e.ID,
			Locked:   !e.IsActive,
			Language: "ja",
			Label:    Label{Style: LabelNumbered, Part: part},
		}})
	}

	c.title = cleanTitle(work.Work.Title)
	c.chapters = chapters
	c.loaded = true

	return nil
}

// comicWalkerTitle splits an episode title into its title (empty when there is
// none), number and part: "第3話後編" is number 3, part "後編"
func comicWalkerTitle(raw string) (title string, number float64, part string) {
	raw = cleanTitle(raw)
	m := comicWalkerNumberRe.FindStringSubmatchIndex(raw)
	if m == nil {
		return raw, 0, ""
	}
	number, ok := gigaviewerNumberOf(raw[m[2]:m[3]])
	if !ok {
		return raw, 0, ""
	}

	rest := raw[m[1]:]
	if p := comicWalkerPartRe.FindString(rest); p != "" {
		part, rest = p, rest[len(p):]
	}
	return cleanTitle(rest), number, part
}

// comicWalkerKey returns the 8-byte XOR key of a page: the first 16 hex digits of its drmHash
func comicWalkerKey(hash string) ([]byte, error) {
	if len(hash) < 16 {
		return nil, fmt.Errorf("comicwalker: drmHash %q is too short for a key", hash)
	}
	key, err := hex.DecodeString(hash[:16])
	if err != nil {
		return nil, fmt.Errorf("comicwalker: drmHash %q is not hex: %w", hash, err)
	}
	return key, nil
}

// comicWalkerXOR returns the Transform that undoes a page's XOR scrambling, the
// key repeated over the image bytes
func comicWalkerXOR(key []byte) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		out := make([]byte, len(data))
		for i, b := range data {
			out[i] = b ^ key[i%len(key)]
		}
		return out, nil
	}
}

// FetchTitle returns the work's title.
func (c *ComicWalker) FetchTitle() (string, error) {
	if err := c.fetchWork(); err != nil {
		return "", err
	}
	return c.title, nil
}

// FetchChapters returns the work's episodes, locked where the listing doesn't open them.
func (c *ComicWalker) FetchChapters() (Filterables, []error) {
	if err := c.fetchWork(); err != nil {
		return nil, []error{err}
	}
	return c.chapters, nil
}

// FetchChapter fetches an episode's pages. An episode the viewer returns no
// pages for isn't open for free.
func (c *ComicWalker) FetchChapter(f Filterable) (*Chapter, error) {
	cchap := f.(*ComicWalkerChapter)

	body, err := http.GetText(http.RequestParams{URL: c.BaseUrl() + "/api/contents/viewer?episodeId=" + cchap.ID + "&imageSizeType=width:1284", Referer: c.BaseUrl() + "/"})
	if err != nil {
		return nil, err
	}

	var viewer comicWalkerViewer
	if err := json.Unmarshal([]byte(body), &viewer); err != nil {
		return nil, err
	}
	if len(viewer.Manuscripts) == 0 {
		return nil, fmt.Errorf("%s is paid, only free episodes can be downloaded", ChapterLabel(f))
	}

	var pages []Page
	for _, m := range viewer.Manuscripts {
		page := Page{
			Number: int64(len(pages) + 1),
			URL:    m.DrmImageURL,
		}
		switch m.DrmMode {
		case "raw":
		case "xor":
			key, err := comicWalkerKey(m.DrmHash)
			if err != nil {
				return nil, err
			}
			page.Transform = comicWalkerXOR(key)
		default:
			return nil, fmt.Errorf("comicwalker: unsupported image scrambling %q", m.DrmMode)
		}
		pages = append(pages, page)
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
