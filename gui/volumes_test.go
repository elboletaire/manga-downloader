// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

func TestLoadPrefillsSiteVolumes(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, volumeChapters(5, 1, 2)...)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	text, site := c.Volumes()
	if site != "1-2; 3-4; 5" || text != site {
		t.Errorf("Volumes() = %q, %q; want the site's 1-2; 3-4; 5 in both", text, site)
	}

	if err := c.SetVolumes("1-3;3-5"); err == nil {
		t.Error("overlapping volumes were accepted")
	}
	if err := c.SetVolumes(" 1-3; 4 "); err != nil {
		t.Fatal(err)
	}
	if text, _ := c.Volumes(); text != "1-3; 4" {
		t.Errorf("box = %q, want it trimmed", text)
	}

	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if text, site := c.Volumes(); text != "" || site != "" {
		t.Errorf("Clear kept the volumes %q, %q", text, site)
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
	if err := c.SetVolumes("1-2;3-4"); err != nil {
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
	if j := c.Jobs()[0]; !j.Volume || j.Total != 8 {
		t.Errorf("volume job = %+v, want a volume row of 2 chapters × 2 pages × 2", j)
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
		t.Errorf("failures = %q, want the 3 chapters and the empty volume", s.Failures)
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
	if text, site := c.Volumes(); text != "" || site != "" {
		t.Fatalf("volumes %q, %q for a site without any", text, site)
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
