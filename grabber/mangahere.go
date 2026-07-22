// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Mangahere is a grabber for mangahere.cc. Series and reader pages are plain
// HTML, but the reader only ever renders one page at a time: the actual page
// image is fetched by client-side JS from an AJAX endpoint
// (chapterfun.ashx?cid=<chapterid>&page=<n>) whose response body is a Dean
// Edwards "eval(function(p,a,c,k,e,d){...})" packed blob that has to be
// unpacked to recover the "pix"/"pvalue" JS variables building the real CDN
// image URL. That per-page AJAX round trip is why this needs its own grabber
// instead of PlainHTML (whose Image selector expects every page URL to
// already be present in a single response).
type Mangahere struct {
	*Grabber
}

func NewMangahere(g *Grabber) *Mangahere {
	return &Mangahere{Grabber: g}
}

// MangahereChapter represents a Mangahere chapter
type MangahereChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a mangahere.cc URL
func (m *Mangahere) Test() (bool, error) {
	re := regexp.MustCompile(`mangahere\.cc`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Mangahere) FetchTitle() (string, error) {
	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return "", err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return "", err
	}

	return sanitizeTitle(doc.Find(".detail-info-right-title-font").Text()), nil
}

// FetchChapters returns the chapters of the manga
func (m Mangahere) FetchChapters() (chapters Filterables, errs []error) {
	body, err := http.Get(http.RequestParams{
		URL: m.URL,
	})
	if err != nil {
		return nil, []error{err}
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, []error{err}
	}

	doc.Find(".detail-main-list li a").Each(func(i int, s *goquery.Selection) {
		title := sanitizeTitle(s.Find(".title3").Text())
		number, ok := parseChapterNumber(title)
		if !ok {
			return
		}

		href := s.AttrOr("href", "")
		if !strings.HasPrefix(href, "http") {
			href = m.BaseUrl() + href
		}

		chapters = append(chapters, &MangahereChapter{
			Chapter{
				Number: number,
				Title:  title,
			},
			href,
		})
	})

	return
}

var (
	mangahereChapterIDRe  = regexp.MustCompile(`var\s+chapterid\s*=\s*(\d+)`)
	mangahereImageCountRe = regexp.MustCompile(`var\s+imagecount\s*=\s*(\d+)`)
	mangahereKeyRe        = regexp.MustCompile(`id="dm5_key"\s+value="([^"]*)"`)
)

// FetchChapter fetches a chapter and its pages
func (m Mangahere) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangahereChapter)

	body, err := http.GetText(http.RequestParams{
		URL:     mchap.URL,
		Referer: m.URL,
	})
	if err != nil {
		return nil, err
	}

	cidMatch := mangahereChapterIDRe.FindStringSubmatch(body)
	if len(cidMatch) != 2 {
		return nil, fmt.Errorf("mangahere: could not find chapterid in %s", mchap.URL)
	}
	cid := cidMatch[1]

	countMatch := mangahereImageCountRe.FindStringSubmatch(body)
	if len(countMatch) != 2 {
		return nil, fmt.Errorf("mangahere: could not find imagecount in %s", mchap.URL)
	}
	count, err := strconv.Atoi(countMatch[1])
	if err != nil {
		return nil, err
	}

	key := ""
	if keyMatch := mangahereKeyRe.FindStringSubmatch(body); len(keyMatch) == 2 {
		key = keyMatch[1]
	}

	// chapterfun.ashx sits next to the reader page itself, i.e.
	// /manga/<slug>/c363/1.html -> /manga/<slug>/c363/chapterfun.ashx
	ashxBase := mchap.URL[:strings.LastIndex(mchap.URL, "/")+1] + "chapterfun.ashx"

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(count),
		Language:   "en",
	}

	for page := 1; page <= count; page++ {
		uri := fmt.Sprintf("%s?cid=%s&page=%d&key=%s", ashxBase, cid, page, key)
		res, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: mchap.URL,
		})
		if err != nil {
			return nil, err
		}

		img, err := unpackMangahereImageURL(res)
		if err != nil {
			return nil, err
		}

		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(page - 1),
			URL:    img,
		})
	}

	return chapter, nil
}

// mangaherePackedRe extracts the arguments of the Dean Edwards packer call
// wrapping chapterfun.ashx's response:
// eval(function(p,a,c,k,e,d){...}('<payload>',<a>,<c>,'<keywords>'.split('|'),0,{}))
var mangaherePackedRe = regexp.MustCompile(`(?s)\}\('(.*)',(\d+),(\d+),'([^']*)'\.split\('\|'\)`)

var (
	mangaherePixRe    = regexp.MustCompile(`var pix\s*=\s*"([^"]*)"`)
	mangaherePvalueRe = regexp.MustCompile(`var pvalue\s*=\s*\["([^"]*)"`)
)

// unpackMangahereImageURL decodes a chapterfun.ashx response (a Dean Edwards
// packed JS blob) and returns the current page's image URL (the "pix" base
// path plus the first "pvalue" entry; any further entries are just the next
// page's preloaded image and are ignored).
func unpackMangahereImageURL(res string) (string, error) {
	m := mangaherePackedRe.FindStringSubmatch(res)
	if len(m) != 5 {
		return "", errors.New("mangahere: could not find packed image data in chapterfun.ashx response")
	}

	a, err := strconv.Atoi(m[2])
	if err != nil {
		return "", err
	}
	c, err := strconv.Atoi(m[3])
	if err != nil {
		return "", err
	}
	keywords := strings.Split(m[4], "|")

	decoded := unpackDeanEdwards(m[1], a, c, keywords)

	pix := mangaherePixRe.FindStringSubmatch(decoded)
	pvalue := mangaherePvalueRe.FindStringSubmatch(decoded)
	if len(pix) != 2 || len(pvalue) != 2 {
		return "", fmt.Errorf("mangahere: could not extract image url from decoded chapterfun.ashx response: %s", decoded)
	}

	url := pix[1] + pvalue[1]
	if strings.HasPrefix(url, "//") {
		url = "https:" + url
	}

	return url, nil
}

// dean edwards packer word tokens are matched as whole words
var packerWordRe = regexp.MustCompile(`\b\w+\b`)

// unpackDeanEdwards reverses the "eval(function(p,a,c,k,e,d){...})" packer
// (a common minifier/obfuscator, used here by mangahere.cc to hide reader
// image URLs). p is the packed payload, a is the radix used to encode word
// tokens, c is the token count and keywords[i] is the real word to substitute
// for token i (or empty to leave the token as-is).
func unpackDeanEdwards(p string, a, c int, keywords []string) string {
	dict := make(map[string]string, c)
	for i := c - 1; i >= 0; i-- {
		token := packerToken(i, a)
		if i < len(keywords) && keywords[i] != "" {
			dict[token] = keywords[i]
		} else {
			dict[token] = token
		}
	}

	return packerWordRe.ReplaceAllStringFunc(p, func(w string) string {
		if v, ok := dict[w]; ok {
			return v
		}
		return w
	})
}

// packerToken reproduces the packer's base-`a` token encoding (digits
// 0-9a-z, falling back to arbitrary chars above base 36 - see the packer's
// own `e` function).
func packerToken(c, a int) string {
	var s string
	if c >= a {
		s = packerToken(c/a, a)
	}
	c %= a
	if c > 35 {
		s += string(rune(c + 29))
	} else {
		s += string("0123456789abcdefghijklmnopqrstuvwxyz"[c])
	}
	return s
}
