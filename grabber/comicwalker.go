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
	// comicWalkerPartRe is the 前編, 中編 or 後編 that a split episode's number ends with
	comicWalkerPartRe = regexp.MustCompile(`^(?:前編|中編|後編)`)
	// comicWalkerChapterRe is the "Chapter1" some English titles start with
	comicWalkerChapterRe = regexp.MustCompile(`^Chapter ?([0-9]+)`)
	// comicWalkerVolumeRe is the number a volume's title ends with
	comicWalkerVolumeRe = regexp.MustCompile(`[0-9０-９]+$`)
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

// comicWalkerWork is the work JSON: its title, the first and latest episodes and
// the volumes, which list the episodes of a work with no serial ones
type comicWalkerWork struct {
	Work struct {
		Title string `json:"title"`
	} `json:"work"`
	FirstEpisodes  comicWalkerEpisodes `json:"firstEpisodes"`
	LatestEpisodes comicWalkerEpisodes `json:"latestEpisodes"`
	Comics         struct {
		Result []struct {
			Title    string               `json:"title"`
			Episodes []comicWalkerEpisode `json:"episodes"`
		} `json:"result"`
	} `json:"comics"`
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

	// the serial episodes come first; a work with none lists its episodes under
	// its volumes, numbered by the volume's title
	episodes := append(work.FirstEpisodes.Result, work.LatestEpisodes.Result...)
	volumes := map[string]int{}
	if len(episodes) == 0 {
		for _, comic := range work.Comics.Result {
			for _, e := range comic.Episodes {
				episodes = append(episodes, e)
				volumes[e.ID] = comicWalkerVolume(comic.Title)
			}
		}
	}

	// the lists overlap, so each episode is kept once; promotions (type pr) aren't episodes
	seen := map[string]bool{}
	var chapters Filterables
	for _, e := range episodes {
		if seen[e.ID] || e.Type == "pr" {
			continue
		}
		seen[e.ID] = true

		title, number, part := comicWalkerTitle(e.Title)
		chapters = append(chapters, &ComicWalkerChapter{Chapter{
			Title:    title,
			Number:   number,
			ID:       e.ID,
			Volume:   volumes[e.ID],
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
	title, number, part = gigaviewerTitle(raw, "")
	// gigaviewerTitle takes the 前編 after a number as the title
	if part == "" {
		if p := comicWalkerPartRe.FindString(title); p != "" {
			part, title = p, cleanTitle(title[len(p):])
		}
	}
	if m := comicWalkerChapterRe.FindStringSubmatch(title); number == 0 && m != nil {
		number, _ = gigaviewerNumberOf(m[1])
		title = cleanTitle(title[len(m[0]):])
	}
	return title, number, part
}

// comicWalkerVolume returns the volume a comic's title ends with ("…　3" is 3), 0 if none
func comicWalkerVolume(title string) int {
	m := comicWalkerVolumeRe.FindString(cleanTitle(title))
	if m == "" {
		return 0
	}
	n, _ := gigaviewerNumberOf(m)
	return int(n)
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
