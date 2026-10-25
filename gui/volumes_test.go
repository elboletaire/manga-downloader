// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// volumeChapters returns chapters 1..n of the given pages, in volumes of per
// chapters each (0 leaves them without one)
func volumeChapters(n, pages, per int) []fakeChapter {
	out := chapters(n, pages)
	if per > 0 {
		for i := range out {
			out[i].volume = i/per + 1
		}
	}
	return out
}

func written(t *testing.T, dir string) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.cbz"))
	out := []string{}
	for _, f := range files {
		out = append(out, filepath.Base(f))
	}
	sort.Strings(out)
	return out
}

// Each load starts the box empty (auto), with the site's volumes offered for editing
func TestLoadOffersSiteVolumes(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, volumeChapters(5, 1, 2)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if text, site, listed := c.Volumes(); text != "" || site != "1-2/3-4/5" || listed != 3 {
		t.Errorf("Volumes() = %q, %q, %d; want an empty box, the site's 1-2/3-4/5 and 3 volumes", text, site, listed)
	}

	if err := c.SetVolumes("1-3/3-5"); err == nil {
		t.Error("overlapping volumes were accepted")
	}
	if err := c.SetVolumes(" 1-3/4 "); err != nil {
		t.Fatal(err)
	}
	if text, _, _ := c.Volumes(); text != "1-3/4" {
		t.Errorf("box = %q, want it trimmed", text)
	}
	if err := c.SetVolumes("parts"); err == nil {
		t.Error("parts was accepted in the Volumes box")
	}

	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if text, site, listed := c.Volumes(); text != "" || site != "" || listed != 0 {
		t.Errorf("Clear kept the volumes %q, %q, %d", text, site, listed)
	}
}

// A listing whose first volume isn't 1 (one language's chapters only) names
// it, so the offered ranges don't start with a run of empty groups
func TestLoadNamedSiteVolume(t *testing.T) {
	list := volumeChapters(5, 1, 0)
	for i := range list {
		list[i].volume = 16
	}
	list[4].volume = 18
	c, _, _ := newTestController(t, false, nil, list...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if text, site, _ := c.Volumes(); site != "16:1-4/18:5" || text != "" {
		t.Errorf("Volumes() = %q, %q; want an empty box and 16:1-4/18:5", text, site)
	}
	if err := c.SetVolumes("16: 1-2/3-4"); err != nil {
		t.Errorf("named volumes were refused: %v", err)
	}
	if err := c.SetVolumes("16: 1-2/16: 3"); err == nil {
		t.Error("a volume given twice was accepted")
	}
}

// Chapter numbers repeated across volumes (an anthology restarting its
// numbering) can't be written as ranges, so no site volumes are offered, but
// the site still lists its volumes and a run packs them as the site files them
func TestLoadSiteVolumesWithoutRanges(t *testing.T) {
	list := []fakeChapter{
		{number: 1, pages: 1, volume: 1},
		{number: 1, pages: 1, volume: 2},
		{number: 2, pages: 1, volume: 1},
		{number: 2, pages: 1, volume: 2},
	}
	c, _, dir := newTestController(t, false, nil, list...)
	c.Configure(func(s *grabber.Settings) { applyPackAs(s, PackVolumes) })
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if text, site, listed := c.Volumes(); text != "" || site != "" || listed != 2 {
		t.Errorf("Volumes() = %q, %q, %d; want an empty box, no ranges and 2 volumes", text, site, listed)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if r := c.Run(); !r.Volumes || r.Rows != 2 {
		t.Errorf("run = %+v, want 2 volume rows", r)
	}
	if s := waitFinished(t, c); s.Volumes != 2 || s.Saved != 4 {
		t.Errorf("summary = %+v, want 2 volume files of 4 chapters", s)
	}
	if files := written(t, dir); len(files) != 2 {
		t.Errorf("wrote %q, want 2 volume files", files)
	}
}

func TestDownloadVolumes(t *testing.T) {
	c, _, dir := newTestController(t, false, nil, volumeChapters(5, 2, 0)...)
	if err := c.Configure(func(s *grabber.Settings) { applyPackAs(s, PackVolumes) }); err != nil {
		t.Fatal(err)
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetVolumes("1-2/3-4"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if r := c.Run(); !r.Volumes || r.Rows != 3 || r.Chapters != 5 {
		t.Errorf("run = %+v, want volumes over 5 chapters in 3 rows", r)
	}
	s := waitFinished(t, c)

	if s.Saved != 5 || s.Volumes != 2 || len(s.Failures) != 0 || s.Err != nil {
		t.Fatalf("summary = %+v", s)
	}
	var titles []string
	for _, j := range c.Jobs() {
		titles = append(titles, j.Title)
		if j.State != JobDone || j.Done != j.Total {
			t.Errorf("job %q = %+v, want done", j.Title, j)
		}
	}
	want := []string{"Vol. 01 · chapters 1–2", "Vol. 02 · chapters 3–4", "Chapter 5"}
	if !reflect.DeepEqual(titles, want) {
		t.Errorf("jobs %q, want %q", titles, want)
	}
	if j := c.Jobs()[0]; !j.Volume || j.Total != volumeScale {
		t.Errorf("volume job = %+v, want a volume row out of volumeScale", j)
	}
	// the test template is a custom one, so it names the volumes too
	files := written(t, dir)
	if wantFiles := []string{"Test Series 1-2 - Vol. 01.cbz", "Test Series 3-4 - Vol. 02.cbz", "Test Series 5 - Chapter 5.cbz"}; !reflect.DeepEqual(files, wantFiles) {
		t.Errorf("wrote %q, want %q", files, wantFiles)
	}
	// the run's volumes aren't an option: the choice is still saved as volumes
	if o := optionsOf(c.Settings()); o.PackAs != PackVolumes {
		t.Errorf("pack-as after the run = %q", o.PackAs)
	}
}

func TestDownloadVolumesWithFailingChapter(t *testing.T) {
	chaps := volumeChapters(4, 1, 2)
	chaps[0].fail = map[int]bool{0: true}
	chaps[2].fail = map[int]bool{0: true}
	chaps[3].fail = map[int]bool{0: true}
	c, _, dir := newTestController(t, false, nil, chaps...)
	c.Configure(func(s *grabber.Settings) { applyPackAs(s, PackVolumes) })
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	s := waitFinished(t, c)

	// volume 1 keeps chapter 2; volume 2 lost both of its chapters
	if s.Volumes != 1 || s.Saved != 1 {
		t.Errorf("summary = %+v, want 1 volume of 1 chapter", s)
	}
	jobs := c.Jobs()
	if len(jobs) != 2 || jobs[0].State != JobDone || jobs[1].State != JobFailed {
		t.Errorf("jobs = %+v, want volume 1 done and volume 2 failed", jobs)
	}
	if len(s.Failures) != 4 {
		t.Errorf("failures = %+v, want the 3 chapters and the empty volume", s.Failures)
	}
	if files := written(t, dir); !reflect.DeepEqual(files, []string{"Test Series 2 - Vol. 01.cbz"}) {
		t.Errorf("wrote %q", files)
	}
}

func TestStartVolumesWithoutSiteVolumes(t *testing.T) {
	c, _, dir := newTestController(t, false, nil, chapters(2, 1)...)
	c.Configure(func(s *grabber.Settings) { applyPackAs(s, PackVolumes) })
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if text, site, listed := c.Volumes(); text != "" || site != "" || listed != 0 {
		t.Fatalf("volumes %q, %q, %d for a site without any", text, site, listed)
	}
	c.SetAll(true)
	if err := c.Start(); !errors.Is(err, core.ErrNoVolumes) {
		t.Fatalf("Start() = %v, want ErrNoVolumes", err)
	}
	if c.Running() {
		t.Error("a refused Start left the controller running")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("a refused run wrote %d files", len(entries))
	}
}

// TestVolumeProgressNeverGoesBack runs volumes of uneven chapters
// concurrently and checks, after every event, that no row and not the run as
// a whole (as the window works them out, see run.ts) ever moves backwards. A
// volume's total used to grow as each of its chapters started, pulling its
// row and the overall bar back every time.
func TestVolumeProgressNeverGoesBack(t *testing.T) {
	chaps := []fakeChapter{
		{number: 1, pages: 9, volume: 1},
		{number: 2, pages: 1, volume: 1},
		{number: 3, pages: 6, volume: 1},
		{number: 4, pages: 2, volume: 2},
		{number: 5, pages: 8, volume: 2, fail: map[int]bool{7: true}},
		{number: 6, pages: 3},
	}
	c, _, _ := newTestController(t, false, nil, chaps...)
	c.Configure(func(s *grabber.Settings) { applyPackAs(s, PackVolumes) })
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)

	// each snapshot is taken and checked under one lock, so they're checked
	// in the order the controller's state went through them
	var mu sync.Mutex
	rowLast := map[string]float64{}
	overallLast := 0.0
	snapshots := 0
	c.notify = func() {
		mu.Lock()
		defer mu.Unlock()
		rows := c.Run().Rows
		if rows == 0 {
			return
		}
		snapshots++
		sum := 0.0
		for _, j := range c.Jobs() {
			f := jobFraction(j)
			if f < rowLast[j.Title] {
				t.Errorf("row %q went back from %.3f to %.3f", j.Title, rowLast[j.Title], f)
			}
			rowLast[j.Title] = f
			sum += f
		}
		overall := sum / float64(rows)
		if overall < overallLast {
			t.Errorf("overall progress went back from %.3f to %.3f", overallLast, overall)
		}
		overallLast = overall
	}

	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)
	mu.Lock()
	defer mu.Unlock()
	if snapshots < 20 || overallLast != 1 {
		t.Errorf("%d snapshots ending at %.3f, want the run seen through to 1", snapshots, overallLast)
	}
}

// jobFraction is the window's jobFraction (format.ts), a finished row being
// complete whatever its count
func jobFraction(j Job) float64 {
	switch {
	case j.State == JobDone || j.State == JobFailed || j.State == JobSkipped:
		return 1
	case j.Total <= 0:
		return 0
	}
	return min(1, max(0, float64(j.Done)/float64(j.Total)))
}

// a parts run's rows are its merged chapters and the chapters on their own,
// as core groups them
func TestDownloadPartsRows(t *testing.T) {
	c, _, _ := newTestController(t, false, nil,
		fakeChapter{number: 1, pages: 1},
		fakeChapter{number: 2, pages: 1},
		fakeChapter{number: 2, pages: 2},
		fakeChapter{number: 3, pages: 1},
	)
	if err := c.Configure(func(s *grabber.Settings) { applyPackAs(s, PackParts) }); err != nil {
		t.Fatal(err)
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if r := c.Run(); !r.Parts || r.Rows != 3 || r.Chapters != 4 {
		t.Errorf("run = %+v, want a parts run of 4 chapters in 3 rows", r)
	}
	waitFinished(t, c)
}

// a chapter's merged parts are named as the chapter, not as a volume
func TestVolumeTitleParts(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	v := &core.Volume{Number: 2, Chapters: grabber.Filterables{
		&grabber.Chapter{Number: 2, Title: "Chapter 2 Part 1"},
		&grabber.Chapter{Number: 2, Title: "Chapter 2 Part 2"},
	}}
	if got := c.volumeTitle(v).Text; got != "Vol. 02 · chapter 2" {
		t.Errorf("volume title = %q", got)
	}
	c.runinfo.Parts = true
	v.Parts = true
	if got := c.volumeTitle(v).Text; got != "Chapter 2 · 2 parts" {
		t.Errorf("parts title = %q", got)
	}
	if got := v.Label(); got != "Chapter 2" {
		t.Errorf("parts label = %q", got)
	}
}
