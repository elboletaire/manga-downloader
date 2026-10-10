// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// newTestView builds the window over a controller on a fake series
func newTestView(t *testing.T, site fakeSite) (*view, *Controller, *fakeSite) {
	t.Helper()
	test.NewApp()
	a := fyne.CurrentApp()
	w := a.NewWindow("test")
	t.Cleanup(w.Close)

	c, fs, _ := newTestController(t, false, nil, site.chapters...)
	fs.pageGate, fs.pageGated = site.pageGate, site.pageGated
	v := newView(a, w, c, nil)
	w.SetContent(v.content)
	w.Resize(fyne.NewSize(960, 720))
	return v, c, fs
}

// snapshot writes the window as the software renderer draws it, when the
// GUI_SCREENSHOT_DIR environment variable names a directory
func snapshot(t *testing.T, v *view, name string) {
	t.Helper()
	dir := os.Getenv("GUI_SCREENSHOT_DIR")
	if dir == "" {
		return
	}
	c := software.NewCanvas()
	c.SetContent(v.content)
	c.Resize(fyne.NewSize(960, 720))
	v.refresh()
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, c.Capture()); err != nil {
		t.Fatal(err)
	}
}

// waitFor polls cond until it's true
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func thirtyChapters() []fakeChapter {
	out := chapters(30, 3)
	out[6].fail = map[int]bool{0: true} // chapter 7 fails its first page
	return out
}

// TestViewScreens walks the window through the states worth reviewing, and
// writes a screenshot of each when GUI_SCREENSHOT_DIR is set
func TestViewScreens(t *testing.T) {
	v, c, fs := newTestView(t, fakeSite{chapters: thirtyChapters()})
	if c.Loaded() || v.title.Text != noSeries {
		t.Fatalf("fresh window shows %q", v.title.Text)
	}
	snapshot(t, v, "01-empty.png")

	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SelectRange("1-12"); err != nil {
		t.Fatal(err)
	}
	v.refresh()
	if v.count.Text != "12 of 30 selected" {
		t.Errorf("count = %q", v.count.Text)
	}
	snapshot(t, v, "02-loaded.png")

	// chapter 5's pages are held back, so the run stops with it in progress
	fs.pageGate = make(chan struct{})
	fs.pageGated = 5
	if err := c.Configure(func(s *grabber.Settings) { s.MaxConcurrency.Chapters = 3 }); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the other chapters to settle", func() bool {
		n := 0
		for _, j := range c.Jobs() {
			if j.State == JobDone || j.State == JobFailed {
				n++
			}
		}
		return n == 11
	})
	v.refresh()
	snapshot(t, v, "03-progress.png")
	close(fs.pageGate)

	s := waitFinished(t, c)
	v.refresh()
	if s.Saved != 11 || len(s.Failures) != 1 {
		t.Fatalf("summary = %+v", s)
	}
	snapshot(t, v, "04-finished.png")

	v.options.Open(0)
	snapshot(t, v, "05-options.png")
}

func TestLoadMessage(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{&core.TitleError{Err: errors.New("boom")}, "Couldn't fetch the series title: boom"},
		{&core.ChaptersError{Errs: []error{errors.New("a"), errors.New("b")}}, "Couldn't fetch the chapter list: error fetching chapters: a; b"},
		{&core.NoChaptersError{Language: "fr"}, core.NoChaptersMessage("fr", "")},
		{errors.New("site not recognised"), "site not recognised"},
	}
	for _, tt := range tests {
		if got := loadMessage(tt.err); got != tt.want {
			t.Errorf("loadMessage(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestSummaryText(t *testing.T) {
	tests := []struct {
		name string
		s    Summary
		want []string
	}{
		{"saved", Summary{Dir: "/d", Saved: 2}, []string{"Saved 2 chapters to /d."}},
		{"single", Summary{Dir: "/d", Saved: 1}, []string{"Saved 1 chapter to /d."}},
		{"bundle", Summary{Dir: "/d", Bundled: true, Bundle: "b.cbz", Saved: 3}, []string{"Saved b.cbz to /d (3 chapters)."}},
		{"bundle failed", Summary{Bundled: true, Err: errors.New("x")}, []string{"The bundle could not be written."}},
		{"cancelled bundle", Summary{Bundled: true, Cancelled: true, Saved: 2}, []string{"Cancelled. No bundle was written."}},
		{"cancelled", Summary{Dir: "/d", Cancelled: true, Saved: 1}, []string{"Cancelled. 1 chapter saved before it stopped, in /d."}},
		{"nothing", Summary{}, []string{"Nothing was saved."}},
		{"failures", Summary{Dir: "/d", Saved: 1, Skipped: 1, Failures: []string{"Chapter 2: boom"}},
			[]string{"Saved 1 chapter to /d.", "1 chapter skipped: --skip-pages excluded every page.", "1 failed:", "  - Chapter 2: boom"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summaryText(&tt.s)
			for _, line := range tt.want {
				if !strings.Contains(got, line) {
					t.Errorf("summaryText() = %q, missing %q", got, line)
				}
			}
		})
	}
}

func TestJobStatus(t *testing.T) {
	tests := []struct {
		job  Job
		want string
	}{
		{Job{State: JobDownloading}, "downloading"},
		{Job{State: JobArchiving}, "archiving"},
		{Job{State: JobDownloading, Status: "Downloading Chapter 3"}, "Downloading Chapter 3"},
		{Job{State: JobDone}, "done"},
		{Job{State: JobFailed, Reason: "packing: disk full"}, "failed: packing: disk full"},
		{Job{State: JobSkipped, Reason: "every page excluded by --skip-pages"}, "skipped: every page excluded by --skip-pages"},
	}
	for _, tt := range tests {
		if got := jobStatus(tt.job); got != tt.want {
			t.Errorf("jobStatus(%+v) = %q, want %q", tt.job, got, tt.want)
		}
	}
}

func TestOptionsRoundTrip(t *testing.T) {
	test.NewApp()
	p := fyne.CurrentApp().Preferences()
	want := *defaultSettings()
	want.OutputDir = "/books"
	want = grabber.Settings{
		MaxConcurrency:   want.MaxConcurrency,
		Retry:            want.Retry,
		OutputDir:        "/books",
		Format:           "raw",
		Bundle:           true,
		Language:         "es-la",
		Scanlator:        "all",
		ConvertImages:    "avif,webp",
		SkipPages:        "2,-1",
		FilenameTemplate: "{{.Series}} {{.Number}}",
		BrowserVisible:   true,
	}
	saveOptions(p, want)

	got := defaultSettings()
	loadOptions(p, got)
	if *got != want {
		t.Errorf("loaded %+v, want %+v", *got, want)
	}
}

func TestDefaultOutputDirPrefersDownloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := defaultOutputDir(); got != home {
		t.Errorf("without Downloads: %q, want %q", got, home)
	}
	downloads := filepath.Join(home, "Downloads")
	if err := os.Mkdir(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := defaultOutputDir(); got != downloads {
		t.Errorf("with Downloads: %q, want %q", got, downloads)
	}
}
