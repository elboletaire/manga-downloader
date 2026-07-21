package packer

import (
	"archive/zip"
	"path/filepath"
	"testing"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/spf13/cobra"
)

// entryNames opens the zip at path and returns its entry names, in order.
func entryNames(t *testing.T, path string) []string {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening archive: %v", err)
	}
	defer r.Close()

	names := make([]string, len(r.File))
	for i, f := range r.File {
		names[i] = f.Name
	}
	return names
}

func TestArchiveCBZNoFiles(t *testing.T) {
	err := ArchiveCBZ(filepath.Join(t.TempDir(), "empty.cbz"), []File{}, func(page, progress int) {})
	if err == nil {
		t.Fatal("expected an error when archiving zero files, got nil")
	}
}

func TestArchiveCBZEntryNames(t *testing.T) {
	// Simulate a bundle: two chapters, each with its own folder and page
	// numbering restarting at 000, per issue #47.
	files := []File{
		{Name: "Chapter 0001/000.jpg", Data: []byte("ch1-p0")},
		{Name: "Chapter 0001/001.jpg", Data: []byte("ch1-p1")},
		{Name: "Chapter 0002/000.jpg", Data: []byte("ch2-p0")},
	}

	path := filepath.Join(t.TempDir(), "bundle.cbz")
	if err := ArchiveCBZ(path, files, func(page, progress int) {}); err != nil {
		t.Fatalf("ArchiveCBZ: %v", err)
	}

	names := entryNames(t, path)
	want := []string{"Chapter 0001/000.jpg", "Chapter 0001/001.jpg", "Chapter 0002/000.jpg"}
	if len(names) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(names), len(want), names)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("entry %d = %q, want %q", i, n, want[i])
		}
	}

	// Both chapters must independently contain a 000.jpg (no global renumbering).
	seen000 := 0
	for _, n := range names {
		if filepath.Base(n) == "000.jpg" {
			seen000++
		}
	}
	if seen000 != 2 {
		t.Errorf("expected 2 entries named 000.jpg (one per chapter folder), got %d", seen000)
	}
}

func TestPaddedChapterNumber(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1, "0001"},
		{10, "0010"},
		{10.5, "0010.5"},
		{186, "0186"},
		{1186, "1186"},
		{1186.5, "1186.5"},
		{0, "0000"},
	}

	for _, c := range cases {
		if got := paddedChapterNumber(c.in); got != c.want {
			t.Errorf("paddedChapterNumber(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// fakeSite is a minimal grabber.Site implementation for testing PackBundle's
// entry-naming behavior without hitting a real site.
type fakeSite struct {
	title    string
	template string
}

func (f *fakeSite) InitFlags(cmd *cobra.Command)                  {}
func (f *fakeSite) Test() (bool, error)                           { return true, nil }
func (f *fakeSite) FetchChapters() (grabber.Filterables, []error) { return nil, nil }
func (f *fakeSite) FetchChapter(grabber.Filterable) (*grabber.Chapter, error) {
	return nil, nil
}
func (f *fakeSite) FetchTitle() (string, error) { return f.title, nil }
func (f *fakeSite) BaseUrl() string             { return "https://example.com" }
func (f *fakeSite) GetFilenameTemplate() string { return f.template }
func (f *fakeSite) GetFormat() string           { return FormatCBZ }
func (f *fakeSite) GetMaxConcurrency() grabber.MaxConcurrency {
	return grabber.MaxConcurrency{Chapters: 1, Pages: 1}
}
func (f *fakeSite) GetPreferredLanguage() string { return "" }
func (f *fakeSite) GetRetries() uint8            { return 0 }

func TestPackBundleEntryNames(t *testing.T) {
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault}

	chapters := []*DownloadedChapter{
		{
			Chapter: &grabber.Chapter{Number: 1},
			Files: []*downloader.File{
				{Data: []byte("ch1-p0")},
				{Data: []byte("ch1-p1")},
			},
		},
		{
			Chapter: &grabber.Chapter{Number: 10.5},
			Files: []*downloader.File{
				{Data: []byte("ch10.5-p0")},
			},
		},
	}

	dir := t.TempDir()
	filename, err := PackBundle(dir, site, chapters, "1-10.5", func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}

	names := entryNames(t, filepath.Join(dir, filename))
	want := []string{
		"Chapter 0001/000.jpg",
		"Chapter 0001/001.jpg",
		"Chapter 0010.5/000.jpg",
	}
	if len(names) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(names), len(want), names)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("entry %d = %q, want %q", i, n, want[i])
		}
	}
}
