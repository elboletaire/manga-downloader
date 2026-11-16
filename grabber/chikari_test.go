// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestChikari points the api at a test server and paces it without the
// real three-second interval, restoring both when the test ends. The handler
// gets every api request's path and query, and the time it arrived.
func newTestChikari(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) (*Chikari, *chikariArrivals) {
	t.Helper()
	arrivals := &chikariArrivals{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arrivals.add(time.Now())
		handle(w, r)
	}))
	t.Cleanup(srv.Close)

	prevBase, prevPacer := chikariAPIBase, chikariAPIPacer
	chikariAPIBase = srv.URL + "/api"
	chikariAPIPacer = &pacer{interval: time.Millisecond}
	t.Cleanup(func() {
		chikariAPIBase, chikariAPIPacer = prevBase, prevPacer
	})

	return NewChikari(&Grabber{URL: "https://chikari.moe/series/absolute-regression", Settings: &Settings{}}), arrivals
}

// chikariArrivals records when the test server received each api request
type chikariArrivals struct {
	mu    sync.Mutex
	times []time.Time
}

func (a *chikariArrivals) add(at time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.times = append(a.times, at)
}

func (a *chikariArrivals) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.times)
}

// minGap is the smallest gap between two consecutive arrivals
func (a *chikariArrivals) minGap() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	times := append([]time.Time(nil), a.times...)
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	gap := time.Duration(1<<63 - 1)
	for i := 1; i < len(times); i++ {
		gap = min(gap, times[i].Sub(times[i-1]))
	}
	return gap
}

// chikariFixture serves a fixture file from testdata
func chikariFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestChikariTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://chikari.moe/series/absolute-regression", true},
		{"https://www.chikari.moe/series/absolute-regression/12", true},
		{"http://chikari.moe", true},
		{"https://chikari.moe.example.com/series/x", false},
		{"https://example.com/chikari.moe/series/x", false},
	}
	for _, tt := range tests {
		got, err := NewChikari(&Grabber{URL: tt.url, Settings: &Settings{}}).Test()
		if err != nil || got != tt.want {
			t.Errorf("Test(%q) = %v, %v; want %v", tt.url, got, err, tt.want)
		}
	}
}

// a series page and every reader link to it resolve to the series; the rest
// of the site (reviews, lists) is left alone
func TestChikariResolveSeriesURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://chikari.moe/series/absolute-regression", "https://chikari.moe/series/absolute-regression"},
		{"https://chikari.moe/series/absolute-regression/", "https://chikari.moe/series/absolute-regression/"},
		{"https://chikari.moe/series/absolute-regression/12", "https://chikari.moe/series/absolute-regression"},
		{"https://chikari.moe/series/absolute-regression/52.5", "https://chikari.moe/series/absolute-regression"},
		{"https://www.chikari.moe/series/absolute-regression/12?scan=asura#top", "https://www.chikari.moe/series/absolute-regression"},
		{"https://chikari.moe/series/absolute-regression/reviews/9", "https://chikari.moe/series/absolute-regression/reviews/9"},
		{"https://chikari.moe/series/absolute-regression/reviews", "https://chikari.moe/series/absolute-regression/reviews"},
		{"https://chikari.moe/", "https://chikari.moe/"},
	}
	for _, tt := range tests {
		c := NewChikari(&Grabber{URL: tt.in, Settings: &Settings{}})
		c.resolveSeriesURL()
		if c.URL != tt.want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", tt.in, c.URL, tt.want)
		}
	}
}

// one chapter per group's release: the decimal number, the volume (a string,
// a number, null) and the chapter with no scans are all read as the site writes them
func TestChikariFetchChaptersReleases(t *testing.T) {
	c, _ := newTestChikari(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(chikariFixture(t, "chikari-listing.json"))
	})

	chapters, errs := c.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}

	type want struct {
		number float64
		id     string
		group  string
		volume int
	}
	wants := []want{
		{1, "asura", "Asura", 0},
		{1, "official", "Official", 0},
		{52.5, "lagoonscans", "Lagoon Scans", 3},
		{53, "", "", 2},
		{54, "asura", "Asura", 0},
	}
	if len(chapters) != len(wants) {
		t.Fatalf("FetchChapters() got %d chapters, want %d", len(chapters), len(wants))
	}
	for i, w := range wants {
		ch := chapters[i].(*ChikariChapter)
		if ch.GetNumber() != w.number || ch.ID != w.id || ch.GetGroup() != w.group || ch.GetVolume() != w.volume {
			t.Errorf("chapter %d = number %v id %q group %q volume %d, want %+v",
				i, ch.GetNumber(), ch.ID, ch.GetGroup(), ch.GetVolume(), w)
		}
	}
}

// the listing is read page by page until the total is reached
func TestChikariFetchChaptersPages(t *testing.T) {
	var offsets []string
	var mu sync.Mutex
	c, _ := newTestChikari(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		offsets = append(offsets, r.URL.Query().Get("offset"))
		mu.Unlock()
		// two chapters a page whatever limit is asked for, five in all
		items := []string{}
		switch r.URL.Query().Get("offset") {
		case "0":
			items = []string{`{"number":1,"scans":[]}`, `{"number":2,"scans":[]}`}
		case "2":
			items = []string{`{"number":3,"scans":[]}`, `{"number":4,"scans":[]}`}
		case "4":
			items = []string{`{"number":5,"scans":[]}`}
		}
		fmt.Fprintf(w, `{"items":[%s],"total":5}`, strings.Join(items, ","))
	})

	chapters, errs := c.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 5 {
		t.Errorf("FetchChapters() got %d chapters, want 5", len(chapters))
	}
	if got := strings.Join(offsets, ","); got != "0,2,4" {
		t.Errorf("listing offsets = %s, want 0,2,4", got)
	}
}

// the pages come from the release's own scan, and the release keeps its group and ID
func TestChikariFetchChapterPages(t *testing.T) {
	var query string
	c, _ := newTestChikari(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Write(chikariFixture(t, "chikari-pages.json"))
	})
	c.resolveSeriesURL()
	listed := &ChikariChapter{Chapter{Title: "Side Story", Number: 52.5, ID: "lagoonscans", Volume: 3}}
	listed.SetGroups("Lagoon Scans")

	chapter, err := c.FetchChapter(listed)
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if query != "scan=lagoonscans" {
		t.Errorf("page request query = %q, want scan=lagoonscans", query)
	}
	if len(chapter.Pages) != 3 || chapter.PagesCount != 3 {
		t.Fatalf("pages = %d (count %d), want 3", len(chapter.Pages), chapter.PagesCount)
	}
	if chapter.Pages[2].Number != 3 || chapter.Pages[2].URL != "https://cdn.chikari.moe/series/53/ch/52.5/002.webp" {
		t.Errorf("last page = %+v", chapter.Pages[2])
	}
	if chapter.ID != "lagoonscans" || chapter.GetGroup() != "Lagoon Scans" || chapter.Volume != 3 {
		t.Errorf("release = id %q group %q volume %d", chapter.ID, chapter.GetGroup(), chapter.Volume)
	}
}

// a chapter with no scans is fetched without a scan parameter
func TestChikariFetchChapterWithoutScan(t *testing.T) {
	var query string
	c, _ := newTestChikari(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Write(chikariFixture(t, "chikari-pages.json"))
	})
	c.resolveSeriesURL()
	if _, err := c.FetchChapter(&ChikariChapter{Chapter{Number: 53}}); err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if query != "" {
		t.Errorf("page request query = %q, want none", query)
	}
}

// a club-gated chapter fails with the reason rather than an empty archive
func TestChikariClubBlockFails(t *testing.T) {
	c, _ := newTestChikari(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(chikariFixture(t, "chikari-club.json"))
	})
	c.resolveSeriesURL()
	_, err := c.FetchChapter(&ChikariChapter{Chapter{Number: 54, ID: "asura", Title: "Episode 54"}})
	if err == nil || !strings.Contains(err.Error(), "club-gated") {
		t.Fatalf("FetchChapter() error = %v, want a club-gated error", err)
	}
}

// the first 429 is a verification gate: it fails at once and no later call
// goes out, since calling again only extends the block
func TestChikariChallengeStopsCalls(t *testing.T) {
	c, arrivals := newTestChikari(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"code":"challenge_required"}`))
	})
	c.resolveSeriesURL()

	for i := range 2 {
		_, err := c.FetchChapter(&ChikariChapter{Chapter{Number: float64(i + 1)}})
		if err == nil || !strings.Contains(err.Error(), "verification") || !strings.Contains(err.Error(), "ten minutes") {
			t.Fatalf("FetchChapter() #%d error = %v, want the verification error", i+1, err)
		}
	}
	if _, errs := c.FetchChapters(); len(errs) == 0 {
		t.Error("FetchChapters() after a challenge returned no error")
	}
	if n := arrivals.count(); n != 1 {
		t.Errorf("api requests after a 429 = %d, want 1", n)
	}
}

// api calls are spaced by the shared pacer, whichever chapter makes them
func TestChikariPacesAPICalls(t *testing.T) {
	c, arrivals := newTestChikari(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(chikariFixture(t, "chikari-pages.json"))
	})
	const interval = 40 * time.Millisecond
	chikariAPIPacer = &pacer{interval: interval}
	c.resolveSeriesURL()

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.FetchChapter(&ChikariChapter{Chapter{Number: 52.5}})
		}()
	}
	wg.Wait()

	if n := arrivals.count(); n != 4 {
		t.Fatalf("api requests = %d, want 4", n)
	}
	// a sleep never returns early, so the gaps can only fall short by the
	// time the server takes to read a request; half the interval is the floor
	if gap := arrivals.minGap(); gap < interval/2 {
		t.Errorf("smallest gap between api calls = %v, want at least %v", gap, interval/2)
	}
}
