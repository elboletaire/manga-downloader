// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

func TestFilenameDefaultGroup(t *testing.T) {
	tests := []struct {
		name    string
		group   string
		version int
		want    string
	}{
		{"with group", "Alpha", 1, "Series 5 - Chapter 5 [Alpha]"},
		{"without group", "", 1, "Series 5 - Chapter 5"},
		{"duplicate", "Alpha", 2, "Series 5 - Chapter 5 [Alpha] v2"},
		{"duplicate without group", "", 2, "Series 5 - Chapter 5 v2"},
	}
	for _, tt := range tests {
		chapter := &grabber.Chapter{Number: 5, Title: "Chapter 5", Group: tt.group}
		parts := NewChapterFileTemplateParts("Series", chapter)
		parts.Version = tt.version
		got, err := NewFilenameFromTemplate(FilenameTemplateDefault, parts)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestVolumeTemplateDefaults(t *testing.T) {
	for _, templ := range []string{"", FilenameTemplateDefault, filenameTemplateLegacyDefault} {
		if got := VolumeTemplate(templ); got != FilenameVolumeTemplateDefault {
			t.Errorf("VolumeTemplate(%q) = %q, want the volume default", templ, got)
		}
	}

	parts := NewVolumeFileTemplateParts("Series", 1, []*grabber.Chapter{{Number: 1, Group: "Alpha"}})
	got, err := NewFilenameFromTemplate(VolumeTemplate(FilenameTemplateDefault), parts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Series Vol. 01 [Alpha]" {
		t.Errorf("volume with group: got %q", got)
	}
}

func TestMigrateFilenameTemplate(t *testing.T) {
	if got := MigrateFilenameTemplate(filenameTemplateLegacyDefault); got != FilenameTemplateDefault {
		t.Errorf("legacy default: got %q, want the new default", got)
	}
	custom := "{{.Series}} {{.Number}}"
	if got := MigrateFilenameTemplate(custom); got != custom {
		t.Errorf("custom template: got %q, want it unchanged", got)
	}
}

// a volume or bundle keeps a group only when all its chapters share it
func TestVolumeGroupIsShared(t *testing.T) {
	tests := []struct {
		name     string
		chapters []*grabber.Chapter
		want     string
	}{
		{"shared", []*grabber.Chapter{{Number: 1, Group: "Alpha"}, {Number: 2, Group: "Alpha"}}, "Alpha"},
		{"mixed", []*grabber.Chapter{{Number: 1, Group: "Alpha"}, {Number: 2, Group: "Delta"}}, ""},
	}
	for _, tt := range tests {
		parts := NewVolumeFileTemplateParts("Series", 1, tt.chapters)
		if parts.Group != tt.want {
			t.Errorf("%s: Group %q, want %q", tt.name, parts.Group, tt.want)
		}
	}
}
