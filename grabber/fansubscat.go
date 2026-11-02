// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// FansubsCat is a grabber for manga.fansubs.cat, the Catalan manga catalogue of
// Fansubs.cat. The series page lists every release of a series, one version tab
// per fansub; a release's pages come from a POST to get_file_data.php, not the page.
type FansubsCat struct {
	*Grabber
}

// NewFansubsCat returns a new FansubsCat grabber
func NewFansubsCat(g *Grabber) *FansubsCat {
	return &FansubsCat{Grabber: g}
}

// FansubsCatChapter is a release listed on a series page
type FansubsCatChapter struct {
	Chapter
}

// fansubsCatNumberRe reads the number off a row's "Capítol N" label, which the
// site writes with a comma for decimals ("Capítol 0,5")
var fansubsCatNumberRe = regexp.MustCompile(`(?i)^cap[ií]tol\s+(\d+(?:[.,]\d+)?)$`)

// fansubsCatHashRe is the "#001 - " some series put before a row's title
var fansubsCatHashRe = regexp.MustCompile(`^#[\d.,]+\s*-\s*`)

// fansubsCatVolumeRe reads the number off a division's "Volum N" title
var fansubsCatVolumeRe = regexp.MustCompile(`^Volum\s+(\d+)$`)

// fansubsCatPage fetches a page of the site and parses it
func fansubsCatPage(rawURL string) (*goquery.Document, error) {
	body, err := http.Get(http.RequestParams{URL: rawURL})
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return goquery.NewDocumentFromReader(body)
}

// Test returns true if the page is a manga series of the Fansubs.cat catalogue,
// by its markup. The page is shared with the other markup testers.
func (f *FansubsCat) Test() (bool, error) {
	body, err := f.identifyPage(f.URL)
	if err != nil {
		return false, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return false, nil
	}
	isCatalogue := doc.Find(`meta[name="current_domain"]`).AttrOr("content", "") == "fansubs.cat"
	return isCatalogue && doc.Find("input#catalogue_type").AttrOr("value", "") == "manga", nil
}

// resolveSeriesURL drops the query of a reader link (?f={file} opens a release
// on the series page), so the URL is the series page's own
func (f *FansubsCat) resolveSeriesURL() error {
	u, err := url.Parse(f.URL)
	if err != nil {
		return err
	}
	u.RawQuery = ""
	u.Fragment = ""
	f.URL = u.String()
	return nil
}

// FetchTitle returns the series title
func (f *FansubsCat) FetchTitle() (string, error) {
	if err := f.resolveSeriesURL(); err != nil {
		return "", err
	}

	doc, err := fansubsCatPage(f.URL)
	if err != nil {
		return "", err
	}

	return sanitizeTitle(doc.Find("h2.series-title").Text()), nil
}

// FetchChapters returns one chapter per release, grouped by the fansub of its
// version tab. Each division's "Volum N" gives the volume
func (f *FansubsCat) FetchChapters() (chapters Filterables, errs []error) {
	if err := f.resolveSeriesURL(); err != nil {
		return nil, []error{err}
	}

	doc, err := fansubsCatPage(f.URL)
	if err != nil {
		return nil, []error{err}
	}

	doc.Find(".version-tab[data-version-id]").Each(func(_ int, tab *goquery.Selection) {
		var groups []string
		tab.Find(".version-fansub-icons .fansub img").Each(func(_ int, img *goquery.Selection) {
			groups = append(groups, sanitizeTitle(img.AttrOr("title", "")))
		})

		content := doc.Find("#version-content-" + tab.AttrOr("data-version-id", ""))
		content.Find(".file-launcher.episode").Each(func(_ int, row *goquery.Selection) {
			chapter := &FansubsCatChapter{Chapter{
				ID:       row.AttrOr("data-file-id", ""),
				Language: "ca",
				Label:    Label{Style: LabelNumbered},
			}}
			chapter.SetGroups(groups...)
			if match := fansubsCatVolumeRe.FindStringSubmatch(sanitizeTitle(row.Closest(".division").Find(".division-title").First().Text())); match != nil {
				chapter.Volume, _ = strconv.Atoi(match[1])
			}

			// a row's first <b> is its label ("Capítol N", or a one-shot's name) and
			// the text after it the title, which may start with "#N - "
			titleCell := row.Find(".episode-title")
			label := sanitizeTitle(titleCell.Find("b").First().Text())
			titleCell.Find("b").First().Remove()
			// a variant's <b> follows its title with no space between them
			titleCell.Find("b").Each(func(_ int, b *goquery.Selection) {
				b.BeforeHtml(" ")
			})
			if match := fansubsCatNumberRe.FindStringSubmatch(label); match != nil {
				chapter.Number, _ = strconv.ParseFloat(strings.Replace(match[1], ",", ".", 1), 64)
				chapter.Title = sanitizeTitle(fansubsCatHashRe.ReplaceAllString(sanitizeTitle(titleCell.Text()), ""))
			} else {
				chapter.Title = label
				chapter.Label.Style = LabelUnnumbered
			}

			chapters = append(chapters, chapter)
		})
	})

	return chapters, nil
}

// fansubsCatFile is the reader's answer to get_file_data.php
type fansubsCatFile struct {
	Result string `json:"result"`
	Data   struct {
		Pages []string `json:"pages"`
	} `json:"data"`
}

// FetchChapter asks the site for the pages of the release
func (f *FansubsCat) FetchChapter(fl Filterable) (*Chapter, error) {
	listed := fl.(*FansubsCatChapter)

	body, err := http.Post(http.RequestParams{
		URL:     f.BaseUrl() + "/get_file_data.php",
		Referer: f.URL,
		Form:    url.Values{"file_id": {listed.ID}},
		Headers: map[string]string{"X-Requested-With": "XMLHttpRequest"},
		Accept:  "application/json",
	})
	if err != nil {
		return nil, fmt.Errorf("fansubs.cat release %s: %w", listed.ID, err)
	}
	defer body.Close()

	var file fansubsCatFile
	if err := json.NewDecoder(body).Decode(&file); err != nil {
		return nil, fmt.Errorf("fansubs.cat release %s: %w", listed.ID, err)
	}
	if file.Result != "ok" || len(file.Data.Pages) == 0 {
		return nil, fmt.Errorf("fansubs.cat release %s: no pages", listed.ID)
	}

	chapter := &Chapter{
		Title:    listed.GetTitle(),
		Label:    labelOf(listed),
		Number:   listed.GetNumber(),
		Volume:   listed.Volume,
		Language: listed.Language,
		ID:       listed.ID,
	}
	chapter.SetGroups(listed.GetGroups()...)

	for i, page := range file.Data.Pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i + 1),
			URL:    page,
		})
	}
	chapter.PagesCount = int64(len(chapter.Pages))

	return chapter, nil
}
