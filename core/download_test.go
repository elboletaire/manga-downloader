// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
)

// recorder keeps the events in the order they were received
type recorder struct {
	mu     sync.Mutex
	events []string
	// withFetch also records Fetching and Fetched, which the other tests'
	// expected sequences leave out
	withFetch bool
}

func (r *recorder) add(format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, fmt.Sprintf(format, a...))
}

func (r *recorder) Fetching(l grabber.Filterable) {
	if r.withFetch {
		r.add("fetching %g", l.GetNumber())
	}
}
func (r *recorder) Fetched(l grabber.Filterable, _ *grabber.Chapter) {
	if r.withFetch {
		r.add("fetched %g", l.GetNumber())
	}
}
func (r *recorder) BundleStart(pages int64) { r.add("bundle-start %d", pages) }
func (r *recorder) FetchFailed(l grabber.Filterable, _ error) {
	r.add("fetch-failed %g", l.GetNumber())
}
func (r *recorder) Skipped(c *grabber.Chapter)                 { r.add("skipped %g", c.Number) }
func (r *recorder) FilenameFailed(c *grabber.Chapter, _ error) { r.add("filename-failed %g", c.Number) }
func (r *recorder) Started(c *grabber.Chapter)                 { r.add("started %g", c.Number) }
func (r *recorder) Page(c *grabber.Chapter, idx int, err error) {
	if err != nil {
		r.add("page-failed %g %d", c.Number, idx)
		return
	}
	r.add("page %g %d", c.Number, idx)
}
func (r *recorder) DownloadFailed(c *grabber.Chapter, _ error) { r.add("download-failed %g", c.Number) }
func (r *recorder) Archived(c *grabber.Chapter)                { r.add("archived %g", c.Number) }
func (r *recorder) Packed(c *grabber.Chapter, _ string, err error) {
	if err != nil {
		r.add("packed-failed %g", c.Number)
		return
	}
	r.add("packed %g", c.Number)
}
func (r *recorder) Downloaded(c *grabber.Chapter) { r.add("downloaded %g", c.Number) }
func (r *recorder) BundlePacking()                { r.add("bundle-packing") }
func (r *recorder) BundleArchived(n int)          { r.add("bundle-archived %d", n) }
func (r *recorder) VolumeStart(v *Volume)         { r.add("volume-start %d", v.Number) }
func (r *recorder) VolumeChapter(v *Volume, c *grabber.Chapter) {
	r.add("volume-chapter %d %g", v.Number, c.Number)
}
func (r *recorder) VolumePacking(v *Volume) { r.add("volume-packing %d", v.Number) }
func (r *recorder) VolumeProgress(v *Volume, f float64) {
	r.add("volume-progress %d %.3f", v.Number, f)
}
func (r *recorder) VolumePacked(v *Volume, name string, err error) {
	if err != nil {
		r.add("volume-failed %d %v", v.Number, err)
		return
	}
	r.add("volume-packed %d %s", v.Number, name)
}

// count returns how many recorded events start with prefix
func (r *recorder) count(prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

// lastBundleStart returns the page total of the last bundle-start event
func (r *recorder) lastBundleStart(t *testing.T) int64 {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	last := ""
	for _, e := range r.events {
		if strings.HasPrefix(e, "bundle-start ") {
			last = e
		}
	}
	var pages int64
	if _, err := fmt.Sscanf(last, "bundle-start %d", &pages); err != nil {
		t.Fatalf("no bundle-start event in %q", r.events)
	}
	return pages
}

// fakeChapter is a chapter the fake site serves: its pages are PNGs, and a
// page path listed in fail answers 500
type fakeChapter struct {
	number float64
	pages  int
	fail   map[int]bool
	// failFetch lists the FetchChapter calls for the chapter (1-based, in
	// order) that answer an error
	failFetch map[int]bool
	// laterPages, when set, is the page count of every fetch after the first
	laterPages int
	// limited pages answer 429 with a 60s Retry-After
	limited map[int]bool
	// volume is the volume the listing gives the chapter, 0 for none
	volume int
	// fetchDelay makes FetchChapter slow
	fetchDelay time.Duration
}

// fakeSite serves fakeChapters from an httptest server
type fakeSite struct {
	*grabber.Grabber
	chapters []fakeChapter
	server   *httptest.Server
	// fetches counts FetchChapter calls, bundle mode fetches each one twice;
	// served counts them per chapter
	mu      sync.Mutex
	fetches int
	served  map[float64]int
}

func (f *fakeSite) Test() (bool, error)                           { return true, nil }
func (f *fakeSite) FetchTitle() (string, error)                   { return "Test Series", nil }
func (f *fakeSite) FetchChapters() (grabber.Filterables, []error) { return nil, nil }

// FetchChapter returns a fresh chapter every time, as a real grabber does:
// SkipPages rewrites the page list, so one shared value would leak between
// the bundle's totals loop and its download.
func (f *fakeSite) FetchChapter(l grabber.Filterable) (*grabber.Chapter, error) {
	f.mu.Lock()
	f.fetches++
	f.mu.Unlock()

	for _, c := range f.chapters {
		if c.number != l.GetNumber() {
			continue
		}
		time.Sleep(c.fetchDelay)
		f.mu.Lock()
		f.served[c.number]++
		n := f.served[c.number]
		f.mu.Unlock()
		if c.failFetch[n] {
			return nil, errors.New("fetch failed")
		}

		pages := c.pages
		if n > 1 && c.laterPages > 0 {
			pages = c.laterPages
		}
		chapter := &grabber.Chapter{
			Title:      fmt.Sprintf("Chapter %g", c.number),
			Number:     c.number,
			PagesCount: int64(pages),
		}
		for i := 0; i < pages; i++ {
			chapter.Pages = append(chapter.Pages, grabber.Page{
				Number: int64(i + 1),
				URL:    fmt.Sprintf("%s/%g/%d", f.server.URL, c.number, i),
			})
		}
		for i := range chapter.Pages {
			if c.limited[i] {
				chapter.Pages[i].URL += "/429"
			}
		}
		if len(c.fail) > 0 {
			// the server decides by the page's path, see newFakeSite
			for i := range chapter.Pages {
				if c.fail[i] {
					chapter.Pages[i].URL += "/fail"
				}
			}
		}
		return chapter, nil
	}

	return nil, errors.New("no such chapter")
}

func newFakeSite(t *testing.T, chapters ...fakeChapter) *fakeSite {
	t.Helper()
	site := &fakeSite{
		Grabber: &grabber.Grabber{
			URL: "https://example.com/manga/test",
			Settings: &grabber.Settings{
				MaxConcurrency:   grabber.MaxConcurrency{Chapters: 1, Pages: 1},
				FilenameTemplate: packer.FilenameTemplateDefault,
				Format:           packer.FormatCBZ,
				ConvertImages:    grabber.ConvertImagesDefault,
			},
		},
		chapters: chapters,
		served:   map[float64]int{},
	}
	site.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/429") {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/fail") {
			http.Error(w, "gone", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, testImage(r.URL.Path))
	}))
	t.Cleanup(site.server.Close)
	return site
}

// testImage is a 1x1 PNG whose colour depends on the path, so pages differ
func testImage(path string) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	var sum uint8
	for i := 0; i < len(path); i++ {
		sum += path[i]
	}
	img.Set(0, 0, color.NRGBA{R: sum, G: 128, B: 255, A: 255})
	return img
}

// newSeries wires the fake site to a Series whose settings write into a
// temporary directory, returning that directory too
func newSeries(t *testing.T, site *fakeSite, bundle bool) (*Series, string) {
	t.Helper()
	dir := t.TempDir()
	site.Settings.OutputDir = dir
	site.Settings.Bundle = bundle
	return &Series{Site: site, Title: "Test Series", settings: site.Settings}, dir
}

// listed returns the listing entries for the chapters, which Download takes
func listed(site *fakeSite) grabber.Filterables {
	var out grabber.Filterables
	for _, c := range site.chapters {
		out = append(out, &grabber.Chapter{Number: c.number, Volume: c.volume})
	}
	return out
}

// cbzFiles lists the archives written to dir
func cbzFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.cbz"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func zipEntries(t *testing.T, path string) []string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

func TestDownloadNonBundle(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 2},
		fakeChapter{number: 2, pages: 1},
	)
	series, dir := newSeries(t, site, false)
	rec := &recorder{}

	name, err := series.Download(context.Background(), listed(site), nil, "", rec)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if name != "" {
		t.Errorf("non-bundle Download returned %q, want none", name)
	}

	want := []string{
		"started 1", "page 1 0", "page 1 1", "archived 1", "archived 1", "packed 1",
		"started 2", "page 2 0", "archived 2", "packed 2",
	}
	if strings.Join(rec.events, "|") != strings.Join(want, "|") {
		t.Errorf("events:\n got %q\nwant %q", rec.events, want)
	}

	if got := cbzFiles(t, dir); len(got) != 2 {
		t.Errorf("wrote %q, want one archive per chapter", got)
	}
}

func TestDownloadBundle(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 2},
		fakeChapter{number: 2, pages: 2},
	)
	series, dir := newSeries(t, site, true)
	rec := &recorder{}

	name, err := series.Download(context.Background(), listed(site), nil, "1-2", rec)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	// the totals loop fetches every chapter once, the download fetches it again
	if site.fetches != 4 {
		t.Errorf("FetchChapter called %d times, want 4 (totals loop + download)", site.fetches)
	}
	if rec.events[0] != "bundle-start 4" {
		t.Errorf("first event %q, want bundle-start with the 4 pages of both chapters", rec.events[0])
	}
	if got := rec.count("downloaded"); got != 2 {
		t.Errorf("got %d downloaded events, want 2", got)
	}
	if got := rec.count("packed"); got != 0 {
		t.Errorf("bundle mode packed %d chapters on their own, want none", got)
	}
	if got := rec.count("bundle-archived"); got != 4 {
		t.Errorf("got %d bundle-archived ticks, want one per page", got)
	}
	if rec.events[len(rec.events)-1] != "bundle-archived 1" {
		t.Errorf("last event %q, want the bundle archive", rec.events[len(rec.events)-1])
	}

	if name == "" {
		t.Fatal("bundle Download returned no filename")
	}
	entries := zipEntries(t, filepath.Join(dir, name))
	if len(entries) != 4 {
		t.Fatalf("bundle holds %d entries, want 4: %q", len(entries), entries)
	}
	if !strings.HasSuffix(entries[0], "/000.png") {
		t.Errorf("bundle entry %q is not a named page of its chapter folder", entries[0])
	}
}

func TestDownloadSkipPagesAll(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 2},
		fakeChapter{number: 2, pages: 2},
	)
	series, _ := newSeries(t, site, false)
	rec := &recorder{}

	// every page of chapter 1 is excluded: it's skipped before any download
	skip := []ranges.Range{{Begin: 1, End: 2}}
	if _, err := series.Download(context.Background(), listed(site)[:1], skip, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if got := strings.Join(rec.events, "|"); got != "skipped 1" {
		t.Errorf("events %q, want only the skip", got)
	}
}

func TestDownloadSkipPagesKeepsTheRest(t *testing.T) {
	site := newFakeSite(t, fakeChapter{number: 1, pages: 4})
	series, dir := newSeries(t, site, false)
	rec := &recorder{}

	// "2,-1" drops the second and the last page, keeping pages 1 and 3
	skip, err := ranges.ParseRelative("2,-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := series.Download(context.Background(), listed(site), skip, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if got := rec.count("page "); got != 2 {
		t.Errorf("downloaded %d pages, want the 2 kept", got)
	}
	// the bar total is the post-skip count: two pages downloaded, two archived
	if got := rec.count("archived"); got != 2 {
		t.Errorf("archived %d pages, want 2", got)
	}
	entries := zipEntries(t, cbzFiles(t, dir)[0])
	if len(entries) != 2 {
		t.Errorf("archive holds %d entries, want 2: %q", len(entries), entries)
	}
}

func TestDownloadFailingPage(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 2, fail: map[int]bool{1: true}},
		fakeChapter{number: 2, pages: 1},
	)
	series, dir := newSeries(t, site, false)
	rec := &recorder{}

	// a failing page fails its own chapter only, not the rest of the run
	if _, err := series.Download(context.Background(), listed(site), nil, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if got := rec.count("page-failed 1 1"); got != 1 {
		t.Errorf("want one page-failed event for page 1, got events %q", rec.events)
	}
	if got := rec.count("download-failed 1"); got != 1 {
		t.Errorf("want download-failed for chapter 1, got events %q", rec.events)
	}
	if got := rec.count("packed 1"); got != 0 {
		t.Errorf("chapter 1 failed, must not be packed")
	}
	if got := rec.count("packed 2"); got != 1 {
		t.Errorf("chapter 2 must still be packed")
	}
	// only chapter 2 is archived: the failed one must not leave a file behind
	if got := cbzFiles(t, dir); len(got) != 1 {
		t.Errorf("wrote %q, want chapter 2's archive alone", got)
	}
}

// checkBundleTotal asserts that the last bundle total is the number of pages
// the bundle's events count: one page event and one archive tick per page
func checkBundleTotal(t *testing.T, rec *recorder) {
	t.Helper()
	total := rec.lastBundleStart(t)
	if got := int64(rec.count("page ")); got != total {
		t.Errorf("got %d page events, want the bundle total %d: %q", got, total, rec.events)
	}
	if got := int64(rec.count("bundle-archived ")); got != total {
		t.Errorf("got %d bundle-archived ticks, want the bundle total %d: %q", got, total, rec.events)
	}
}

func TestDownloadBundleTotalWhenPrepassFails(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 2, failFetch: map[int]bool{1: true}},
		fakeChapter{number: 2, pages: 2},
	)
	series, dir := newSeries(t, site, true)
	rec := &recorder{}

	name, err := series.Download(context.Background(), listed(site), nil, "1-2", rec)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	// the totals fetch of chapter 1 failed, so it's counted from its download
	if rec.events[0] != "bundle-start 2" {
		t.Errorf("first event %q, want bundle-start 2 without chapter 1", rec.events[0])
	}
	if got := rec.count("downloaded"); got != 2 {
		t.Errorf("got %d downloaded events, want both chapters", got)
	}
	if got := rec.lastBundleStart(t); got != 4 {
		t.Errorf("last bundle total %d, want 4", got)
	}
	checkBundleTotal(t, rec)
	if entries := zipEntries(t, filepath.Join(dir, name)); len(entries) != 4 {
		t.Errorf("bundle holds %d entries, want 4: %q", len(entries), entries)
	}
}

func TestDownloadBundleTotalWhenDownloadFindsMorePages(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 2, laterPages: 3},
		fakeChapter{number: 2, pages: 2},
	)
	series, _ := newSeries(t, site, true)
	rec := &recorder{}

	if _, err := series.Download(context.Background(), listed(site), nil, "1-2", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if got := rec.lastBundleStart(t); got != 5 {
		t.Errorf("last bundle total %d, want 5 (chapter 1 now has 3 pages)", got)
	}
	checkBundleTotal(t, rec)
}

func TestDownloadBundleTotalWhenDownloadFindsFewerPages(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 3, laterPages: 1},
		fakeChapter{number: 2, pages: 2},
	)
	series, _ := newSeries(t, site, true)
	rec := &recorder{}

	if _, err := series.Download(context.Background(), listed(site), nil, "1-2", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if got := rec.lastBundleStart(t); got != 3 {
		t.Errorf("last bundle total %d, want 3 (chapter 1 now has 1 page)", got)
	}
	checkBundleTotal(t, rec)
}

func TestDownloadBundleSkipPagesLastStillDropsOnePerChapter(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 3},
		fakeChapter{number: 2, pages: 3},
	)
	series, _ := newSeries(t, site, true)
	rec := &recorder{}

	skip, err := ranges.ParseRelative("-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := series.Download(context.Background(), listed(site), skip, "1-2", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// both fetches drop the same page, so the total is never corrected
	if got := rec.count("bundle-start"); got != 1 {
		t.Errorf("got %d bundle-start events, want 1: %q", got, rec.events)
	}
	if got := rec.lastBundleStart(t); got != 4 {
		t.Errorf("bundle total %d, want 4 (two pages per chapter)", got)
	}
	checkBundleTotal(t, rec)
}

func TestDownloadBundleTotalWhenDownloadFetchFails(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 2, failFetch: map[int]bool{2: true}},
		fakeChapter{number: 2, pages: 2},
	)
	series, dir := newSeries(t, site, true)
	rec := &recorder{}

	name, err := series.Download(context.Background(), listed(site), nil, "1-2", rec)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	// the totals fetch counted chapter 1, its failed download keeps that
	// count (the bar shows the failure), and the bundle is packed without it
	if got := rec.count("bundle-start"); got != 1 {
		t.Errorf("got %d bundle-start events, want 1: %q", got, rec.events)
	}
	if got := rec.lastBundleStart(t); got != 4 {
		t.Errorf("last bundle total %d, want 4", got)
	}
	if got := rec.count("fetch-failed 1"); got != 1 {
		t.Errorf("want fetch-failed for chapter 1: %q", rec.events)
	}
	if got := int64(rec.count("bundle-archived ")); got != 2 {
		t.Errorf("got %d bundle-archived ticks, want chapter 2's 2 pages: %q", got, rec.events)
	}
	if entries := zipEntries(t, filepath.Join(dir, name)); len(entries) != 2 {
		t.Errorf("bundle holds %d entries, want chapter 2's 2 pages: %q", len(entries), entries)
	}
}

func TestDownloadBundleWithFailingChapter(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 1, fail: map[int]bool{0: true}},
		fakeChapter{number: 2, pages: 1},
	)
	series, dir := newSeries(t, site, true)
	rec := &recorder{}

	name, err := series.Download(context.Background(), listed(site), nil, "1-2", rec)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	// the bundle is still packed from the chapters that made it
	if got := rec.count("download-failed 1"); got != 1 {
		t.Errorf("want download-failed for chapter 1, got %q", rec.events)
	}
	if entries := zipEntries(t, filepath.Join(dir, name)); len(entries) != 1 {
		t.Errorf("bundle holds %d entries, want only chapter 2's single page: %q", len(entries), entries)
	}
}

func TestDownloadCancelledBeforeStart(t *testing.T) {
	site := newFakeSite(t, fakeChapter{number: 1, pages: 1})
	series, dir := newSeries(t, site, true)
	rec := &recorder{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := series.Download(ctx, listed(site), nil, "1", rec)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	// a cancelled bundle must not be written from whatever it had
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("cancelled run wrote %d files", len(entries))
	}
}

func TestDownloadNilEventsAreIgnored(t *testing.T) {
	site := newFakeSite(t, fakeChapter{number: 1, pages: 1})
	series, _ := newSeries(t, site, false)

	if _, err := series.Download(context.Background(), listed(site), nil, "", nil); err != nil {
		t.Fatalf("Download with nil events: %v", err)
	}
}

// failures records the errors FetchFailed reports, by chapter number
type failures struct {
	NoEvents
	errs map[float64]error
}

func (f *failures) FetchFailed(l grabber.Filterable, err error) {
	f.errs[l.GetNumber()] = err
}

// a chapter the listing marks as locked fails before its reader page is
// fetched, in both modes (bundle mode also counts pages before downloading),
// and the rest of the run goes on
func TestDownloadLockedChapterFailsUnfetched(t *testing.T) {
	for _, bundle := range []bool{false, true} {
		site := newFakeSite(t,
			fakeChapter{number: 1, pages: 1},
			fakeChapter{number: 2, pages: 1},
		)
		series, dir := newSeries(t, site, bundle)
		ev := &failures{errs: map[float64]error{}}
		chapters := listed(site)
		chapters[0].(*grabber.Chapter).Locked = true

		if _, err := series.Download(context.Background(), chapters, nil, "", ev); err != nil {
			t.Fatalf("bundle=%v: Download: %v", bundle, err)
		}

		if got := site.served[1]; got != 0 {
			t.Errorf("bundle=%v: locked chapter was fetched %d times, want 0", bundle, got)
		}
		if err := ev.errs[1]; !errors.Is(err, ErrLocked) {
			t.Errorf("bundle=%v: chapter 1 failed with %v, want ErrLocked", bundle, err)
		}
		if _, ok := ev.errs[2]; ok {
			t.Errorf("bundle=%v: chapter 2 failed: %v", bundle, ev.errs[2])
		}
		if !bundle {
			if got := cbzFiles(t, dir); len(got) != 1 {
				t.Errorf("bundle=%v: wrote %q, want chapter 2's archive alone", bundle, got)
			}
		}
	}
}

// a locked chapter inside a volume is never fetched, yet the volume is packed
// from the others and its progress still reaches 1
func TestDownloadLockedChapterInVolume(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 1},
		fakeChapter{number: 2, pages: 2},
	)
	series, dir := newSeries(t, site, false)
	site.Settings.Volumes = "1-2"
	rec := &recorder{}
	chapters := listed(site)
	chapters[0].(*grabber.Chapter).Locked = true

	if _, err := series.Download(context.Background(), chapters, nil, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if got := site.served[1]; got != 0 {
		t.Errorf("locked chapter was fetched %d times, want 0", got)
	}
	if got := rec.count("fetch-failed 1"); got != 1 {
		t.Errorf("want fetch-failed for chapter 1: %q", rec.events)
	}
	if got := rec.count("volume-packed 1 "); got != 1 {
		t.Errorf("want volume 1 packed: %q", rec.events)
	}
	var last string
	for _, e := range rec.events {
		if strings.HasPrefix(e, "volume-progress 1 ") {
			last = e
		}
	}
	if last != "volume-progress 1 1.000" {
		t.Errorf("last volume progress %q, want 1.000", last)
	}
	files := cbzFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("wrote %q, want the volume alone", files)
	}
	if entries := zipEntries(t, files[0]); len(entries) != 2 {
		t.Errorf("volume holds %q, want chapter 2's 2 pages", entries)
	}
}

// Fetching comes before the page list is fetched, and Fetched pairs the
// listing entry with the chapter right after, ahead of Started.
func TestDownloadFetchingBeforeStarted(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 1, fetchDelay: 20 * time.Millisecond},
		fakeChapter{number: 2, pages: 1, failFetch: map[int]bool{1: true}},
	)
	series, _ := newSeries(t, site, false)
	rec := &recorder{withFetch: true}

	if _, err := series.Download(context.Background(), listed(site), nil, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	want := []string{
		"fetching 1", "fetched 1", "started 1", "page 1 0", "archived 1", "packed 1",
		"fetching 2", "fetch-failed 2",
	}
	if strings.Join(rec.events, "|") != strings.Join(want, "|") {
		t.Errorf("events:\n got %q\nwant %q", rec.events, want)
	}
}

// The bundle's totals pass doesn't report Fetching: only the download's fetch
// does, once per chapter.
func TestDownloadBundleFetchingOncePerChapter(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 1},
		fakeChapter{number: 2, pages: 1},
	)
	series, _ := newSeries(t, site, true)
	rec := &recorder{withFetch: true}

	if _, err := series.Download(context.Background(), listed(site), nil, "1-2", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if got := rec.count("fetching "); got != 2 {
		t.Errorf("got %d fetching events, want 2: %q", got, rec.events)
	}
	if rec.events[0] != "bundle-start 2" {
		t.Errorf("first event %q, want the totals pass to report nothing before bundle-start", rec.events[0])
	}
	for _, n := range []string{"1", "2"} {
		if indexOf(rec.events, "fetching "+n) > indexOf(rec.events, "started "+n) {
			t.Errorf("chapter %s started before it was fetching: %q", n, rec.events)
		}
	}
}

// A volume's chapters report Fetching too, ahead of VolumeChapter.
func TestDownloadVolumeFetchingBeforeVolumeChapter(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 1},
		fakeChapter{number: 2, pages: 1},
	)
	series, _ := newSeries(t, site, false)
	site.Settings.Volumes = "1-2"
	rec := &recorder{withFetch: true}

	if _, err := series.Download(context.Background(), listed(site), nil, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	for _, n := range []string{"1", "2"} {
		f, v := indexOf(rec.events, "fetching "+n), indexOf(rec.events, "volume-chapter 1 "+n)
		if f < 0 || v < 0 || f > v {
			t.Errorf("chapter %s: fetching at %d, volume-chapter at %d: %q", n, f, v, rec.events)
		}
	}
}

// blockedSite holds FetchChapter until release is closed, as a slow grabber
// that takes no context does.
type blockedSite struct {
	*fakeSite
	entered chan struct{}
	release chan struct{}
}

func (b *blockedSite) FetchChapter(l grabber.Filterable) (*grabber.Chapter, error) {
	b.entered <- struct{}{}
	<-b.release
	return b.fakeSite.FetchChapter(l)
}

// A run cancelled while a chapter's page list is being fetched can't stop the
// fetch, so the chapter must still end instead of staying in "fetching".
func TestDownloadCancelledWhileFetching(t *testing.T) {
	site := &blockedSite{
		fakeSite: newFakeSite(t, fakeChapter{number: 1, pages: 1}),
		entered:  make(chan struct{}, 1),
		release:  make(chan struct{}),
	}
	series, _ := newSeries(t, site.fakeSite, false)
	series.Site = site
	rec := &recorder{withFetch: true}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := series.Download(ctx, listed(site.fakeSite), nil, "", rec)
		done <- err
	}()
	<-site.entered
	cancel()
	close(site.release)
	<-done

	ended := false
	for _, e := range rec.events {
		switch e {
		case "download-failed 1", "packed 1", "fetch-failed 1":
			ended = true
		}
	}
	if rec.events[0] != "fetching 1" || !ended {
		t.Errorf("events %q, want fetching followed by the chapter ending", rec.events)
	}
}

func indexOf(events []string, e string) int {
	for i, got := range events {
		if got == e {
			return i
		}
	}
	return -1
}
