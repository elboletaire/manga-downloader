// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

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
	part := func(part, title string, style grabber.LabelStyle) *DownloadedChapter {
		return &DownloadedChapter{Chapter: &grabber.Chapter{Number: 356, Title: title, Label: grabber.Label{Style: style, Part: part}}}
	}
	for _, tt := range []struct {
		name  string
		parts []*DownloadedChapter
		want  string
	}{
		{"shared title", []*DownloadedChapter{part("1", "Un títol", grabber.LabelNumbered), part("2", "Un títol", grabber.LabelNumbered)}, "Un títol"},
		{"titles differ", []*DownloadedChapter{part("1", "Berserk 356a", grabber.LabelNumbered), part("2", "Berserk 356b", grabber.LabelNumbered)}, "Chapter 356"},
		{"no parts given", []*DownloadedChapter{part("", "Berserk 356a", grabber.LabelTitle), part("", "Berserk 356b", grabber.LabelTitle)}, "Chapter 356"},
		{"identical duplicates keep their label", []*DownloadedChapter{part("", "Foo", grabber.LabelPrefixed), part("", "Foo", grabber.LabelPrefixed)}, "Chapter 356 - Foo"},
	} {
		chapter := mergedChapter(tt.parts)
		if got := grabber.ChapterLabel(&chapter); got != tt.want {
			t.Errorf("%s: named %q, want %q", tt.name, got, tt.want)
		}
	}
}
