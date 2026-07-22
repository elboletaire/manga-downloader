// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Fanfox is a grabber for fanfox.net (Manga Fox): the chapter list is plain
// HTML, but the reader only ships a loading spinner - the actual page image
// URL for each page is fetched, one page at a time, from the site's
// chapterfun.ashx endpoint. That response is JS packed with the "Dean
// Edwards" P.A.C.K.E.R. obfuscator (see unpackPackerJS), not encrypted: it
// unpacks deterministically with no browser/JS engine needed.
type Fanfox struct {
	*Grabber
	title string
}

func NewFanfox(g *Grabber) *Fanfox {
	return &Fanfox{Grabber: g}
}

// FanfoxChapter represents a Fanfox Chapter
type FanfoxChapter struct {
	Chapter
	URL string
}

// fanfoxDomainRe matches fanfox.net (with or without www.)
var fanfoxDomainRe = regexp.MustCompile(`fanfox\.net`)

// Test returns true if the URL is a fanfox.net URL
func (m *Fanfox) Test() (bool, error) {
	if !fanfoxDomainRe.MatchString(m.URL) {
		return false, nil
	}

	// fanfox gates the chapter list behind an age-confirmation interstitial
	// (even for non-mature titles, e.g. Chainsaw Man) unless this cookie is
	// present - no login/JS challenge involved, just a cookie plain HTTP can
	// set itself
	http.SetCookie("fanfox.net", "isAdult", "1")

	return true, nil
}

// FetchTitle fetches and returns the manga title
func (m *Fanfox) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

	doc, err := m.fetchDoc(m.URL)
	if err != nil {
		return "", err
	}

	m.title = sanitizeTitle(doc.Find(".detail-info-right-title-font").Text())

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m Fanfox) FetchChapters() (Filterables, []error) {
	doc, err := m.fetchDoc(m.URL)
	if err != nil {
		return nil, []error{err}
	}

	var chapters Filterables
	doc.Find("#chapterlist ul.detail-main-list li a").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Find(".title3").Text())
		number, ok := parseChapterNumber(text)
		if !ok {
			return
		}

		u := s.AttrOr("href", "")
		if !strings.HasPrefix(u, "http") {
			u = m.BaseUrl() + u
		}

		chapters = append(chapters, &FanfoxChapter{
			Chapter{
				Number: number,
				Title:  text,
			},
			u,
		})
	})

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (m Fanfox) FetchChapter(f Filterable) (*Chapter, error) {
	fchap := f.(*FanfoxChapter)

	doc, err := m.fetchDoc(fchap.URL)
	if err != nil {
		return nil, err
	}
	html, err := doc.Html()
	if err != nil {
		return nil, err
	}

	chapterId, err := jsIntVar(html, "chapterid")
	if err != nil {
		return nil, err
	}
	pageCount, err := jsIntVar(html, "imagecount")
	if err != nil {
		return nil, err
	}
	// mkey is normally empty, but the reader page always carries a (possibly
	// blank) hidden input for it - chapterfun.ashx wants it echoed back
	mkey := doc.Find("#dm5_key").AttrOr("value", "")

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(pageCount),
		Language:   "en",
	}

	for page := 1; page <= pageCount; page++ {
		uri := fmt.Sprintf("%s/chapterfun.ashx?cid=%d&page=%d&key=%s", m.BaseUrl(), chapterId, page, mkey)
		body, err := http.GetText(http.RequestParams{
			URL:     uri,
			Referer: fchap.URL,
		})
		if err != nil {
			return nil, fmt.Errorf("fetching page %d: %w", page, err)
		}

		imgs, err := unpackFanfoxImages(body)
		if err != nil {
			return nil, fmt.Errorf("decoding page %d: %w", page, err)
		}
		if len(imgs) == 0 {
			return nil, fmt.Errorf("no image url found for page %d", page)
		}

		imgURL := imgs[0]
		if strings.HasPrefix(imgURL, "//") {
			imgURL = "https:" + imgURL
		}

		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(page),
			URL:    imgURL,
		})
	}

	return chapter, nil
}

// fetchDoc fetches a URL and parses it as an HTML document
func (m Fanfox) fetchDoc(uri string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{
		URL:     uri,
		Referer: m.BaseUrl(),
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// jsIntVar extracts a "var name = 123;" (or "var name=123;") integer from
// the passed html/js source
func jsIntVar(src, name string) (int, error) {
	re := regexp.MustCompile(`var\s+` + regexp.QuoteMeta(name) + `\s*=\s*(\d+)`)
	matches := re.FindStringSubmatch(src)
	if len(matches) != 2 {
		return 0, fmt.Errorf("could not find the %q variable", name)
	}

	return strconv.Atoi(matches[1])
}

// packerCallRe matches a "Dean Edwards" P.A.C.K.E.R. packed payload, as
// returned by fanfox's chapterfun.ashx endpoint:
// eval(function(p,a,c,k,e,d){...}('<payload>',<a>,<c>,'<dictionary>'.split('|'),0,{}))
var packerCallRe = regexp.MustCompile(`(?s)eval\(function\(p,a,c,k,e,d\).*?\}\('(.*)',(\d+),(\d+),'(.*)'\.split\('\|'\)`)

// unpackPackerJS reverses the P.A.C.K.E.R. obfuscation: it's a plain
// tokenizer substitution (not encryption), documented at
// http://dean.edwards.name/packer/ - every token in the packed payload is a
// base-`a` encoded index into the dictionary, and gets replaced back with
// its dictionary word.
func unpackPackerJS(payload string) (string, error) {
	m := packerCallRe.FindStringSubmatch(payload)
	if len(m) != 5 {
		return "", fmt.Errorf("payload does not look like a packed script")
	}

	p := m[1]
	a, err := strconv.Atoi(m[2])
	if err != nil || a < 2 {
		return "", fmt.Errorf("invalid packer radix %q", m[2])
	}
	c, err := strconv.Atoi(m[3])
	if err != nil {
		return "", fmt.Errorf("invalid packer count %q", m[3])
	}
	k := strings.Split(m[4], "|")

	for i := c - 1; i >= 0; i-- {
		if i >= len(k) || k[i] == "" {
			continue
		}
		token := packerToken(i, a)
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(token) + `\b`)
		p = re.ReplaceAllString(p, k[i])
	}

	return p, nil
}

// packerToken re-implements the packer's own base-`a` digit encoder:
// e=function(c){return(c<a?"":e(parseInt(c/a)))+((c=c%a)>35?String.fromCharCode(c+29):c.toString(36))}
func packerToken(value, base int) string {
	prefix := ""
	if value >= base {
		prefix = packerToken(value/base, base)
	}
	remainder := value % base
	if remainder > 35 {
		return prefix + string(rune(remainder+29))
	}

	return prefix + strconv.FormatInt(int64(remainder), 36)
}

// fanfoxImageFuncRe extracts the pieces the unpacked chapterfun.ashx script
// builds its image URLs from: a base "pix" CDN path (duplicated verbatim as
// a literal for the first array item) and a list of per-page query-string
// suffixes.
var (
	fanfoxPixRe      = regexp.MustCompile(`var\s+pix\s*=\s*"([^"]*)"`)
	fanfoxFirstPixRe = regexp.MustCompile(`if\s*\(i==0\)\s*\{\s*pvalue\[i\]\s*=\s*"([^"]*)"`)
	fanfoxPvalueRe   = regexp.MustCompile(`var\s+pvalue\s*=\s*(\[[^\]]*\])`)
	fanfoxStringRe   = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
)

// unpackFanfoxImages decodes a chapterfun.ashx response into its ordered
// image URLs (usually 1 or 2: the current page, and the next page's
// preload)
func unpackFanfoxImages(payload string) ([]string, error) {
	src, err := unpackPackerJS(payload)
	if err != nil {
		return nil, err
	}

	pixMatch := fanfoxPixRe.FindStringSubmatch(src)
	if len(pixMatch) != 2 {
		return nil, fmt.Errorf("could not find the pix base url")
	}
	pix := pixMatch[1]

	firstPix := pix
	if fp := fanfoxFirstPixRe.FindStringSubmatch(src); len(fp) == 2 {
		firstPix = fp[1]
	}

	pvalueMatch := fanfoxPvalueRe.FindStringSubmatch(src)
	if len(pvalueMatch) != 2 {
		return nil, fmt.Errorf("could not find the pvalue array")
	}

	suffixes := fanfoxStringRe.FindAllStringSubmatch(pvalueMatch[1], -1)
	urls := make([]string, 0, len(suffixes))
	for i, s := range suffixes {
		base := pix
		if i == 0 {
			base = firstPix
		}
		urls = append(urls, base+s[1])
	}

	return urls, nil
}
