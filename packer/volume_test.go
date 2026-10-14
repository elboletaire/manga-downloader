// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
)

func TestVolumeFilenames(t *testing.T) {
	chapters := []*grabber.Chapter{{Number: 1}, {Number: 7.5}}
	tests := []struct {
		name     string
		template string
		volume   int
		chapters []*grabber.Chapter
		version  int
		want     string
	}{
		{"default", FilenameTemplateDefault, 1, chapters, 1, "One/Piece Vol. 01"},
		{"unset template", "", 12, chapters, 1, "One/Piece Vol. 12"},
		{"three digits", "", 123, chapters, 1, "One/Piece Vol. 123"},
		{"duplicate", FilenameTemplateDefault, 1, chapters, 2, "One/Piece Vol. 01 v2"},
		{"user template", "{{.Series}} {{.Number}} - {{.Title}}", 3, chapters, 1, "One_Piece 1-7.5 - Vol. 03"},
		{"single chapter", "{{.Number}}", 3, chapters[:1], 1, "1"},
		{"volume only", "v{{.Volume}}", 3, chapters, 1, "v3"},
	}
	for _, tt := range tests {
		parts := NewVolumeFileTemplateParts("One/Piece", tt.volume, tt.chapters)
		parts.Version = tt.version
		got, err := NewFilenameFromTemplate(VolumeTemplate(tt.template), parts)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		// the default template keeps the series as sanitized, "_" for "/"
		if tt.template == "" || tt.template == FilenameTemplateDefault {
			tt.want = "One_Piece" + tt.want[len("One/Piece"):]
		}
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestChapterFilenameVolume(t *testing.T) {
	parts := NewChapterFileTemplateParts("Series", &grabber.Chapter{Number: 12, Title: "Chapter 12", Volume: 2})
	got, err := NewFilenameFromTemplate(`{{.Series}} v{{printf "%02d" .Volume}} {{.Number}}`, parts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Series v02 12" {
		t.Errorf("got %q, want %q", got, "Series v02 12")
	}
}

// {{.Group}} names the chapter's group, sanitized, and a volume's or bundle's
// group only when all its chapters share one
func TestGroupInFilenameTemplates(t *testing.T) {
	chapter := &grabber.Chapter{Number: 3, Title: "Chapter 3", Group: "Alpha/Delta"}
	got, err := NewFilenameFromTemplate("{{.Series}} {{.Number}} [{{.Group}}]", NewChapterFileTemplateParts("Series", chapter))
	if err != nil {
		t.Fatal(err)
	}
	if got != "Series 3 [Alpha_Delta]" {
		t.Errorf("chapter: got %q", got)
	}

	shared := []*grabber.Chapter{
		{Number: 1, Group: "Alpha"},
		{Number: 2, Group: "Alpha"},
	}
	mixed := []*grabber.Chapter{
		{Number: 1, Group: "Alpha"},
		{Number: 2, Group: "Delta"},
	}
	ungrouped := []*grabber.Chapter{{Number: 1}, {Number: 2}}
	cases := []struct {
		name     string
		chapters []*grabber.Chapter
		want     string
	}{
		{"shared group", shared, "Series [Alpha]"},
		{"mixed groups", mixed, "Series []"},
		{"no group", ungrouped, "Series []"},
	}
	for _, c := range cases {
		parts := NewVolumeFileTemplateParts("Series", 1, c.chapters)
		got, err := NewFilenameFromTemplate("{{.Series}} [{{.Group}}]", parts)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPackVolume(t *testing.T) {
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault}
	chapters := []*DownloadedChapter{
		{Chapter: &grabber.Chapter{Number: 1}, Files: []*downloader.File{{Data: []byte("a")}, {Data: []byte("b")}}},
		{Chapter: &grabber.Chapter{Number: 2}, Files: []*downloader.File{{Data: []byte("c")}}},
	}

	dir := t.TempDir()
	for _, want := range []string{"Test Series Vol. 02.cbz", "Test Series Vol. 02 v2.cbz"} {
		pages := 0
		name, err := PackVolume(dir, site, 2, chapters, func(page, _ int) { pages += page })
		if err != nil {
			t.Fatalf("PackVolume: %v", err)
		}
		if name != want {
			t.Errorf("name = %q, want %q", name, want)
		}
		if pages != 3 {
			t.Errorf("progress reported %d pages, want 3", pages)
		}
		names := entryNames(t, filepath.Join(dir, name))
		if wantNames := []string{"Chapter 0001/000.jpg", "Chapter 0001/001.jpg", "Chapter 0002/000.jpg"}; !reflect.DeepEqual(names, wantNames) {
			t.Errorf("entries = %v, want %v", names, wantNames)
		}
	}
}
