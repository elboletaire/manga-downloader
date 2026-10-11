// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestWebtoonsTest(t *testing.T) {
	cases := []struct {
		url     string
		want    bool
		wantURL string // rewritten URL after Test(); empty means unchanged
	}{
		{
			"https://www.webtoons.com/en/fantasy/tower-of-god/list?title_no=95",
			true,
			"https://www.webtoons.com/en/fantasy/tower-of-god/list?title_no=95",
		},
		// the episode viewer maps back to the series' list
		{
			"https://www.webtoons.com/en/fantasy/tower-of-god/season-1-ep-0/viewer?title_no=95&episode_no=1",
			true,
			"https://www.webtoons.com/en/fantasy/tower-of-god/list?title_no=95",
		},
		// the mobile host and another language resolve to the same list
		{
			"https://m.webtoons.com/es/fantasy/tower-of-god/list?title_no=95",
			true,
			"https://www.webtoons.com/es/fantasy/tower-of-god/list?title_no=95",
		},
		{
			"https://www.webtoons.com/en/canvas/dark-tales/list?title_no=440824",
			true,
			"https://www.webtoons.com/en/canvas/dark-tales/list?title_no=440824",
		},
		// no title_no, nothing to list
		{"https://www.webtoons.com/en/fantasy/tower-of-god/", false, "https://www.webtoons.com/en/fantasy/tower-of-god/"},
		{"https://example.com/en/fantasy/tower-of-god/list?title_no=95", false, "https://example.com/en/fantasy/tower-of-god/list?title_no=95"},
	}

	for _, c := range cases {
		w := NewWebtoons(&Grabber{URL: c.url})
		got, err := w.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
			continue
		}
		if w.URL != c.wantURL {
			t.Errorf("Test(%q) rewrote URL to %q, want %q", c.url, w.URL, c.wantURL)
		}
	}
}

// webtoonsListFixture is a trimmed episode list page: the same markup as the
// series list, with the pager and the sidebar left out
const webtoonsListFixture = `<html><head><meta property="og:title" content="Tower of God" /></head><body>
<h1 class="subj">Tower of God</h1>
<ul id="_listUl" class="detail_list">
	<li class="_episodeItem detail_list_item" id="episode_653" data-episode-no="653">
		<a href="https://www.webtoons.com/en/fantasy/tower-of-god/season-3-ep-235-season-3-finale/viewer?title_no=95&amp;episode_no=653" class="detail_list_link">
			<span class="thmb"><img src="https://webtoon-phinf.pstatic.net/thumb.png" alt="[Season 3] Ep. 235"></span>
			<span class="subj"><span>[Season 3] Ep. 235 (Season 3 Finale)</span></span>
			<span class="date">Feb 23, 2025</span>
			<span class="tx">#652</span>
		</a>
	</li>
	<li class="_episodeItem detail_list_item" id="episode_652" data-episode-no="652">
		<a href="https://www.webtoons.com/en/fantasy/tower-of-god/season-3-ep-234/viewer?title_no=95&amp;episode_no=652" class="detail_list_link">
			<span class="subj"><span>	[Season 3] Ep. 234
			</span></span>
		</a>
	</li>
	<li class="_episodeItem detail_list_item" id="episode_nolink" data-episode-no="651">
	</li>
</ul>
</body></html>`

// webtoonsCanvasFixture has the canvas layout: the title sits in a span
// followed by an "UP" marker, and the creator's EDIT link repeats the number
const webtoonsCanvasFixture = `<ul id="_listUl" class="detail_list">
	<li class="_episodeItem detail_list_item" id="episode_80" data-episode-no="80">
		<a href="https://www.webtoons.com/en/canvas/dark-tales/trapped-in-the-house-of-flies-pt10/viewer?title_no=440824&amp;episode_no=80&amp;readingLanguageCode=en" class="detail_list_link">
			<span class="subj"><span>TRAPPED IN THE HOUSE OF FLIES PT.10</span><em class="tx_up">UP</em></span>
		</a>
		<span class="manage"><a href="#" data-episode-no="80" class="btn_type6 _btnEpisodeEdit">EDIT</a></span>
	</li>
</ul>`

func TestWebtoonsEpisodes(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(webtoonsListFixture))
	if err != nil {
		t.Fatal(err)
	}

	got := webtoonsEpisodes(doc)
	if len(got) != 2 {
		t.Fatalf("webtoonsEpisodes() returned %d episodes, want 2: %+v", len(got), got)
	}

	want := []webtoonsEpisode{
		{
			title:  "[Season 3] Ep. 235 (Season 3 Finale)",
			number: 653,
			url:    "https://www.webtoons.com/en/fantasy/tower-of-god/season-3-ep-235-season-3-finale/viewer?title_no=95&episode_no=653",
		},
		{
			title:  "[Season 3] Ep. 234",
			number: 652,
			url:    "https://www.webtoons.com/en/fantasy/tower-of-god/season-3-ep-234/viewer?title_no=95&episode_no=652",
		},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("episode %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	doc, err = goquery.NewDocumentFromReader(strings.NewReader(webtoonsCanvasFixture))
	if err != nil {
		t.Fatal(err)
	}
	got = webtoonsEpisodes(doc)
	if len(got) != 1 {
		t.Fatalf("canvas webtoonsEpisodes() returned %d episodes, want 1: %+v", len(got), got)
	}
	if got[0].title != "TRAPPED IN THE HOUSE OF FLIES PT.10" || got[0].number != 80 {
		t.Errorf("canvas episode = %+v", got[0])
	}
}

// webtoonsViewerFixture is a trimmed episode viewer: two reader images, and a
// sidebar of other episodes' thumbnails that must not be taken for pages
const webtoonsViewerFixture = `<html><body>
<div id="_thumbnailList">
	<img src="https://webtoons-static.pstatic.net/image/bg_transparency.png" data-url="https://webtoon-phinf.pstatic.net/tower_000.jpg" class="_thumbnailImages" width="92" height="87" alt="[Season 1] Ep. 0">
</div>
<div class="viewer_img _img_viewer_area " id="_imageList">
	<img src="https://webtoons-static.pstatic.net/image/bg_transparency.png" width="700" height="1140.0" alt="image" class="_images" data-url="https://webtoon-phinf.pstatic.net/a.jpg?type=q90" rel="nofollow">
	<img src="https://webtoons-static.pstatic.net/image/bg_transparency.png" width="700" height="1140.0" alt="image" class="_images" data-url="https://webtoon-phinf.pstatic.net/b.jpg?type=q90" rel="nofollow">
</div>
</body></html>`

func TestWebtoonsPages(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(webtoonsViewerFixture))
	if err != nil {
		t.Fatal(err)
	}

	got := webtoonsPages(doc)
	if len(got) != 2 {
		t.Fatalf("webtoonsPages() returned %d pages, want 2: %+v", len(got), got)
	}
	if got[0].URL != "https://webtoon-phinf.pstatic.net/a.jpg?type=q90" || got[0].Number != 1 {
		t.Errorf("page 1 = %+v", got[0])
	}
	if got[1].URL != "https://webtoon-phinf.pstatic.net/b.jpg?type=q90" || got[1].Number != 2 {
		t.Errorf("page 2 = %+v", got[1])
	}

	// a locked or pulled episode renders the reader without any pages
	doc, err = goquery.NewDocumentFromReader(strings.NewReader(`<div id="_imageList"></div>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := webtoonsPages(doc); len(got) != 0 {
		t.Errorf("webtoonsPages() on an empty reader = %+v, want none", got)
	}
}

// webtoonsAPIFixture is a trimmed page of the mobile episode API for a
// series with more episodes after it: the cursor points at the next page
const webtoonsAPIFixture = `{"result":{"episodeList":[` +
	`{"episodeNo":653,"thumbnail":"/20250116_127/thumb.png","episodeTitle":"[Season 3] Ep. 235 (Season 3 Finale)","viewerLink":"/en/fantasy/tower-of-god/season-3-ep-235-season-3-finale/viewer?title_no=95&episode_no=653","exposureDateMillis":1740362508000,"displayUp":false,"hasBgm":false},` +
	`{"episodeNo":652,"thumbnail":"/20250109_294/thumb.png","episodeTitle":"[Season 3] Ep. 234 ","viewerLink":"/en/fantasy/tower-of-god/season-3-ep-234/viewer?title_no=95&episode_no=652","exposureDateMillis":1739700000000,"displayUp":false,"hasBgm":false}` +
	`],"nextCursor":650},"success":true}`

// webtoonsAPIRefusal is the answer for an id the webtoon endpoint doesn't serve
const webtoonsAPIRefusal = `{"result":null,"message":null,"success":false}`

func TestWebtoonsAPIPage(t *testing.T) {
	base, err := url.Parse("https://www.webtoons.com/en/fantasy/tower-of-god/list?title_no=95")
	if err != nil {
		t.Fatal(err)
	}

	episodes, next, err := webtoonsAPIPage([]byte(webtoonsAPIFixture), base)
	if err != nil {
		t.Fatalf("webtoonsAPIPage() error = %v", err)
	}
	if next != 650 {
		t.Errorf("next cursor = %d, want 650", next)
	}

	want := []webtoonsEpisode{
		{
			title:  "[Season 3] Ep. 235 (Season 3 Finale)",
			number: 653,
			url:    "https://www.webtoons.com/en/fantasy/tower-of-god/season-3-ep-235-season-3-finale/viewer?title_no=95&episode_no=653",
		},
		{
			title:  "[Season 3] Ep. 234",
			number: 652,
			url:    "https://www.webtoons.com/en/fantasy/tower-of-god/season-3-ep-234/viewer?title_no=95&episode_no=652",
		},
	}
	if len(episodes) != len(want) {
		t.Fatalf("webtoonsAPIPage() returned %d episodes, want %d", len(episodes), len(want))
	}
	for i := range want {
		if episodes[i] != want[i] {
			t.Errorf("episode %d = %+v, want %+v", i, episodes[i], want[i])
		}
	}

	// the last page has no next cursor
	_, next, err = webtoonsAPIPage([]byte(`{"result":{"episodeList":[],"nextCursor":0},"success":true}`), base)
	if err != nil || next != 0 {
		t.Errorf("last page = next %d, err %v; want next 0 and no error", next, err)
	}

	// an id the API doesn't serve must make the caller fall back to the HTML list
	if _, _, err := webtoonsAPIPage([]byte(webtoonsAPIRefusal), base); err == nil {
		t.Error("webtoonsAPIPage() on a refusal returned no error")
	}
}

// webtoonsAPIPageJSON is a page of the episode API with one episode and the
// given next cursor
func webtoonsAPIPageJSON(episodeNo, next int64) string {
	return fmt.Sprintf(`{"result":{"episodeList":[{"episodeNo":%d,"episodeTitle":"Ep %d","viewerLink":"/en/fantasy/tower-of-god/ep/viewer?title_no=95&episode_no=%d"}],"nextCursor":%d},"success":true}`,
		episodeNo, episodeNo, episodeNo, next)
}

func TestWebtoonsWalkAPI(t *testing.T) {
	base, err := url.Parse("https://www.webtoons.com/en/fantasy/tower-of-god/list?title_no=95")
	if err != nil {
		t.Fatal(err)
	}

	// cursor 0 leads to 650, 650 to 300, and 300 back to 650: a cycle the
	// walk must stop at, after listing each of its three pages once
	pages := map[int64]string{
		0:   webtoonsAPIPageJSON(1, 650),
		650: webtoonsAPIPageJSON(2, 300),
		300: webtoonsAPIPageJSON(3, 650),
	}
	calls := 0
	episodes, err := webtoonsWalkAPI(base, func(cursor int64) (string, error) {
		calls++
		body, ok := pages[cursor]
		if !ok {
			return "", fmt.Errorf("unexpected cursor %d", cursor)
		}
		return body, nil
	})
	if err != nil {
		t.Fatalf("webtoonsWalkAPI() on a cycle error = %v", err)
	}
	if calls != 3 || len(episodes) != 3 {
		t.Errorf("webtoonsWalkAPI() on a cycle made %d calls and listed %d episodes, want 3 and 3", calls, len(episodes))
	}

	// a cursor that never repeats is cut off at the page cap, as an error
	// rather than a silently shortened list
	calls = 0
	_, err = webtoonsWalkAPI(base, func(cursor int64) (string, error) {
		calls++
		return webtoonsAPIPageJSON(cursor+1, cursor+1), nil
	})
	if err == nil {
		t.Error("webtoonsWalkAPI() past the page cap returned no error")
	}
	if calls != webtoonsAPIMaxPages {
		t.Errorf("webtoonsWalkAPI() past the page cap made %d calls, want %d", calls, webtoonsAPIMaxPages)
	}
}
