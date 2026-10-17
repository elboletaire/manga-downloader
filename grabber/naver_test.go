// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"io"
	nethttp "net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

func TestNaverTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
		out  string
	}{
		{"https://comic.naver.com/webtoon/list?titleId=183559", true, "https://comic.naver.com/webtoon/list?titleId=183559"},
		{"https://comic.naver.com/webtoon/detail?titleId=183559&no=10&week=mon", true, "https://comic.naver.com/webtoon/list?titleId=183559"},
		{"https://m.comic.naver.com/bestChallenge/list?titleId=841794", true, "https://comic.naver.com/bestChallenge/list?titleId=841794"},
		{"https://comic.naver.com/challenge/detail?titleId=854428&no=1", true, "https://comic.naver.com/challenge/list?titleId=854428"},
		{"https://comic.naver.com/webtoon/list", false, ""},
		{"https://comic.naver.com/index", false, ""},
		{"https://example.com/webtoon/list?titleId=1", false, ""},
	}
	for _, c := range cases {
		n := NewNaver(&Grabber{URL: c.url})
		got, err := n.Test()
		if err != nil || got != c.want {
			t.Errorf("Test(%q) = %v, %v; want %v", c.url, got, err, c.want)
		}
		if got && n.URL != c.out {
			t.Errorf("Test(%q) URL = %q, want %q", c.url, n.URL, c.out)
		}
	}
}

type naverRT map[string]string

func (r naverRT) RoundTrip(req *nethttp.Request) (*nethttp.Response, error) {
	body, ok := r[req.URL.RequestURI()]
	status := 200
	if !ok {
		status, body = 404, ""
	}

	return &nethttp.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: nethttp.Header{}, Request: req}, nil
}

func TestNaverFetchChapters(t *testing.T) {
	http.SetRoundTripper(naverRT{
		"/api/article/list?titleId=7&page=1": `{"pageInfo":{"totalPages":2},"articleList":[{"no":2,"subtitle":"Two","charge":false},{"no":1,"subtitle":"One","charge":true}]}`,
		"/api/article/list?titleId=7&page=2": `{"pageInfo":{"totalPages":2},"articleList":[{"no":3,"subtitle":"Three","charge":false},{"no":2,"subtitle":"Two","charge":false}]}`,
	})
	defer http.SetRoundTripper(nil)

	n := NewNaver(&Grabber{URL: "https://comic.naver.com/webtoon/list?titleId=7"})
	n.titleID = "7"
	chapters, errs := n.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3 (deduped)", len(chapters))
	}
	byNo := map[float64]bool{}
	for _, c := range chapters {
		byNo[c.GetNumber()] = c.(*NaverChapter).Locked
	}
	if !byNo[1] || byNo[2] || byNo[3] {
		t.Errorf("locked flags = %v", byNo)
	}
}

func TestNaverLogin(t *testing.T) {
	http.SetRoundTripper(naverRT{"/api/article/list?titleId=9&page=1": `"LOGIN"`})
	defer http.SetRoundTripper(nil)

	n := NewNaver(&Grabber{URL: "https://comic.naver.com/webtoon/list?titleId=9"})
	n.titleID = "9"
	_, errs := n.FetchChapters()
	if len(errs) != 1 || !errors.Is(errs[0], errNaverLogin) {
		t.Fatalf("errs = %v, want errNaverLogin", errs)
	}
}

func TestNaverPages(t *testing.T) {
	html := `<div id="comic_view_area"><div class="wt_viewer">
<img src="https://x/banner.png"><img id="content_image_0" src="https://image-comic.pstatic.net/a.jpg">
<img id="content_image_1" src="https://image-comic.pstatic.net/b.jpg"></div></div>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	pages := naverPages(doc)
	if len(pages) != 2 || pages[1].URL != "https://image-comic.pstatic.net/b.jpg" || pages[1].Number != 2 {
		t.Errorf("pages = %+v", pages)
	}
}
