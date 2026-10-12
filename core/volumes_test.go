// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// chapters builds a sorted listing out of number/volume pairs
func chapters(pairs ...float64) grabber.Filterables {
	var out grabber.Filterables
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, &grabber.Chapter{Number: pairs[i], Volume: int(pairs[i+1])})
	}
	return out
}

func numbers(f grabber.Filterables) []float64 {
	out := []float64{}
	for _, c := range f {
		out = append(out, c.GetNumber())
	}
	return out
}

func TestParseVolumesSpec(t *testing.T) {
	if spec, err := ParseVolumes(" "); spec != nil || err != nil {
		t.Errorf("empty: got %v, %v; want nil, nil", spec, err)
	}
	if spec, err := ParseVolumes("AUTO"); err != nil || !spec.Auto {
		t.Errorf("auto: got %+v, %v", spec, err)
	}
	if spec, err := ParseVolumes("1-7;8"); err != nil || len(spec.Groups) != 2 {
		t.Errorf("groups: got %+v, %v", spec, err)
	}
	if _, err := ParseVolumes("1-7;5"); err == nil {
		t.Error("overlap: want an error")
	}
}

func TestGroupVolumes(t *testing.T) {
	list := chapters(1, 1, 2, 1, 2.5, 0, 3, 2, 4, 2, 5, 0, 6, 4)
	tests := []struct {
		name    string
		spec    string
		want    map[int][]float64
		loose   []float64
		wantErr error
	}{
		{"auto", "auto", map[int][]float64{1: {1, 2}, 2: {3, 4}, 4: {6}}, []float64{2.5, 5}, nil},
		{"explicit", "1-2;3-4", map[int][]float64{1: {1, 2}, 2: {3, 4}}, []float64{2.5, 5, 6}, nil},
		{"decimals and commas", "1-2.5,6;3", map[int][]float64{1: {1, 2, 2.5, 6}, 2: {3}}, []float64{4, 5}, nil},
		{"empty group keeps numbering", "1-2;;3-6", map[int][]float64{1: {1, 2}, 3: {3, 4, 5, 6}}, []float64{2.5}, nil},
		{"group matching nothing", "1-2;100-200", map[int][]float64{1: {1, 2}}, []float64{2.5, 3, 4, 5, 6}, nil},
	}
	for _, tt := range tests {
		spec, err := ParseVolumes(tt.spec)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		vols, loose, err := GroupVolumes(list, spec)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		got := map[int][]float64{}
		var order []int
		for _, v := range vols {
			got[v.Number] = numbers(v.Chapters)
			order = append(order, v.Number)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: volumes = %v, want %v", tt.name, got, tt.want)
		}
		if !sort.IntsAreSorted(order) {
			t.Errorf("%s: volumes out of order: %v", tt.name, order)
		}
		if !reflect.DeepEqual(numbers(loose), tt.loose) {
			t.Errorf("%s: loose = %v, want %v", tt.name, numbers(loose), tt.loose)
		}
	}

	if _, _, err := GroupVolumes(chapters(1, 0, 2, 0), &VolumeSpec{Auto: true}); !errors.Is(err, ErrNoVolumes) {
		t.Errorf("auto without volumes: got %v, want ErrNoVolumes", err)
	}
}

func TestSiteVolumes(t *testing.T) {
	tests := []struct {
		name string
		list grabber.Filterables
		want string
	}{
		{"none", chapters(1, 0, 2, 0), ""},
		{"contiguous", chapters(1, 1, 2, 1, 3, 2, 4, 2), "1-2; 3-4"},
		{"single chapter volume", chapters(1, 1, 2, 2), "1; 2"},
		{"loose chapter breaks the run", chapters(1, 1, 1.5, 0, 2, 1, 3, 2), "1,2; 3"},
		{"missing volume keeps its place", chapters(1, 1, 2, 3), "1; ; 2"},
		{"loose tail", chapters(1, 1, 2, 1, 3, 0, 4, 0), "1-2"},
		{"repeated number stays in its first volume", chapters(1, 1, 2, 1, 2, 2, 3, 2), "1-2; 3"},
	}
	for _, tt := range tests {
		got := SiteVolumes(tt.list)
		if got != tt.want {
			t.Errorf("%s: SiteVolumes = %q, want %q", tt.name, got, tt.want)
			continue
		}
		if got == "" {
			continue
		}
		// what it writes must parse back into the same grouping
		spec, err := ParseVolumes(got)
		if err != nil {
			t.Errorf("%s: %q doesn't parse: %v", tt.name, got, err)
		}
		auto, _, _ := GroupVolumes(tt.list, &VolumeSpec{Auto: true})
		explicit, _, _ := GroupVolumes(tt.list, spec)
		if len(auto) != len(explicit) {
			t.Errorf("%s: %d volumes from the site, %d from %q", tt.name, len(auto), len(explicit), got)
		}
	}
}

func TestValidateVolumes(t *testing.T) {
	tests := []struct {
		volumes string
		bundle  bool
		ok      bool
	}{
		{"", true, true},
		{"auto", false, true},
		{"1-7;8-16", false, true},
		{"1-7;6-16", false, false},
		{"1-7;x", false, false},
		{"auto", true, false},
		{"1-7", true, false},
	}
	for _, tt := range tests {
		s := &grabber.Settings{Format: packer.FormatCBZ, ConvertImages: grabber.ConvertImagesDefault, Volumes: tt.volumes, Bundle: tt.bundle}
		_, err := Validate(s)
		if (err == nil) != tt.ok {
			t.Errorf("Validate(volumes %q, bundle %v) = %v, want ok %v", tt.volumes, tt.bundle, err, tt.ok)
		}
		var se *SettingError
		if err != nil && (!errors.As(err, &se) || se.Setting != "volumes") {
			t.Errorf("Validate(volumes %q): %v is not a volumes SettingError", tt.volumes, err)
		}
	}
}

// basenames returns the sorted file names of paths
func basenames(paths []string) []string {
	out := []string{}
	for _, p := range paths {
		out = append(out, filepath.Base(p))
	}
	sort.Strings(out)
	return out
}

func TestDownloadVolumes(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 2},
		fakeChapter{number: 2, pages: 1},
		fakeChapter{number: 3, pages: 1},
		fakeChapter{number: 4, pages: 1},
		fakeChapter{number: 5, pages: 1},
	)
	series, dir := newSeries(t, site, false)
	site.Settings.Volumes = "1-2;3-4"
	rec := &recorder{}

	name, err := series.Download(context.Background(), listed(site), nil, "", rec)
	if err != nil || name != "" {
		t.Fatalf("Download = %q, %v", name, err)
	}

	// no totals loop: each chapter is fetched once, unlike a bundle's
	if site.fetches != 5 {
		t.Errorf("FetchChapter called %d times, want 5", site.fetches)
	}
	if got := rec.events[:2]; !reflect.DeepEqual(got, []string{"volume-start 1", "volume-start 2"}) {
		t.Errorf("first events %q, want both volumes announced first", got)
	}
	want := []string{
		"volume-start 1", "volume-start 2",
		"volume-chapter 1 1", "started 1", "page 1 0", "page 1 1", "downloaded 1",
		"volume-chapter 1 2", "started 2", "page 2 0", "downloaded 2",
		"volume-packing 1", "volume-archived 1 1", "volume-archived 1 1", "volume-archived 1 1", "volume-packed 1 Test Series Vol. 01.cbz",
		"volume-chapter 2 3", "started 3", "page 3 0", "downloaded 3",
		"volume-chapter 2 4", "started 4", "page 4 0", "downloaded 4",
		"volume-packing 2", "volume-archived 2 1", "volume-archived 2 1", "volume-packed 2 Test Series Vol. 02.cbz",
		"started 5", "page 5 0", "archived 5", "packed 5",
	}
	if strings.Join(rec.events, "|") != strings.Join(want, "|") {
		t.Errorf("events:\n got %q\nwant %q", rec.events, want)
	}

	files := basenames(cbzFiles(t, dir))
	if wantFiles := []string{"Test Series 5 - Chapter 5.cbz", "Test Series Vol. 01.cbz", "Test Series Vol. 02.cbz"}; !reflect.DeepEqual(files, wantFiles) {
		t.Errorf("wrote %q, want %q", files, wantFiles)
	}
	entries := zipEntries(t, filepath.Join(dir, "Test Series Vol. 01.cbz"))
	if wantEntries := []string{"Chapter 0001/000.png", "Chapter 0001/001.png", "Chapter 0002/000.png"}; !reflect.DeepEqual(entries, wantEntries) {
		t.Errorf("volume 1 holds %q, want %q", entries, wantEntries)
	}
}

func TestDownloadVolumesAuto(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 1, volume: 3},
		fakeChapter{number: 2, pages: 1, volume: 3},
		fakeChapter{number: 3, pages: 1},
	)
	series, dir := newSeries(t, site, false)
	site.Settings.Volumes = "auto"
	// the loose chapter's name gets the volume its listing has, none here
	site.Settings.FilenameTemplate = "{{.Series}} v{{.Volume}} c{{.Number}}"

	if _, err := series.Download(context.Background(), listed(site), nil, "", &recorder{}); err != nil {
		t.Fatal(err)
	}
	// a custom template names the volumes too, with the volume's chapters as .Number
	files := basenames(cbzFiles(t, dir))
	if want := []string{"Test Series v0 c3.cbz", "Test Series v3 c1-2.cbz"}; !reflect.DeepEqual(files, want) {
		t.Errorf("wrote %q, want %q", files, want)
	}
}

func TestDownloadChapterNameGetsListedVolume(t *testing.T) {
	site := newFakeSite(t, fakeChapter{number: 7, pages: 1, volume: 2})
	series, dir := newSeries(t, site, false)
	site.Settings.FilenameTemplate = `{{.Series}} v{{printf "%02d" .Volume}} c{{.Number}}`

	if _, err := series.Download(context.Background(), listed(site), nil, "", &recorder{}); err != nil {
		t.Fatal(err)
	}
	if files := basenames(cbzFiles(t, dir)); !reflect.DeepEqual(files, []string{"Test Series v02 c7.cbz"}) {
		t.Errorf("wrote %q, want the listed volume in the chapter's name", files)
	}
}

func TestDownloadVolumesAutoWithoutVolumes(t *testing.T) {
	site := newFakeSite(t, fakeChapter{number: 1, pages: 1})
	series, dir := newSeries(t, site, false)
	site.Settings.Volumes = "auto"
	rec := &recorder{}

	_, err := series.Download(context.Background(), listed(site), nil, "", rec)
	var se *SettingError
	if !errors.As(err, &se) || !errors.Is(err, ErrNoVolumes) {
		t.Fatalf("got %v, want a SettingError wrapping ErrNoVolumes", err)
	}
	if site.fetches != 0 || len(rec.events) != 0 || len(cbzFiles(t, dir)) != 0 {
		t.Errorf("failed after starting: %d fetches, events %q", site.fetches, rec.events)
	}
}

func TestDownloadVolumesBrokenTemplate(t *testing.T) {
	site := newFakeSite(t, fakeChapter{number: 1, pages: 1})
	series, _ := newSeries(t, site, false)
	site.Settings.Volumes = "1"
	site.Settings.FilenameTemplate = "{{.Nope}}"

	_, err := series.Download(context.Background(), listed(site), nil, "", &recorder{})
	var se *SettingError
	if !errors.As(err, &se) || se.Setting != "filename-template" || site.fetches != 0 {
		t.Fatalf("got %v after %d fetches, want a filename-template SettingError before any", err, site.fetches)
	}
}

func TestDownloadVolumesFailingChapters(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 1, fail: map[int]bool{0: true}},
		fakeChapter{number: 2, pages: 1},
		fakeChapter{number: 3, pages: 1, fail: map[int]bool{0: true}},
	)
	series, dir := newSeries(t, site, false)
	site.Settings.Volumes = "1-2;3"
	rec := &recorder{}

	if _, err := series.Download(context.Background(), listed(site), nil, "", rec); err != nil {
		t.Fatal(err)
	}
	// volume 1 goes on without its failed chapter; volume 2 has nothing left
	if rec.count("volume-packed 1") != 1 {
		t.Errorf("volume 1 not packed: %q", rec.events)
	}
	if rec.count("volume-failed 2 "+ErrEmptyVolume.Error()) != 1 {
		t.Errorf("volume 2 not reported empty: %q", rec.events)
	}
	if rec.count("volume-packing 2") != 0 {
		t.Errorf("empty volume 2 was packed: %q", rec.events)
	}
	if files := basenames(cbzFiles(t, dir)); !reflect.DeepEqual(files, []string{"Test Series Vol. 01.cbz"}) {
		t.Errorf("wrote %q, want volume 1 alone", files)
	}
	if entries := zipEntries(t, filepath.Join(dir, "Test Series Vol. 01.cbz")); !reflect.DeepEqual(entries, []string{"Chapter 0002/000.png"}) {
		t.Errorf("volume 1 holds %q", entries)
	}
}

func TestDownloadVolumesCancelled(t *testing.T) {
	site := newFakeSite(t, fakeChapter{number: 1, pages: 1}, fakeChapter{number: 2, pages: 1})
	series, dir := newSeries(t, site, false)
	site.Settings.Volumes = "1-2"
	rec := &recorder{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := series.Download(ctx, listed(site), nil, "", rec); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	// the volume is announced, then closed as cancelled, never packed
	want := []string{"volume-start 1", "volume-failed 1 " + context.Canceled.Error()}
	if !reflect.DeepEqual(rec.events, want) {
		t.Errorf("events %q, want %q", rec.events, want)
	}
	if len(cbzFiles(t, dir)) != 0 {
		t.Error("a cancelled volume was written")
	}
}

func TestDownloadVolumesIgnoredWhenBundling(t *testing.T) {
	// Validate refuses the pair; Download, given it anyway, bundles as asked
	site := newFakeSite(t, fakeChapter{number: 1, pages: 1})
	series, _ := newSeries(t, site, true)
	site.Settings.Volumes = "1"
	rec := &recorder{}

	if _, err := series.Download(context.Background(), listed(site), nil, "1", rec); err != nil {
		t.Fatal(err)
	}
	if rec.count("volume-") != 0 {
		t.Errorf("volume events in bundle mode: %q", rec.events)
	}
}
