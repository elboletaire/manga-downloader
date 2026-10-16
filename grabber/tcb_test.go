// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// volumeGroupedChaptersHTML mimics the ajax/chapters response of a Madara
// theme that groups chapters under "Volume N" wrappers (e.g. gdscans.com),
// as opposed to a flat chapter list (e.g. lhtranslation.net).
const volumeGroupedChaptersHTML = `
<div class="page-content-listing single-page">
	<div class="listing-chapters_wrap cols-1 show-more">
		<ul class="main version-chap volumns">
			<li class="parent has-child active">
				<a href="javascript:void(0)" class="has-child">Volume 10</a>
				<ul class="sub-chap list-chap">
					<li>
						<ul class="sub-chap-list">
							<li class="wp-manga-chapter">
								<a href="https://gdscans.com/manga/example/volume-10/chapter-50/">Chapter 50</a>
							</li>
						</ul>
					</li>
				</ul>
			</li>
			<li class="parent has-child">
				<a href="javascript:void(0)" class="has-child">Volume 9</a>
				<ul class="sub-chap list-chap">
					<li>
						<ul class="sub-chap-list">
							<li class="wp-manga-chapter">
								<a href="https://gdscans.com/manga/example/volume-9/chapter-49/">Chapter 49</a>
							</li>
							<li class="wp-manga-chapter">
								<a href="https://gdscans.com/manga/example/volume-9/chapter-48/">Chapter 48</a>
							</li>
						</ul>
					</li>
				</ul>
			</li>
		</ul>
	</div>
</div>
`

func TestTcbFetchChaptersSkipsVolumeWrappers(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(volumeGroupedChaptersHTML))
	if err != nil {
		t.Fatalf("failed to parse fixture: %s", err)
	}

	tcb := Tcb{
		Grabber: &Grabber{URL: "https://gdscans.com/manga/example/"},
		chaps:   doc.Find("li.wp-manga-chapter"),
	}

	chapters, errs := tcb.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	// A naive body.Find("li") would also match the two "Volume N" wrapper
	// <li> elements and their nested list-wrapper <li>, yielding bogus
	// duplicate/garbage entries. Scoped to .wp-manga-chapter we must get
	// exactly the 3 real chapters, none of them "Volume N".
	if len(chapters) != 3 {
		t.Fatalf("expected 3 chapters, got %d", len(chapters))
	}

	wantNumbers := map[float64]bool{50: false, 49: false, 48: false}
	for _, c := range chapters {
		if strings.Contains(c.GetTitle(), "Volume") {
			t.Fatalf("volume wrapper leaked into chapters list: %q", c.GetTitle())
		}
		if _, ok := wantNumbers[c.GetNumber()]; !ok {
			t.Fatalf("unexpected chapter number %v", c.GetNumber())
		}
		if id := c.(*TcbChapter).ID; !strings.HasPrefix(id, "/manga/example/") {
			t.Fatalf("chapter %v ID = %q, want its URL path", c.GetNumber(), id)
		}
		wantNumbers[c.GetNumber()] = true
	}
	for num, seen := range wantNumbers {
		if !seen {
			t.Fatalf("expected chapter number %v not found", num)
		}
	}
}

func TestTcbSeriesRe(t *testing.T) {
	tests := []struct {
		url    string
		series string // empty when the URL must not match
	}{
		{"https://lhtranslation.net/manga/gaikotsu-kishi-sama/", "manga/gaikotsu-kishi-sama"},
		{"https://lilymanga.net/gl/a-love-hate-chord/", "gl/a-love-hate-chord"},
		{"https://mangalivre.to/manga/chainsaw-man-pt-br/", "manga/chainsaw-man-pt-br"},
		{"https://webtoonia.fun/seri/ask-bagimlisi/bolum-19/", "seri/ask-bagimlisi"},
		{"https://lhtranslation.net/manga/gaikotsu-kishi-sama/chapter-1/", "manga/gaikotsu-kishi-sama"},
		{"https://gdscans.com/manga/example/volume-10/chapter-50/", "manga/example"},
		{"https://lilymanga.net/gl/a-love-hate-chord/episode-02/", "gl/a-love-hate-chord"},
		{"https://lilymanga.net/gl/a-love-hate-chord", ""},
		{"https://lhtranslation.net/manga/", ""},
		{"https://example.com/somanga/foo/", ""},
		{"https://example.com/glimpse/foo/", ""},
		{"https://example.com/comics/foo/", ""},
		{"https://example.com/", ""},
	}

	for _, tt := range tests {
		match := tcbSeriesRe.FindStringSubmatch(tt.url)
		got := ""
		if match != nil {
			got = match[1]
		}
		if got != tt.series {
			t.Errorf("tcbSeriesRe on %q: got %q, want %q", tt.url, got, tt.series)
		}
	}
}

// A pasted chapter URL must download its series: the ajax/chapters list and
// the title both come from the series page, not from the chapter page.
func TestTcbChapterURLUsesSeries(t *testing.T) {
	const seriesHTML = `<html><body><h1>A Love-Hate Chord</h1></body></html>`
	const chaptersHTML = `<ul><li class="wp-manga-chapter"><a href="/gl/a-love-hate-chord/episode-01/">Episode 01</a></li></ul>`

	var requested []string
	mux := http.NewServeMux()
	mux.HandleFunc("/gl/a-love-hate-chord/ajax/chapters", func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		_, _ = w.Write([]byte(chaptersHTML))
	})
	mux.HandleFunc("/gl/a-love-hate-chord/", func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		_, _ = w.Write([]byte(seriesHTML))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tcb := NewTcb(&Grabber{URL: srv.URL + "/gl/a-love-hate-chord/episode-01/", Settings: &Settings{}})
	ok, err := tcb.Test()
	if err != nil || !ok {
		t.Fatalf("Test() = %v, %v; want the chapter URL to match its series", ok, err)
	}
	if want := srv.URL + "/gl/a-love-hate-chord/"; tcb.series != want {
		t.Fatalf("series URL = %q, want %q", tcb.series, want)
	}

	title, err := tcb.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error: %s", err)
	}
	if title != "A Love-Hate Chord" {
		t.Errorf("FetchTitle() = %q, want the series title", title)
	}

	want := []string{"/gl/a-love-hate-chord/ajax/chapters", "/gl/a-love-hate-chord/"}
	if len(requested) != len(want) || requested[0] != want[0] || requested[1] != want[1] {
		t.Errorf("requested paths = %v, want %v", requested, want)
	}
}

func TestTcbMovedHost(t *testing.T) {
	for _, in := range []string{
		"https://www.diamondfansub.com/seri/a/bolum-1/",
		"https://www.webtoonia.fun/seri/a/bolum-1/",
		"https://WWW.Webtoonia.fun/seri/a/bolum-1/",
	} {
		got := tcbMovedHostRe.ReplaceAllString(in, "${1}webtoonia.fun/")
		if want := "https://webtoonia.fun/seri/a/bolum-1/"; got != want {
			t.Errorf("rewrite of %q = %q, want %q", in, got, want)
		}
	}
}

// Madara's h1 can hold a rank badge and tab-indented text; neither belongs in
// the title.
func TestTcbFetchTitleDropsBadges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<h1>\n\t\t<span class=\"manga-title-badges custom top-3\">Top 3</span>\n\t\tGreat Heaven\n\t\tSovereign</h1>"))
	}))
	t.Cleanup(srv.Close)

	tcb := NewTcb(&Grabber{URL: srv.URL, Settings: &Settings{}})
	tcb.series = srv.URL + "/manga/x/"
	title, err := tcb.FetchTitle()
	if err != nil || title != "Great Heaven Sovereign" {
		t.Errorf("FetchTitle() = %q, %v; want %q", title, err, "Great Heaven Sovereign")
	}
}

// Themes add other h1s (a hero banner, a summary heading): .post-title wins,
// and without it only the first h1 counts.
func TestTcbFetchTitleSingleH1(t *testing.T) {
	for name, tc := range map[string]struct{ html, want string }{
		"post-title": {`<h1>Hero</h1><div class="post-title"><h1>Real</h1></div><h1>Summary</h1>`, "Real"},
		"first h1":   {`<h1>Hero</h1><div class="content"><h1>Summary</h1></div>`, "Hero"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.html))
			}))
			t.Cleanup(srv.Close)

			tcb := NewTcb(&Grabber{URL: srv.URL, Settings: &Settings{}})
			tcb.series = srv.URL + "/manga/x/"
			if title, err := tcb.FetchTitle(); err != nil || title != tc.want {
				t.Errorf("FetchTitle() = %q, %v; want %q", title, err, tc.want)
			}
		})
	}
}

func TestTcbChapterNumber(t *testing.T) {
	tests := []struct {
		title, href string
		want        float64
	}{
		{"Chapter 12.5", "https://x.com/manga/s/chapter-12-5/", 12.5},
		{"Bölüm", "https://x.com/manga/s/cilt-2/bolum-16/", 16},
		{"One-shot", "https://x.com/manga/s/one-shot/", 0},
	}
	for _, tt := range tests {
		if got := tcbChapterNumber(tt.title, tt.href); got != tt.want {
			t.Errorf("tcbChapterNumber(%q, %q) = %v, want %v", tt.title, tt.href, got, tt.want)
		}
	}
}

func TestTcbFetchChapterRejectsNonMangaReaders(t *testing.T) {
	for name, tc := range map[string]struct{ html, want string }{
		"video": {`<div class="reading-content-wrap chapter-type-video"><iframe src="https://mega.nz/embed/x"></iframe></div>`, "Ep 1 is a video, not a manga chapter"},
		"text":  {`<div class="reading-content-wrap chapter-type-text"><p>Once upon a time</p></div>`, "Ep 1 is a novel chapter, not a manga chapter"},
		"manga": {`<div class="reading-content-wrap chapter-type-manga"><div class="reading-content"><img id="image-0" data-src="/a.png"></div></div>`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.html))
			}))
			t.Cleanup(srv.Close)

			tcb := NewTcb(&Grabber{URL: srv.URL, Settings: &Settings{}})
			ch, err := tcb.FetchChapter(&TcbChapter{Chapter: Chapter{Title: "Ep 1"}, URL: srv.URL + "/manga/x/ep-1/"})
			if tc.want == "" {
				if err != nil || len(ch.Pages) != 1 {
					t.Fatalf("FetchChapter() = %v, %v; want 1 page", ch, err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Errorf("FetchChapter() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestTcbSplitTitleGroup(t *testing.T) {
	deca := Tcb{Grabber: &Grabber{URL: "https://reader.decadencescans.com/manga/ashi-girl/"}}
	other := Tcb{Grabber: &Grabber{URL: "https://example.com/manga/x/"}}
	for _, tc := range []struct {
		tcb       Tcb
		title     string
		want      string
		wantGroup string
	}{
		{deca, "30th Battle - REDLION", "30th Battle", "REDLION"},
		{deca, "35th Battle - fixed - REDLION", "35th Battle - fixed", "REDLION"},
		{deca, "30th Battle - redlion", "30th Battle", "REDLION"},
		{deca, "Extra - Part 1 - X", "Extra - Part 1 - X", ""},
		{other, "30th Battle - REDLION", "30th Battle - REDLION", ""},
	} {
		title, group := tc.tcb.splitTitleGroup(tc.title)
		if title != tc.want || group != tc.wantGroup {
			t.Errorf("%q: got %q, %q; want %q, %q", tc.title, title, group, tc.want, tc.wantGroup)
		}
	}
}

// A paged reader's pages are fetched concurrently (bounded) but must come back in page
// order with their reader page numbers.
func TestTcbFetchChapterPagedReaderConcurrent(t *testing.T) {
	const pages = 12
	const delay = 50 * time.Millisecond
	var inFlight, peak atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/x/ch-1/", func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder
		// Madara renders the pager more than once, with the same pages.
		for range 3 {
			sb.WriteString(`<select id="single-pager">`)
			for i := 1; i <= pages; i++ {
				fmt.Fprintf(&sb, `<option data-redirect="http://%s/manga/x/ch-1/p/%d/">%d</option>`, r.Host, i, i)
			}
			sb.WriteString(`</select>`)
		}
		_, _ = w.Write([]byte(sb.String()))
	})
	mux.HandleFunc("/manga/x/ch-1/p/", func(w http.ResponseWriter, r *http.Request) {
		n := strings.Trim(strings.TrimPrefix(r.URL.Path, "/manga/x/ch-1/p/"), "/")
		cur := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(delay)
		fmt.Fprintf(w, `<div class="reading-content"><img src="/img/%s.jpg"></div>`, n)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tcb := NewTcb(&Grabber{URL: srv.URL + "/manga/x/", Settings: &Settings{}})
	chapter, err := tcb.FetchChapter(&TcbChapter{Chapter{Title: "ch-1", Number: 1}, srv.URL + "/manga/x/ch-1/"})
	if err != nil {
		t.Fatalf("FetchChapter() error: %s", err)
	}
	if len(chapter.Pages) != pages {
		t.Fatalf("got %d pages, want %d", len(chapter.Pages), pages)
	}
	for i, p := range chapter.Pages {
		if want := fmt.Sprintf("%s/img/%d.jpg", srv.URL, i+1); p.URL != want || p.Number != int64(i+1) {
			t.Errorf("page %d = %d %q, want %d %q", i, p.Number, p.URL, i+1, want)
		}
	}
	if p := int(peak.Load()); p < 2 || p > tcbPageWorkers {
		t.Errorf("peak concurrent page fetches = %d, want between 2 and %d", p, tcbPageWorkers)
	}
}

// A paged reader's embedded chapter_preloaded_images list is used as is,
// without walking the pager.
func TestTcbFetchChapterPreloadedImages(t *testing.T) {
	var walked atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/x/ch-1/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<select id="single-pager"><option data-redirect="http://%[1]s/manga/x/ch-1/p/1/">1</option>
<option data-redirect="http://%[1]s/manga/x/ch-1/p/2/">2</option></select>
<script type="text/javascript" id="chapter_preloaded_images">
var chapter_preloaded_images = ["https:\/\/cdn.example.com\/b\/p000.jpg", "https:\/\/cdn.example.com\/a\/p001.jpg","\/wp-content\/p002.jpg"], chapter_images_per_page = 1;
</script>
<div class="reading-content"><img src="https://cdn.example.com/b/p000.jpg"></div>`, r.Host)
	})
	mux.HandleFunc("/manga/x/ch-1/p/", func(w http.ResponseWriter, r *http.Request) {
		walked.Add(1)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tcb := NewTcb(&Grabber{URL: srv.URL + "/manga/x/", Settings: &Settings{}})
	chapter, err := tcb.FetchChapter(&TcbChapter{Chapter{Title: "ch-1", Number: 1}, srv.URL + "/manga/x/ch-1/"})
	if err != nil {
		t.Fatalf("FetchChapter() error: %s", err)
	}
	want := []string{"https://cdn.example.com/b/p000.jpg", "https://cdn.example.com/a/p001.jpg", srv.URL + "/wp-content/p002.jpg"}
	if len(chapter.Pages) != len(want) {
		t.Fatalf("got %d pages, want %d", len(chapter.Pages), len(want))
	}
	for i, p := range chapter.Pages {
		if p.URL != want[i] || p.Number != int64(i+1) {
			t.Errorf("page %d = %d %q, want %d %q", i, p.Number, p.URL, i+1, want[i])
		}
	}
	if n := walked.Load(); n != 0 {
		t.Errorf("walked %d reader pages, want 0", n)
	}
}

// A chapter without a pager is read from the page already fetched.
func TestTcbFetchChapterSinglePageFetchesOnce(t *testing.T) {
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/manga/x/ch-1/", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprint(w, `<div class="reading-content"><img data-src=" /img/1.jpg "><img src="/img/2.jpg"></div>`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	tcb := NewTcb(&Grabber{URL: srv.URL + "/manga/x/", Settings: &Settings{}})
	chapter, err := tcb.FetchChapter(&TcbChapter{Chapter{Title: "ch-1", Number: 1}, srv.URL + "/manga/x/ch-1/"})
	if err != nil {
		t.Fatalf("FetchChapter() error: %s", err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[1].URL != srv.URL+"/img/2.jpg" {
		t.Errorf("pages = %+v, want /img/1.jpg and /img/2.jpg", chapter.Pages)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("chapter page fetched %d times, want 1", n)
	}
}

// a trailing " - 💦" marks a members-only chapter: locked, and not part of the title
func TestTcbFetchChaptersLockedMarker(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<ul>
<li class="wp-manga-chapter"><a href="https://webtoonia.fun/seri/x/bolum-6/">Bölüm 6 - 💦</a></li>
<li class="wp-manga-chapter"><a href="https://webtoonia.fun/seri/x/bolum-5/">Bölüm 5</a></li></ul>`))
	if err != nil {
		t.Fatal(err)
	}
	tcb := Tcb{Grabber: &Grabber{URL: "https://webtoonia.fun/seri/x/"}, chaps: doc.Find("li.wp-manga-chapter")}
	chapters, _ := tcb.FetchChapters()
	if len(chapters) != 2 || !LockedOf(chapters[0]) || LockedOf(chapters[1]) {
		t.Fatalf("locked flags wrong: %+v", chapters)
	}
	if got := chapters[0].GetTitle(); got != "Bölüm 6" || chapters[0].GetNumber() != 6 {
		t.Errorf("got %q (%v), want \"Bölüm 6\" (6)", got, chapters[0].GetNumber())
	}

	// another host keeps the marker in the title and the chapter unlocked
	tcb.URL = "https://example.com/manga/x/"
	chapters, _ = tcb.FetchChapters()
	if LockedOf(chapters[0]) || chapters[0].GetTitle() != "Bölüm 6 - 💦" {
		t.Errorf("other host: locked %v, title %q", LockedOf(chapters[0]), chapters[0].GetTitle())
	}
}
