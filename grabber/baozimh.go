// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/fatih/color"
)

// Baozimh is a grabber for baozimh.com (包子漫画). The series page is plain
// server-rendered HTML (www.baozimh.com 301-redirects to cn.bzmgcn.com, but the
// http client follows that transparently) listing every chapter, and the reader
// page embeds every page as an <amp-img id="chapter-img-N-M" src=...> with a
// real CDN URL - no browser, no pagination, no javascript needed.
//
// It needs its own grabber instead of a PlainHTML selector because chapter
// numbers live in the Chinese title text ("第109话"), which chapterNumberRe
// doesn't recognise, and the reader URL carries the number in a chapter_slot=
// query param that urlChapterNumberRe doesn't match either.
type Baozimh struct {
	*Grabber
	title string
}

func NewBaozimh(g *Grabber) *Baozimh {
	return &Baozimh{Grabber: g}
}

// BaozimhChapter represents a Baozimh Chapter
type BaozimhChapter struct {
	Chapter
	URL string
}

// baozimhChapterNumberRe extracts the chapter number from the Chinese title
// text ("第109话", "第 1.5 话").
var baozimhChapterNumberRe = regexp.MustCompile(`第\s*(\d+(?:\.\d+)?)\s*话`)

// baozimhChapterNumber parses the chapter number out of a "第N话" title,
// returning false for entries without one (序章 prologue, 后记 afterword).
func baozimhChapterNumber(text string) (float64, bool) {
	match := baozimhChapterNumberRe.FindStringSubmatch(text)
	if len(match) == 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// Test returns true if the URL is a baozimh.com URL (also matching the
// cn.bzmgcn.com domain the site redirects to)
func (m *Baozimh) Test() (bool, error) {
	re := regexp.MustCompile(`(?i)(baozimh\.com|bzmgcn\.com)`)
	return re.MatchString(m.URL), nil
}

// FetchTitle fetches and returns the manga title
func (m *Baozimh) FetchTitle() (string, error) {
	if m.title != "" {
		return m.title, nil
	}

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

	// the <h1> is the site brand ("包子漫画"), not the series title; the real
	// one lives in the .comics-detail__title element
	m.title = sanitizeTitle(doc.Find(".comics-detail__title").Text())

	return m.title, nil
}

// FetchChapters returns the chapters of the manga
func (m Baozimh) FetchChapters() (chapters Filterables, errs []error) {
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

	// some rows are rendered twice (identical href, e.g. a re-uploaded
	// chapter), so dedupe by URL to avoid downloading each of those chapters
	// twice
	seen := map[string]bool{}
	doc.Find("a.comics-chapters__item").Each(func(i int, s *goquery.Selection) {
		title := sanitizeTitle(s.Find("span").Text())

		number, ok := baozimhChapterNumber(title)
		if !ok {
			// 序章 (prologue) and 后记 (afterword) entries carry no "第N话"
			// number; skip them like PlainHTML skips non-chapter rows
			return
		}

		u := m.BaseUrl() + s.AttrOr("href", "")
		if seen[u] {
			return
		}
		seen[u] = true

		chapters = append(chapters, &BaozimhChapter{
			Chapter{
				Number: number,
				Title:  title,
			},
			u,
		})
	})

	return
}

// FetchChapter fetches a chapter and its pages
func (m Baozimh) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*BaozimhChapter)
	body, err := http.Get(http.RequestParams{
		URL: mchap.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	pages := []Page{}
	// only the id="chapter-img-N-M" images are the chapter's pages; the reader
	// also renders a bottom "recommended comics" widget with <amp-img> covers
	// that carry no chapter-img id and must not be included
	doc.Find(`amp-img[id^="chapter-img-"]`).Each(func(i int, s *goquery.Selection) {
		img := s.AttrOr("src", "")
		if img == "" {
			color.Yellow("page %d of %s has no URL to fetch from 😕 (will be ignored)", i, f.GetTitle())
			return
		}
		pages = append(pages, Page{
			Number: int64(len(pages)),
			URL:    img,
		})
	})

	return &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Language:   "zh",
		Pages:      pages,
	}, nil
}
