// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/elboletaire/manga-downloader/http"
)

// toonzPage wraps an RSC stream the way the site's pages ship it: one
// self.__next_f.push call holding the stream as a JSON string
func toonzPage(stream string) string {
	quoted, _ := json.Marshal(stream)
	return `<html><body><script>self.__next_f.push([1,` + string(quoted) + `])</script></body></html>`
}

func TestToonzResolveSeriesURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"https://toonz.to/manga/one-piece-9plei/chapter/1200", "https://toonz.to/manga/one-piece-9plei"},
		{"https://toonx.to/adult/big-brother/chapter/1-abc", "https://toonx.to/adult/big-brother"},
		{"https://toonx.to/adult/big-brother", "https://toonx.to/adult/big-brother"},
		{"https://toonz.to/manhwa/some-series/chapter/1", "https://toonz.to/manhwa/some-series"},
		{"https://www.toonz.to/manhwa/some-series/chapter/4.01", "https://www.toonz.to/manhwa/some-series"},
		{"https://toonz.to/manhwa/some-series/chapter/1?x=1", "https://toonz.to/manhwa/some-series"},
		{"https://toonz.to/manhwa/some-series", "https://toonz.to/manhwa/some-series"},
		{"https://TOONZ.to/Manhwa/Some-Series/Chapter/1", "https://TOONZ.to/Manhwa/Some-Series"},
	}

	for _, c := range cases {
		tz := &Toonz{Grabber: &Grabber{URL: c.input}}
		tz.resolveSeriesURL()
		if tz.URL != c.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", c.input, tz.URL, c.want)
		}
	}
}

// toonzChapterListJSON is the start of the chapter API's response for One Piece
// (1305 chapters in all), with its first entry, the one without a title, and the last
const toonzChapterListJSON = `{"chapters":[` +
	`{"id":13749229,"chapterNumber":"1200.00","title":null,"slug":"chapter-1200","pageCount":17,"date":"2026-11-27T02:10:13.000Z"},` +
	`{"id":13704791,"chapterNumber":"1199.00","title":null,"slug":"chapter-1199","pageCount":13,"date":"2026-11-19T20:35:15.000Z"},` +
	`{"id":10051913,"chapterNumber":"0.00","title":null,"slug":"chapter-0-85cr1","pageCount":51,"date":"2022-06-25T17:46:32.000Z"}]}`

func TestToonzParseChapterList(t *testing.T) {
	chapters, err := parseToonzChapterList([]byte(toonzChapterListJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(chapters))
	}
	if chapters[0].ID != 13749229 || chapters[0].ChapterNumber != "1200.00" || chapters[0].Slug != "chapter-1200" {
		t.Errorf("first chapter = %+v", chapters[0])
	}
	if chapters[2].ChapterNumber != "0.00" || chapters[2].Title != "" || chapters[2].Slug != "chapter-0-85cr1" {
		t.Errorf("last chapter = %+v", chapters[2])
	}
}

func TestToonzComicID(t *testing.T) {
	// the page carries two comics' cards: the series' own is the one whose slug matches its URL
	stream := `{"comicId":1010782,"comicSlug":"one-piece","comicType":"manga"},` +
		`{"comicId":1010782,"comicSlug":"one-piece-9plei","comicType":"manga","chapterCount":1305},` +
		`{"comicId":2066,"comicSlug":"other-series-abc","comicType":"manga"}`

	id, err := toonzComicID(stream, "https://toonz.to/manga/one-piece-9plei")
	if err != nil {
		t.Fatal(err)
	}
	if id != "1010782" {
		t.Errorf("comic ID = %q, want 1010782", id)
	}

	if _, err := toonzComicID(stream, "https://toonz.to/manga/missing-series"); err == nil {
		t.Error("expected an error for a series whose comic is not on the page")
	}
}

func TestToonzChapterCount(t *testing.T) {
	// the count follows the initialChapters array, as on the real page
	stream := `{"comicSlug":"one-piece-9plei","initialChapters":[{"id":13749229,"chapterNumber":"1200.00"}],` +
		`"chapterCount":1305,"comicType":"manga"}`

	n, err := toonzChapterCount(stream)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1305 {
		t.Errorf("chapter count = %d, want 1305", n)
	}

	if _, err := toonzChapterCount(`{"comicSlug":"x"}`); err == nil {
		t.Error("expected an error when the page has no chapter count")
	}

	// a related-series card with its own count after the series' must not change it
	related := stream + `,{"comicSlug":"other","chapterCount":42}`
	n, err = toonzChapterCount(related)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1305 {
		t.Errorf("chapter count with a related card = %d, want 1305", n)
	}
}

// TestToonzCheckCount covers the guard: a list shorter than the site's count
// must fail, whatever the page's escaping, since that was the silent failure
func TestToonzCheckCount(t *testing.T) {
	if err := checkToonzCount(40, 1305); err == nil || !strings.Contains(err.Error(), "40 of 1305") {
		t.Errorf("short list: err = %v, want an incomplete-list error", err)
	}
	if err := checkToonzCount(1305, 1305); err != nil {
		t.Errorf("full list: err = %v", err)
	}
}

func TestToonzChapterImages(t *testing.T) {
	stream := `5:["$","$L1f",null,{"chapter":{"id":66038,"chapterNumber":"1.00",` +
		`"images":[{"id":1,"filename":"chapters/1300/66038/001-01.webp","width":800,"height":1392,"sortOrder":1},` +
		`{"id":2,"filename":"chapters/1300/66038/001-02.webp","width":800,"height":1392,"sortOrder":2}]}}]`

	names, err := toonzChapterImages(toonzPage(stream))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"chapters/1300/66038/001-01.webp", "chapters/1300/66038/001-02.webp"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("images = %v, want %v", names, want)
	}
}

// TestToonzFetchChaptersURLAndID checks a chapter's reader URL comes from its
// slug (a float would turn "4.10" into "4.1") and its ID from the payload's id
func TestToonzFetchChaptersURLAndID(t *testing.T) {
	tz := &Toonz{
		Grabber: &Grabber{URL: "https://toonz.to/manhwa/some-series"},
		series: &toonzSeries{
			title: "Some Series",
			chapters: []toonzListedChapter{
				{ID: 10041845, ChapterNumber: "4.10", Slug: "chapter-4.10", Title: "Prologue"},
				{ID: 66038, ChapterNumber: "1.00", Slug: "chapter-1"},
			},
		},
	}

	chapters, errs := tz.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}

	want := []struct {
		url, id string
		number  float64
	}{
		{"https://toonz.to/manhwa/some-series/chapter/4.10", "10041845", 4.1},
		{"https://toonz.to/manhwa/some-series/chapter/1", "66038", 1},
	}
	if len(chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i].(*ToonzChapter)
		if c.URL != w.url || c.ID != w.id || c.Number != w.number {
			t.Errorf("chapter %d = {URL %q, ID %q, Number %v}, want {%q, %q, %v}", i, c.URL, c.ID, c.Number, w.url, w.id, w.number)
		}
	}
}

// TestToonzReaderRefreshesExpiredSession fakes a session whose clearance has
// expired: the reader answers 403 until the series page is rendered again. Five
// chapters fail together, and the series page must be rendered exactly once.
func TestToonzReaderRefreshesExpiredSession(t *testing.T) {
	var mu sync.Mutex
	fresh := false
	renders := 0

	tz := NewToonz(&Grabber{URL: "https://toonz.to/manga/one-piece-9plei"})
	tz.get = func(rawURL, referer string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		if !fresh {
			return nil, &http.StatusError{Code: 403}
		}
		return []byte("ok"), nil
	}
	tz.render = func(seriesURL string) error {
		mu.Lock()
		defer mu.Unlock()
		renders++
		fresh = true
		return nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body, err := tz.readerPage("https://toonz.to/manga/one-piece-9plei/chapter/1")
			if err == nil && string(body) != "ok" {
				err = fmt.Errorf("body = %q, want ok", body)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("readerPage: %v", err)
		}
	}
	if renders != 1 {
		t.Errorf("series page rendered %d times, want 1", renders)
	}
}

// TestToonzReaderOtherErrorsDontRefresh checks only a 403 or 503 renders the
// series page again: a 404 is the chapter's own problem.
func TestToonzReaderOtherErrorsDontRefresh(t *testing.T) {
	renders := 0
	tz := NewToonz(&Grabber{URL: "https://toonz.to/manga/one-piece-9plei"})
	tz.get = func(rawURL, referer string) ([]byte, error) {
		return nil, &http.StatusError{Code: 404}
	}
	tz.render = func(seriesURL string) error {
		renders++
		return nil
	}

	if _, err := tz.readerPage("https://toonz.to/manga/one-piece-9plei/chapter/1"); err == nil {
		t.Error("readerPage: want the 404 back")
	}
	if renders != 0 {
		t.Errorf("series page rendered %d times for a 404, want 0", renders)
	}
}

// TestToonzFailedRefreshSticks: a render that fails reaches every chapter that
// was waiting on it, and the chapters after it, without rendering again
func TestToonzFailedRefreshSticks(t *testing.T) {
	var mu sync.Mutex
	renders := 0
	errRender := errors.New("browser closed")

	tz := NewToonz(&Grabber{URL: "https://toonz.to/manga/one-piece-9plei"})
	tz.get = func(rawURL, referer string) ([]byte, error) {
		return nil, &http.StatusError{Code: 403}
	}
	tz.render = func(seriesURL string) error {
		mu.Lock()
		defer mu.Unlock()
		renders++
		return errRender
	}

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := tz.readerPage("https://toonz.to/manga/one-piece-9plei/chapter/1")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	failed := 0
	for err := range errs {
		if errors.Is(err, errRender) {
			failed++
		}
	}
	if failed != 5 {
		t.Errorf("%d chapters got the render error, want 5", failed)
	}

	// a chapter fetched after the failure gets the same error without a render
	if _, err := tz.readerPage("https://toonz.to/manga/one-piece-9plei/chapter/2"); !errors.Is(err, errRender) {
		t.Errorf("later chapter: err = %v, want the render error", err)
	}

	if renders != 1 {
		t.Errorf("series page rendered %d times, want 1", renders)
	}
}
