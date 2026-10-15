// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"bytes"
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
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// fakeChapter is a chapter the fake site serves; pages listed in fail answer 500
type fakeChapter struct {
	number float64
	pages  int
	fail   map[int]bool
	// volume is the volume the site lists the chapter under, 0 for none
	volume int
}

// listing is a chapter as listed by the fake site
type listing struct {
	number float64
	title  string
	volume int
}

func (l listing) GetNumber() float64 { return l.number }
func (l listing) GetVolume() int     { return l.volume }
func (l listing) GetTitle() string   { return l.title }

// fakeSite serves fakeChapters from an httptest server. When gate is set,
// FetchChapter blocks until it's closed, so a test can cancel mid-run.
type fakeSite struct {
	*grabber.Grabber
	server   *httptest.Server
	chapters []fakeChapter
	// gate, when set, makes FetchChapter announce itself on entered and then
	// wait for gate to be closed
	gate    chan struct{}
	entered chan struct{}
	// pageGate, when set, holds the pages of chapter pageGated until closed
	pageGate  chan struct{}
	pageGated float64
}

func (f *fakeSite) Test() (bool, error) { return true, nil }
func (f *fakeSite) FetchTitle() (string, error) {
	return "Test Series", nil
}

func (f *fakeSite) FetchChapters() (grabber.Filterables, []error) {
	out := grabber.Filterables{}
	for _, c := range f.chapters {
		out = append(out, listing{number: c.number, title: fmt.Sprintf("Chapter %g", c.number), volume: c.volume})
	}
	return out, nil
}

func (f *fakeSite) FetchChapter(l grabber.Filterable) (*grabber.Chapter, error) {
	if f.gate != nil {
		f.entered <- struct{}{}
		<-f.gate
	}
	for _, c := range f.chapters {
		if c.number != l.GetNumber() {
			continue
		}
		chapter := &grabber.Chapter{
			Title:      l.GetTitle(),
			Number:     c.number,
			PagesCount: int64(c.pages),
		}
		for i := 0; i < c.pages; i++ {
			url := fmt.Sprintf("%s/%g/%d", f.server.URL, c.number, i)
			if c.fail[i] {
				url += "/fail"
			}
			chapter.Pages = append(chapter.Pages, grabber.Page{Number: int64(i + 1), URL: url})
		}
		return chapter, nil
	}
	return nil, fmt.Errorf("no chapter %g", l.GetNumber())
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		img.Set(x, 0, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newTestController returns a controller over a fake site serving chapters
// 1..n (with the given overrides), writing into a temp directory
func newTestController(t *testing.T, bundle bool, gate chan struct{}, chapters ...fakeChapter) (*Controller, *fakeSite, string) {
	t.Helper()
	img := pngBytes(t)
	var site *fakeSite
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if site.pageGate != nil && strings.HasPrefix(r.URL.Path, fmt.Sprintf("/%g/", site.pageGated)) {
			<-site.pageGate
		}
		if strings.HasSuffix(r.URL.Path, "/fail") {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(img)
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	settings := &grabber.Settings{
		Bundle:           bundle,
		MaxConcurrency:   grabber.MaxConcurrency{Chapters: 2, Pages: 2},
		Format:           "cbz",
		ConvertImages:    "none",
		FilenameTemplate: "{{.Series}} {{.Number}} - {{.Title}}",
		OutputDir:        dir,
	}
	site = &fakeSite{
		Grabber:  &grabber.Grabber{URL: "https://example.com/manga/test", Settings: settings},
		server:   server,
		chapters: chapters,
		gate:     gate,
		entered:  make(chan struct{}, 16),
	}

	c := NewController(settings, nil)
	// like core.Identify, the site reads the settings it's given
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return site, nil
	}
	return c, site, dir
}

// waitFinished polls until the run has a summary
func waitFinished(t *testing.T, c *Controller) *Summary {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if s := c.Summary(); s != nil && !c.Running() {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("download did not finish")
	return nil
}

func chapters(n int, pages int) []fakeChapter {
	out := make([]fakeChapter, n)
	for i := range out {
		out[i] = fakeChapter{number: float64(i + 1), pages: pages}
	}
	return out
}

func TestLoadListsChaptersAndSelection(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 2)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if c.Title() != "Test Series" || len(c.Chapters()) != 5 {
		t.Fatalf("title %q, %d chapters", c.Title(), len(c.Chapters()))
	}

	n, err := c.SelectRange("2-3,5")
	if err != nil || n != 3 {
		t.Fatalf("SelectRange = %d, %v; want 3, nil", n, err)
	}
	if c.SelectedCount() != 3 || c.IsSelected(0) || !c.IsSelected(1) {
		t.Errorf("selection after range is wrong: %d selected", c.SelectedCount())
	}

	c.SetAll(false)
	if c.SelectedCount() != 0 {
		t.Errorf("SetAll(false) left %d selected", c.SelectedCount())
	}
	c.SetAll(true)
	if c.SelectedCount() != 5 {
		t.Errorf("SetAll(true) selected %d", c.SelectedCount())
	}
	if _, err := c.SelectRange("nope"); err == nil {
		t.Error("SelectRange accepted an invalid range")
	}
}

func TestClearForgetsTheSeriesAndKeepsTheOptions(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 2)...)
	if err := c.Configure(func(s *grabber.Settings) { s.Format = "epub" }); err != nil {
		t.Fatal(err)
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SelectRange("1-2"); err != nil {
		t.Fatal(err)
	}
	// a finished run's results, as the Activity tab shows them
	c.mu.Lock()
	c.jobs = []*Job{{}}
	c.summary = &Summary{Saved: 2}
	c.mu.Unlock()
	gen, logged := c.SelectionGen(), len(c.Log())

	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if c.Loaded() || c.URL() != "" || c.Title() != "" || len(c.Chapters()) != 0 || c.SelectedCount() != 0 {
		t.Errorf("Clear left a series: url %q, loaded %v, %d chapters", c.URL(), c.Loaded(), len(c.Chapters()))
	}
	if c.Summary() != nil || len(c.Jobs()) != 0 {
		t.Errorf("Clear left the results: summary %v, %d jobs", c.Summary(), len(c.Jobs()))
	}
	if c.SelectionGen() == gen {
		t.Error("Clear didn't change the selection generation, so the window keeps the old selection")
	}
	if language, scanlator := c.Filters(); language != nil || scanlator != nil {
		t.Errorf("Clear left the filters: language %v, scanlator %v", language, scanlator)
	}
	if len(c.Log()) != logged {
		t.Errorf("Clear changed the log: %d lines, want %d", len(c.Log()), logged)
	}
	if c.Settings().Format != "epub" {
		t.Errorf("Clear changed the options: format %q", c.Settings().Format)
	}
}

func TestClearClosesTheBrowser(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	closed := 0
	c.closeBrowser = func() { closed++ }
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}

	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	// it closes in the background
	c.closing.Wait()
	if closed != 1 {
		t.Errorf("Clear closed the browser %d times, want once", closed)
	}
}

func TestClearEmitsBeforeTheBrowserCloses(t *testing.T) {
	notified := make(chan struct{}, 100)
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.notify = func() {
		select {
		case notified <- struct{}{}:
		default:
		}
	}
	unblock := make(chan struct{})
	closed := make(chan struct{})
	c.closeBrowser = func() {
		<-unblock
		close(closed)
	}

	done := make(chan error, 1)
	go func() { done <- c.Clear() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Clear waited for the browser to close")
	}
	select {
	case <-notified:
	default:
		t.Error("Clear didn't emit the cleared state before the browser closed")
	}
	if c.Loaded() {
		t.Error("the series is still loaded")
	}

	// a load issued meanwhile waits for the close, then works
	loaded := make(chan error, 1)
	go func() { loaded <- c.Load("https://example.com/manga/test") }()
	select {
	case err := <-loaded:
		t.Fatalf("the load didn't wait for the closing browser: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(unblock)
	<-closed
	select {
	case err := <-loaded:
		if err != nil {
			t.Fatalf("load after the close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the load never finished")
	}
	if !c.Loaded() {
		t.Error("the load after the close didn't load the series")
	}
}

func TestClearRefusedWhileLoading(t *testing.T) {
	const url = "https://example.com/manga/test"
	c, _, _ := newTestController(t, false, nil, chapters(1, 1)...)
	closed := 0
	c.closeBrowser = func() { closed++ }
	if err := c.BeginLoad(url); err != nil {
		t.Fatal(err)
	}
	if err := c.Clear(); err != errBusy {
		t.Fatalf("Clear while loading = %v; want errBusy", err)
	}
	if closed != 0 {
		t.Error("a refused Clear closed the browser in use")
	}
	if err := c.FinishLoad(url); err != nil {
		t.Fatal(err)
	}
}

func TestStartValidatesBeforeDownloading(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	if err := c.Start(); err == nil {
		t.Error("Start without a series succeeded")
	}

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err == nil {
		t.Error("Start without a selection succeeded")
	}

	c.SetAll(true)
	if err := c.Configure(func(s *grabber.Settings) { s.Format = "pdf" }); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err == nil || !strings.Contains(err.Error(), "pdf") {
		t.Errorf("Start with a bad format = %v, want a format error", err)
	}
	if c.Running() {
		t.Error("a refused Start left the controller running")
	}
}

func TestDownloadNonBundle(t *testing.T) {
	c, _, dir := newTestController(t, false, nil, chapters(3, 2)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	s := waitFinished(t, c)

	if s.Saved != 3 || s.Cancelled || s.Err != nil || len(s.Failures) != 0 {
		t.Fatalf("summary = %+v", s)
	}
	for _, j := range c.Jobs() {
		if j.State != JobDone || j.Done != j.Total || j.Total != 2*(core.DownloadWeight+core.ArchiveWeight) {
			t.Errorf("job %q = %+v, want done with 2 pages' steps", j.Title, j)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.cbz"))
	if len(files) != 3 {
		t.Errorf("wrote %d cbz files, want 3 (in %s)", len(files), dir)
	}
}

// advance keeps a row downloading until its first archived step, the CLI's
// switch point: the downloads fill it to 80% and no further
func TestAdvanceSwitchesToArchivingAtDownloadShare(t *testing.T) {
	const pages = 10
	j := &Job{Total: pages * (core.DownloadWeight + core.ArchiveWeight), State: JobDownloading}

	j.Done = pages * core.DownloadWeight
	advance(j)
	if j.State != JobDownloading {
		t.Errorf("state with every page downloaded = %v, want downloading", j.State)
	}
	j.Done += core.ArchiveWeight
	advance(j)
	if j.State != JobArchiving {
		t.Errorf("state after the first archived page = %v, want archiving", j.State)
	}
}

func TestDownloadBundleWithFailure(t *testing.T) {
	chaps := chapters(3, 2)
	chaps[1].fail = map[int]bool{1: true}
	c, _, dir := newTestController(t, true, nil, chaps...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	s := waitFinished(t, c)

	if s.Err != nil || s.Cancelled {
		t.Fatalf("summary = %+v", s)
	}
	if len(s.Failures) != 1 || !strings.Contains(s.Failures[0], "Chapter 2") {
		t.Errorf("failures = %v, want chapter 2", s.Failures)
	}
	if s.Saved != 2 || s.Bundle == "" {
		t.Errorf("saved %d, bundle %q; want 2 chapters and a file", s.Saved, s.Bundle)
	}
	if _, err := os.Stat(filepath.Join(dir, s.Bundle)); err != nil {
		t.Errorf("bundle file missing: %v", err)
	}
	jobs := c.Jobs()
	if len(jobs) != 1 || jobs[0].State != JobDone || jobs[0].Done != jobs[0].Total {
		t.Errorf("bundle job = %+v, want one done row", jobs)
	}
}

func TestSkipPagesSkipsWholeChapter(t *testing.T) {
	c, _, dir := newTestController(t, false, nil, chapters(2, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	// one page per chapter, so skipping page 1 excludes all of them
	if err := c.Configure(func(s *grabber.Settings) { s.SkipPages = "1" }); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	s := waitFinished(t, c)
	if s.Skipped != 2 || s.Saved != 0 {
		t.Errorf("summary = %+v, want 2 skipped", s)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.cbz"))
	if len(files) != 0 {
		t.Errorf("skipped chapters still wrote %v", files)
	}
}

func TestCancelStartsNoNewChapters(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(4, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Configure(func(s *grabber.Settings) { s.MaxConcurrency.Chapters = 1 }); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	// the first chapter is blocked in FetchChapter once the loop has started
	// it, so cancelling then leaves the other three untouched
	<-site.entered
	c.Cancel()
	close(gate)

	s := waitFinished(t, c)
	if !s.Cancelled || s.Saved != 1 {
		t.Errorf("summary = %+v, want cancelled with 1 saved", s)
	}
}

func TestConfigureRefusedWhileBusy(t *testing.T) {
	gate := make(chan struct{})
	c, _, _ := newTestController(t, false, gate, chapters(1, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(func(*grabber.Settings) {}); err != errBusy {
		t.Errorf("Configure while running = %v, want errBusy", err)
	}
	close(gate)
	waitFinished(t, c)
}

func TestSelectionRefusedWhileBusy(t *testing.T) {
	gate := make(chan struct{})
	c, _, _ := newTestController(t, false, gate, chapters(3, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetSelected(0, true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	before := c.SelectedIndices()

	check := func(state string) {
		t.Helper()
		if err := c.SetSelected(1, true); err != errBusy {
			t.Errorf("SetSelected while %s = %v, want errBusy", state, err)
		}
		if err := c.SetAll(false); err != errBusy {
			t.Errorf("SetAll while %s = %v, want errBusy", state, err)
		}
		if _, err := c.SelectRange("1-3"); err != errBusy {
			t.Errorf("SelectRange while %s = %v, want errBusy", state, err)
		}
		if err := c.SetSelectedMany([]int{2}, true); err != errBusy {
			t.Errorf("SetSelectedMany while %s = %v, want errBusy", state, err)
		}
		if got := c.SelectedIndices(); fmt.Sprint(got) != fmt.Sprint(before) {
			t.Errorf("selection changed while %s: %v, was %v", state, got, before)
		}
	}
	check("running")
	if _, err := c.SelectRange("nope"); err != errBusy {
		t.Errorf("a bad range while running = %v, want errBusy", err)
	}
	close(gate)
	waitFinished(t, c)

	if err := c.BeginLoad("https://example.com/manga/other"); err != nil {
		t.Fatal(err)
	}
	check("loading")
	if _, err := c.SelectRange("nope"); err != errBusy {
		t.Errorf("a bad range while loading = %v, want errBusy", err)
	}
	c.abortLoad(fmt.Errorf("cancelled"))
}

func TestStaleAfterFilterChange(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(1, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if c.Stale() {
		t.Fatal("freshly loaded series is stale")
	}
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "fr" }); err != nil {
		t.Fatal(err)
	}
	if !c.Stale() {
		t.Error("language change not reported as stale")
	}
	c.SetAll(true)
	if err := c.Start(); err == nil {
		t.Error("Start with a stale list succeeded")
	}
}

func TestNotStaleWhileReloadingAfterFilterChange(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(1, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(func(s *grabber.Settings) { s.Language = "fr" }); err != nil {
		t.Fatal(err)
	}
	if err := c.BeginLoad("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if c.Stale() {
		t.Error("a reload started after a filter change is reported as stale while it loads")
	}
	if err := c.FinishLoad("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if c.Stale() {
		t.Error("the list is stale after the reload applied the filter")
	}
}

func TestLoadUnrecognisedSite(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(1, 1)...)
	c.identify = func(string, *grabber.Settings) (grabber.Site, []error) { return nil, nil }
	if err := c.Load("https://nowhere.example"); err == nil || c.Loaded() {
		t.Errorf("Load of an unknown site = %v, loaded %v", err, c.Loaded())
	}
}

func TestSetSelectedManyIsAllOrNothing(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}

	if err := c.SetSelectedMany([]int{0, 2, 9}, true); err == nil {
		t.Fatal("SetSelectedMany accepted an index out of range")
	}
	if err := c.SetSelectedMany([]int{-1}, true); err == nil {
		t.Fatal("SetSelectedMany accepted a negative index")
	}
	if c.SelectedCount() != 0 {
		t.Fatalf("a rejected call changed the selection: %d selected", c.SelectedCount())
	}

	if err := c.SetSelectedMany([]int{0, 2, 4}, true); err != nil {
		t.Fatal(err)
	}
	if c.SelectedCount() != 3 || !c.IsSelected(2) || c.IsSelected(1) {
		t.Errorf("selection after SetSelectedMany is wrong: %d selected", c.SelectedCount())
	}
	if err := c.SetSelectedMany([]int{2, 4}, false); err != nil {
		t.Fatal(err)
	}
	if got := c.SelectedIndices(); len(got) != 1 || got[0] != 0 {
		t.Errorf("deselecting left %v", got)
	}
}

func TestSetSelectedManyNotifiesOnce(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(5, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	pushes := 0
	c.notify = func() { pushes++ }

	if err := c.SetSelectedMany([]int{0, 1, 2, 3}, true); err != nil {
		t.Fatal(err)
	}
	if pushes != 1 {
		t.Errorf("SetSelectedMany notified %d times, want 1", pushes)
	}
}

func TestSetSelectedManyRefusedWhileBusy(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(3, 1)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}

	c.mu.Lock()
	c.loading = true
	c.mu.Unlock()
	if err := c.SetSelectedMany([]int{0}, true); err != errBusy {
		t.Errorf("SetSelectedMany while loading = %v, want errBusy", err)
	}
	c.mu.Lock()
	c.loading = false
	c.running = true
	c.mu.Unlock()
	if err := c.SetSelectedMany([]int{0}, true); err != errBusy {
		t.Errorf("SetSelectedMany while running = %v, want errBusy", err)
	}
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	if c.SelectedCount() != 0 {
		t.Errorf("refused calls changed the selection: %d selected", c.SelectedCount())
	}
}

// TestEventsNameTheReleaseWhenCopiesNeedAGroup checks that the job row and the
// log name the group only when the loaded list has copies that need it
func TestEventsNameTheReleaseWhenCopiesNeedAGroup(t *testing.T) {
	failed := errors.New("boom")
	tests := []struct {
		name       string
		groupDupes bool
		wantTags   string
		wantLabel  string
	}{
		{"no copies", false, "", "Chapter 5"},
		{"group copies", true, " [Alpha]", "Chapter 5 [Alpha]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewController(&grabber.Settings{}, nil)
			c.hasGroupDuplicates = tt.groupDupes
			// the maps a run starts with, which the events write into
			c.byChap = map[*grabber.Chapter]*Job{}
			c.failedC = map[*grabber.Chapter]bool{}
			ch := &grabber.Chapter{Title: "Chapter 5", Number: 5, Language: "en", Group: "Alpha", PagesCount: 1}
			e := events{c}
			e.Started(ch)
			e.DownloadFailed(ch, failed)

			jobs := c.Jobs()
			if len(jobs) != 1 || jobs[0].Title != "Chapter 5" || jobs[0].Tags != tt.wantTags {
				t.Fatalf("jobs = %+v, want one titled \"Chapter 5\" with tags %q", jobs, tt.wantTags)
			}
			// the log keeps the whole label
			log := c.Log()
			if got, want := log[len(log)-1].Text, "error downloading "+tt.wantLabel+": boom"; got != want {
				t.Errorf("log = %q, want %q", got, want)
			}
		})
	}
}
