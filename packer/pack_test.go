// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
)

// A template that ignores {{.Version}} renders the same name for every try,
// so pack adds the " v2" itself, rather than looping on the one name forever
func TestPackTemplateWithoutVersionAddsTheSuffix(t *testing.T) {
	const template = "{{.Series}} {{.Number}} [{{.Group}}]"
	parts := FilenameTemplateParts{Series: "X", Number: "1", Group: "Alpha"}
	files := epubBundleFiles(t)
	noProgress := func(page, progress int) {}

	tests := []struct {
		format string
		names  []string
	}{
		{FormatCBZ, []string{"X 1 [Alpha].cbz", "X 1 [Alpha] v2.cbz", "X 1 [Alpha] v3.cbz"}},
		{FormatCBR, []string{"X 1 [Alpha].cbr", "X 1 [Alpha] v2.cbr", "X 1 [Alpha] v3.cbr"}},
		{FormatEPUB, []string{"X 1 [Alpha].epub", "X 1 [Alpha] v2.epub", "X 1 [Alpha] v3.epub"}},
		{FormatRaw, []string{"X 1 [Alpha]", "X 1 [Alpha] v2", "X 1 [Alpha] v3"}},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			dir := t.TempDir()
			for _, want := range tt.names {
				got, err := pack(dir, tt.format, template, "X", parts, files, EPUBInfo{Series: "X"}, noProgress)
				if err != nil {
					t.Fatalf("pack() error = %v", err)
				}
				if got != want {
					t.Errorf("pack() = %q, want %q", got, want)
				}
				if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
					t.Errorf("expected %s on disk: %v", want, err)
				}
			}
		})
	}
}

// The default template keeps naming its duplicates with the version it renders
func TestPackDefaultTemplateNamesDuplicatesByVersion(t *testing.T) {
	dir := t.TempDir()
	parts := FilenameTemplateParts{Series: "X", Number: "1", Title: "Start"}
	files := epubBundleFiles(t)
	noProgress := func(page, progress int) {}

	for _, want := range []string{"X 1 - Start.cbz", "X 1 - Start v2.cbz"} {
		got, err := pack(dir, FormatCBZ, FilenameTemplateDefault, "X", parts, files, EPUBInfo{}, noProgress)
		if err != nil {
			t.Fatalf("pack() error = %v", err)
		}
		if got != want {
			t.Errorf("pack() = %q, want %q", got, want)
		}
	}
}

// a name that's taken for every try stops after maxPackAttempts instead of
// looping forever
func TestPackGivesUpOnAName(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= maxPackAttempts; i++ {
		name := "X"
		if i > 1 {
			name = fmt.Sprintf("X v%d", i)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".cbz"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := pack(dir, FormatCBZ, "{{.Series}}", "X", FilenameTemplateParts{Series: "X"}, epubBundleFiles(t), EPUBInfo{}, func(page, progress int) {})
	if err == nil {
		t.Fatal("pack() succeeded with every name taken")
	}
}

// a merged file is named after the parts' shared title, else the chapter's
// number, whatever label style its release has (a site may give none)
func TestMergedChapterName(t *testing.T) {
	part := func(part, title string, style grabber.LabelStyle) *grabber.Chapter {
		return &grabber.Chapter{Number: 356, Title: title, Label: grabber.Label{Style: style, Part: part}}
	}
	for _, tt := range []struct {
		name  string
		parts []*grabber.Chapter
		want  string
	}{
		{"shared title", []*grabber.Chapter{part("1", "Un títol", grabber.LabelNumbered), part("2", "Un títol", grabber.LabelNumbered)}, "Un títol"},
		{"titles differ", []*grabber.Chapter{part("1", "Berserk 356a", grabber.LabelNumbered), part("2", "Berserk 356b", grabber.LabelNumbered)}, "Chapter 356"},
		{"no parts given", []*grabber.Chapter{part("", "Berserk 356a", grabber.LabelTitle), part("", "Berserk 356b", grabber.LabelTitle)}, "Chapter 356"},
		{"identical duplicates keep their label", []*grabber.Chapter{part("", "Foo", grabber.LabelPrefixed), part("", "Foo", grabber.LabelPrefixed)}, "Chapter 356 - Foo"},
	} {
		chapter := mergedChapter(tt.parts)
		if got := grabber.ChapterLabel(&chapter); got != tt.want {
			t.Errorf("%s: named %q, want %q", tt.name, got, tt.want)
		}
	}
}

// the parts are packed in the order given, the first part's pages first
func TestPackPartsKeepsPartOrder(t *testing.T) {
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault}
	part := func(label string, files ...string) *DownloadedChapter {
		d := &DownloadedChapter{Chapter: &grabber.Chapter{Number: 19, Label: grabber.Label{Style: grabber.LabelNumbered, Part: label}}}
		for _, f := range files {
			d.Files = append(d.Files, &downloader.File{Data: []byte(f)})
		}
		return d
	}
	chapters := []*DownloadedChapter{part("1", "p1-0"), part("2", "p2-0", "p2-1")}

	dir := t.TempDir()
	filename, err := PackParts(dir, site, chapters, func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackParts: %v", err)
	}
	names := entryNames(t, filepath.Join(dir, filename))
	want := []string{"Chapter 0019/000.jpg", "Chapter 0019 (2)/000.jpg", "Chapter 0019 (2)/001.jpg"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Errorf("entries = %v, want %v", names, want)
	}
}

// the name helpers give what the Pack* functions write on an empty folder, for
// every format, and a second pack of the same file gets the " v2" they leave out
func TestNameHelpersMatchPack(t *testing.T) {
	// EPUB shows only real images, so the pages are a PNG
	page := pngBytes(t, gradientImage(8, 12))
	chapters := []*DownloadedChapter{
		{Chapter: &grabber.Chapter{Number: 1, Title: "Start", Group: "Alpha"}, Files: []*downloader.File{{Data: page}}},
		{Chapter: &grabber.Chapter{Number: 2, Title: "Next", Group: "Alpha"}, Files: []*downloader.File{{Data: page}}},
	}
	parts := []*DownloadedChapter{
		{Chapter: &grabber.Chapter{Number: 19, Label: grabber.Label{Style: grabber.LabelNumbered, Part: "1"}}, Files: []*downloader.File{{Data: page}}},
		{Chapter: &grabber.Chapter{Number: 19, Label: grabber.Label{Style: grabber.LabelNumbered, Part: "2"}}, Files: []*downloader.File{{Data: page}}},
	}
	listed := chaptersOf(chapters)
	noProgress := func(page, progress int) {}

	for _, tt := range []struct {
		format string
		ext    string
	}{
		{FormatCBZ, ".cbz"}, {FormatCBR, ".cbr"}, {FormatEPUB, ".epub"}, {FormatRaw, ""},
	} {
		site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: tt.format}
		cases := []struct {
			name  string
			first func() (string, error)
			pack  func(dir string) (string, error)
		}{
			{"chapter",
				func() (string, error) {
					return ChapterName("Test Series", FilenameTemplateDefault, tt.format, listed[0])
				},
				func(dir string) (string, error) { return PackSingle(dir, site, chapters[0], noProgress) }},
			{"volume",
				func() (string, error) {
					return VolumeName("Test Series", FilenameTemplateDefault, tt.format, 2, "", listed)
				},
				func(dir string) (string, error) { return PackVolume(dir, site, 2, "", chapters, noProgress) }},
			{"parts",
				func() (string, error) {
					return PartsName("Test Series", FilenameTemplateDefault, tt.format, chaptersOf(parts))
				},
				func(dir string) (string, error) { return PackParts(dir, site, parts, noProgress) }},
			{"bundle",
				func() (string, error) {
					return BundleName("Test Series", FilenameTemplateDefault, tt.format, "1-2", listed)
				},
				func(dir string) (string, error) { return PackBundle(dir, site, chapters, "1-2", noProgress) }},
		}
		for _, c := range cases {
			t.Run(tt.format+" "+c.name, func(t *testing.T) {
				first, err := c.first()
				if err != nil {
					t.Fatalf("name: %v", err)
				}
				dir := t.TempDir()
				if Exists(dir, first) {
					t.Fatalf("%s exists in an empty folder", first)
				}
				got, err := c.pack(dir)
				if err != nil {
					t.Fatalf("pack: %v", err)
				}
				if got != first {
					t.Errorf("pack wrote %q, the helper named %q", got, first)
				}
				if !Exists(dir, first) {
					t.Errorf("Exists(%q) = false after packing", first)
				}

				again, err := c.pack(dir)
				if err != nil {
					t.Fatalf("second pack: %v", err)
				}
				if want := strings.TrimSuffix(first, tt.ext) + " v2" + tt.ext; again != want {
					t.Errorf("second pack wrote %q, want %q", again, want)
				}
				if first2, _ := c.first(); first2 != first {
					t.Errorf("helper changed to %q after a pack", first2)
				}
			})
		}
	}
}

// Exists is true for a file or a folder in the output directory, and false otherwise
func TestExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.cbz"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir, "file.cbz") || !Exists(dir, "folder") {
		t.Error("Exists misses a file or a folder")
	}
	if Exists(dir, "missing.cbz") {
		t.Error("Exists finds a name that isn't there")
	}
}
