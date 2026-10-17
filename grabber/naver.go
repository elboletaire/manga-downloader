// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Naver is a grabber for Naver Webtoon (comic.naver.com), including its
// challenge and bestChallenge titles. The episode list and title come from
// JSON endpoints, the pages from the server-rendered viewer.
//
// Paid episodes (charge) redirect to the list when logged out, so they are
// listed as Locked. Adult series answer the list API with "LOGIN" and can't
// be read without an account.
type Naver struct {
	*Grabber
	title    string
	titleID  string
	episodes []*NaverChapter
}

func NewNaver(g *Grabber) *Naver {
	return &Naver{Grabber: g}
}

// NaverChapter represents a Naver Webtoon episode
type NaverChapter struct {
	Chapter
	// URL is the absolute URL of the episode's viewer
	URL string
}

var (
	naverHostRe = regexp.MustCompile(`^(www\.|m\.)?comic\.naver\.com$`)
	naverPathRe = regexp.MustCompile(`^/(webtoon|challenge|bestChallenge)/(list|detail)/?$`)

	errNaverLogin = errors.New("this series requires a Naver login")
)

// Test matches a comic.naver.com list or episode URL with a titleId. Both
// resolve to the whole series' list.
func (n *Naver) Test() (bool, error) {
	uri, err := url.Parse(n.URL)
	if err != nil {
		return false, err
	}
	if !naverHostRe.MatchString(uri.Hostname()) {
		return false, nil
	}
	m := naverPathRe.FindStringSubmatch(uri.Path)
	id := uri.Query().Get("titleId")
	if m == nil || id == "" {
		return false, nil
	}

	n.titleID = id
	n.URL = fmt.Sprintf("https://comic.naver.com/%s/list?titleId=%s", m[1], url.QueryEscape(id))

	return true, nil
}

// naverEpisode is one row of the list API
type naverEpisode struct {
	No       int    `json:"no"`
	Subtitle string `json:"subtitle"`
	Charge   bool   `json:"charge"`
}

// naverList is the list API's payload
type naverList struct {
	PageInfo struct {
		TotalPages int `json:"totalPages"`
	} `json:"pageInfo"`
	ArticleList []naverEpisode `json:"articleList"`
}

// naverGet fetches an API path as JSON, turning the "LOGIN" string adult
// series answer with (or a 401) into errNaverLogin
func (n *Naver) naverGet(path string, v any) error {
	body, err := http.GetText(http.RequestParams{
		URL:     n.BaseUrl() + path,
		Referer: n.BaseUrl() + "/",
	})
	var se *http.StatusError
	if errors.As(err, &se) && se.Code == 401 {
		return errNaverLogin
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == `"LOGIN"` {
		return errNaverLogin
	}

	return json.Unmarshal([]byte(body), v)
}

// FetchTitle fetches and returns the series title
func (n *Naver) FetchTitle() (string, error) {
	if n.title != "" {
		return n.title, nil
	}

	var info struct {
		TitleName string `json:"titleName"`
	}
	if err := n.naverGet("/api/article/list/info?titleId="+url.QueryEscape(n.titleID), &info); err != nil {
		return "", err
	}
	n.title = sanitizeTitle(info.TitleName)
	if n.title == "" {
		return "", fmt.Errorf("no title found for %s", n.URL)
	}

	return n.title, nil
}

// FetchChapters returns every episode of the series
func (n *Naver) FetchChapters() (chapters Filterables, errs []error) {
	if n.episodes == nil {
		eps, err := n.fetchEpisodes()
		if err != nil {
			return nil, []error{err}
		}
		n.episodes = eps
	}
	for _, c := range n.episodes {
		chapters = append(chapters, c)
	}

	return
}

// fetchEpisodes walks the list pages; those past the last repeat it, so it
// stops at totalPages and dedupes by episode number
func (n *Naver) fetchEpisodes() ([]*NaverChapter, error) {
	seen := map[int]bool{}
	var out []*NaverChapter
	totalPages := 1

	for page := 1; page <= totalPages; page++ {
		var list naverList
		path := fmt.Sprintf("/api/article/list?titleId=%s&page=%d", url.QueryEscape(n.titleID), page)
		if err := n.naverGet(path, &list); err != nil {
			return nil, err
		}
		totalPages = list.PageInfo.TotalPages

		for _, ep := range list.ArticleList {
			if seen[ep.No] {
				continue
			}
			seen[ep.No] = true
			out = append(out, &NaverChapter{
				Chapter: Chapter{
					Title:    ep.Subtitle,
					Number:   float64(ep.No),
					ID:       strconv.Itoa(ep.No),
					Locked:   ep.Charge,
					Language: "ko",
					Label:    Label{Style: LabelNumbered},
				},
				URL: fmt.Sprintf("%s/%s/detail?titleId=%s&no=%d", n.BaseUrl(), n.kind(), url.QueryEscape(n.titleID), ep.No),
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no episodes found for %s", n.URL)
	}

	return out, nil
}

// kind returns the path section (webtoon, challenge, bestChallenge) of the URL
func (n *Naver) kind() string {
	if m := naverPathRe.FindStringSubmatch(strings.SplitN(strings.TrimPrefix(n.URL, n.BaseUrl()), "?", 2)[0]); m != nil {
		return m[1]
	}
	return "webtoon"
}

// FetchChapter fetches an episode and its pages
func (n *Naver) FetchChapter(f Filterable) (*Chapter, error) {
	nc := f.(*NaverChapter)

	body, err := http.GetText(http.RequestParams{URL: nc.URL, Referer: n.BaseUrl() + "/"})
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	pages := naverPages(doc)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for episode %v (locked or unavailable?): %s", f.GetNumber(), nc.URL)
	}

	return &Chapter{
		Title:      f.GetTitle(),
		Number:     f.GetNumber(),
		PagesCount: int64(len(pages)),
		Pages:      pages,
		ID:         nc.ID,
		Language:   nc.Language,
		Label:      nc.Label,
	}, nil
}

// naverPages reads the viewer's images; the age banner has no content_image id
func naverPages(doc *goquery.Document) (pages []Page) {
	doc.Find("#comic_view_area .wt_viewer img[id^=content_image_]").Each(func(_ int, s *goquery.Selection) {
		src := s.AttrOr("src", s.AttrOr("data-src", ""))
		if src == "" {
			return
		}
		pages = append(pages, Page{Number: int64(len(pages) + 1), URL: src})
	})

	return
}
