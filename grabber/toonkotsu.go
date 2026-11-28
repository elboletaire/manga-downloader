// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// Toonkotsu is a grabber for toonkotsu.com. Its HTML is behind a Cloudflare
// challenge but its JSON API and CDN aren't, so each release is listed and
// fetched by id with its group; pages are AES-256-CTR, undone in Page.Transform.
type Toonkotsu struct {
	*Grabber
	series *toonkotsuSeries
}

func NewToonkotsu(g *Grabber) *Toonkotsu {
	return &Toonkotsu{Grabber: g}
}

// ToonkotsuChapter represents a Toonkotsu release; its ID is the release's id
type ToonkotsuChapter struct {
	Chapter
}

// toonkotsuHostRe matches toonkotsu.com and its subdomains
var toonkotsuHostRe = regexp.MustCompile(`(?i)(^|\.)toonkotsu\.com$`)

// toonkotsuPathRe takes the series code from /title/{code}-{slug}, which
// chapter URLs (/title/{code}-{slug}/{id}-chapter-N) share with series URLs
var toonkotsuPathRe = regexp.MustCompile(`^/title/([A-Za-z0-9]+)(?:-[^/]*)?(?:/|$)`)

// toonkotsuTitleRe strips the "Chapter N" the label starts with, and the
// separator after it
var toonkotsuTitleRe = regexp.MustCompile(`(?i)^chapter\s+[\d.]+\s*(?:[:\-–—]\s*)?`)

// toonkotsuVolumeTitleRe matches a title that only names a volume, which the
// volume already says
var toonkotsuVolumeTitleRe = regexp.MustCompile(`(?i)^vol(?:ume)?\.?\s*\d+$`)

// Test returns true if the URL is a toonkotsu.com URL. It only checks the
// hostname (no fetch) so it can be tried early without extra requests.
func (t *Toonkotsu) Test() (bool, error) {
	u, err := url.Parse(t.URL)
	if err != nil {
		return false, nil
	}

	return toonkotsuHostRe.MatchString(u.Hostname()), nil
}

// FetchTitle fetches and returns the manga title
func (t *Toonkotsu) FetchTitle() (string, error) {
	series, err := t.fetchSeries()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(series.Series.Title), nil
}

// FetchChapters returns every release of every chapter. An alt is another
// release of its row's chapter, so it takes the row's number and volume.
func (t *Toonkotsu) FetchChapters() (Filterables, []error) {
	series, err := t.fetchSeries()
	if err != nil {
		return nil, []error{err}
	}

	var chapters Filterables
	for _, row := range series.Chapters {
		vol := int(row.Vol)
		chapters = append(chapters, toonkotsuChapter(row.toonkotsuRelease, row.N, vol))
		for _, alt := range row.Alts {
			chapters = append(chapters, toonkotsuChapter(alt, row.N, vol))
		}
	}

	return chapters, nil
}

// FetchChapter fetches a release and its pages
func (t *Toonkotsu) FetchChapter(f Filterable) (*Chapter, error) {
	tc := f.(*ToonkotsuChapter)

	origin, _, err := toonkotsuSeriesCode(t.URL)
	if err != nil {
		return nil, err
	}

	uri := origin + "/api/manga/chapter/" + tc.ID
	page := &toonkotsuPages{}
	if err := toonkotsuGetJSON(uri, origin+"/", page); err != nil {
		return nil, err
	}
	if strconv.FormatInt(page.Chapter.ID, 10) != tc.ID {
		return nil, fmt.Errorf("toonkotsu: %s answered release %d", uri, page.Chapter.ID)
	}
	if len(page.Chapter.Pages) == 0 {
		return nil, fmt.Errorf("toonkotsu: no pages in %s", uri)
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(len(page.Chapter.Pages)),
		Language:   "en",
		ID:         tc.ID,
		Volume:     tc.Volume,
	}
	chapter.SetGroups(tc.GetGroups()...)
	for i, p := range page.Chapter.Pages {
		pg := Page{
			Number: int64(i + 1),
			URL:    toonkotsuPageURL(origin, p.Src),
		}
		if p.K != "" && p.N != "" {
			pg.Transform, err = toonkotsuDecrypter(p.K, p.N)
			if err != nil {
				return nil, err
			}
		}
		chapter.Pages = append(chapter.Pages, pg)
	}

	return chapter, nil
}

// fetchSeries fetches and caches the series and its full release list
func (t *Toonkotsu) fetchSeries() (*toonkotsuSeries, error) {
	if t.series != nil {
		return t.series, nil
	}

	origin, code, err := toonkotsuSeriesCode(t.URL)
	if err != nil {
		return nil, err
	}

	series := &toonkotsuSeries{}
	seriesURL := origin + "/cdn/v1/series/" + code
	if err := toonkotsuGetJSON(seriesURL, origin+"/", series); err != nil {
		return nil, err
	}
	if series.Series.Title == "" {
		return nil, fmt.Errorf("toonkotsu: no series in %s", seriesURL)
	}

	t.series = series

	return series, nil
}

// toonkotsuGetJSON GETs uri and decodes its JSON into v. A Cloudflare challenge
// answers 200 with HTML, which gets a clear error rather than a decode one.
func toonkotsuGetJSON(uri, referer string, v any) error {
	body, err := http.Get(http.RequestParams{URL: uri, Referer: referer})
	if err != nil {
		return err
	}
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("<")) {
		return fmt.Errorf("toonkotsu: the API answered with a Cloudflare challenge (%s)", uri)
	}

	return json.Unmarshal(raw, v)
}

// toonkotsuSeriesCode returns the site's origin (www. dropped) and the series
// code from a series or chapter URL, the path segment before the first dash
// of /title/{code}-{slug}
func toonkotsuSeriesCode(raw string) (origin, code string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}

	m := toonkotsuPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", errSeriesNotFound(raw)
	}

	origin = u.Scheme + "://" + strings.TrimPrefix(strings.ToLower(u.Host), "www.")

	return origin, m[1], nil
}

// toonkotsuChapter builds the chapter for one release. n is the chapter's
// number, nil for an unnumbered release (a side story, a special).
func toonkotsuChapter(rel toonkotsuRelease, n *float64, vol int) *ToonkotsuChapter {
	c := &ToonkotsuChapter{Chapter{
		ID:       strconv.FormatInt(rel.ID, 10),
		Volume:   vol,
		Language: "en",
	}}
	if n != nil {
		c.Number = *n
		c.Label = Label{Style: LabelPrefixed}
		c.Title = toonkotsuTitle(rel.Label)
	} else {
		c.Label = Label{Style: LabelUnnumbered}
		c.Title = sanitizeTitle(rel.Label)
	}
	if rel.Group != nil {
		c.SetGroups(rel.Group.Name)
	}

	return c
}

// toonkotsuTitle returns a numbered release's title: its label without the
// "Chapter N" it starts with, and without a bare "Volume N" the label may end
// with, since the volume already says that
func toonkotsuTitle(label string) string {
	title := toonkotsuTitleRe.ReplaceAllString(label, "")
	if toonkotsuVolumeTitleRe.MatchString(strings.TrimSpace(title)) {
		title = ""
	}

	return sanitizeTitle(title)
}

// toonkotsuPageURL resolves a page's src, which is a path on the origin
func toonkotsuPageURL(origin, src string) string {
	if strings.HasPrefix(src, "/") {
		return origin + src
	}

	return src
}

// toonkotsuDecrypter undoes a page's AES-CTR: k is the key, n the counter
// block's first 8 bytes, both unpadded base64url. Each call builds a fresh
// stream, as a retry reruns the Transform.
func toonkotsuDecrypter(k, n string) (func([]byte) ([]byte, error), error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k, "="))
	if err != nil {
		return nil, fmt.Errorf("toonkotsu: bad page key: %w", err)
	}
	nonce, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(n, "="))
	if err != nil {
		return nil, fmt.Errorf("toonkotsu: bad page nonce: %w", err)
	}
	if len(nonce) < 8 {
		return nil, fmt.Errorf("toonkotsu: page nonce is %d bytes, want 8", len(nonce))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("toonkotsu: bad page key: %w", err)
	}

	return func(data []byte) ([]byte, error) {
		// the counter's low 8 bytes start at zero, the high 8 are the nonce
		iv := make([]byte, aes.BlockSize)
		copy(iv, nonce[:8])
		out := make([]byte, len(data))
		cipher.NewCTR(block, iv).XORKeyStream(out, data)

		return out, nil
	}, nil
}

// toonkotsuSeries is the series endpoint's answer, trimmed to what's read
type toonkotsuSeries struct {
	Series struct {
		Title string `json:"title"`
	} `json:"series"`
	Chapters []toonkotsuRow `json:"chapters"`
}

// toonkotsuRow is a chapter's top-level release, with its alternative releases
type toonkotsuRow struct {
	toonkotsuRelease
	Alts []toonkotsuRelease `json:"alts"`
}

// toonkotsuRelease is one release of a chapter. Only the top-level rows carry
// a number and volume; the alts of a row take them from it.
type toonkotsuRelease struct {
	ID    int64           `json:"id"`
	N     *float64        `json:"n"`
	Label string          `json:"label"`
	Vol   jsonVolume      `json:"vol"`
	Group *toonkotsuGroup `json:"group"`
}

type toonkotsuGroup struct {
	Name string `json:"name"`
}

// toonkotsuPages is the release endpoint's answer, trimmed to its pages
type toonkotsuPages struct {
	Chapter struct {
		ID    int64 `json:"id"`
		Pages []struct {
			Src string `json:"src"`
			K   string `json:"k"`
			N   string `json:"n"`
		} `json:"pages"`
	} `json:"chapter"`
}
