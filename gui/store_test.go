// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
)

func TestOptionsSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.json")
	want := OptionsDTO{
		OutputDir:        "/books",
		Format:           "epub",
		ReadingDirection: "ltr",
		PackAs:           PackBundle,
		Language:         "es",
		Scanlator:        "group",
		ConvertImages:    "avif,webp",
		SkipPages:        "2,-1",
		FilenameTemplate: "{{.Series}} {{.Number}}",
		BrowserVisible:   true,
	}
	if err := saveOptions(path, want); err != nil {
		t.Fatal(err)
	}

	s := defaultSettings()
	if err := loadOptions(path, s); err != nil {
		t.Fatal(err)
	}
	if got := optionsOf(*s); got != want {
		t.Errorf("loaded %+v, want %+v", got, want)
	}
}

func TestSaveOptionsIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "gui.json")
	if err := saveOptions(path, OptionsDTO{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("gui.json mode %o, want 0600", perm)
	}
}

func TestLoadOptionsMissingFileKeepsDefaults(t *testing.T) {
	s := defaultSettings()
	before := optionsOf(*s)
	if err := loadOptions(filepath.Join(t.TempDir(), "absent.json"), s); err != nil {
		t.Fatalf("missing file is not an error: %v", err)
	}
	if optionsOf(*s) != before {
		t.Error("a missing file changed the settings")
	}
}

func TestLoadOptionsPartialFileKeepsOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.json")
	if err := os.WriteFile(path, []byte(`{"format":"raw","bundle":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	s := defaultSettings()
	s.OutputDir = "/keep"
	if err := loadOptions(path, s); err != nil {
		t.Fatal(err)
	}
	if s.Format != "raw" || !s.Bundle {
		t.Errorf("present keys not loaded: format %q bundle %v", s.Format, s.Bundle)
	}
	if s.OutputDir != "/keep" || s.ConvertImages != grabber.ConvertImagesDefault {
		t.Errorf("missing keys changed: output %q convert %q", s.OutputDir, s.ConvertImages)
	}
}

func TestLoadOptionsCorruptFileLeavesSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.json")
	if err := os.WriteFile(path, []byte(`{"format":`), 0600); err != nil {
		t.Fatal(err)
	}
	s := defaultSettings()
	before := optionsOf(*s)
	if err := loadOptions(path, s); err == nil {
		t.Fatal("a corrupt file was accepted")
	}
	if optionsOf(*s) != before {
		t.Error("a corrupt file changed the settings")
	}
}

// every convert-images value the window can save must come back unchanged,
// so a preference written by one run loads in the next
func TestConvertImagesValuesSurviveSaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.json")
	for _, value := range []string{"avif", "avif,webp", "webp", "none"} {
		if err := saveOptions(path, OptionsDTO{ConvertImages: value}); err != nil {
			t.Fatal(err)
		}
		s := defaultSettings()
		if err := loadOptions(path, s); err != nil {
			t.Fatal(err)
		}
		if s.ConvertImages != value {
			t.Errorf("saved %q, loaded %q", value, s.ConvertImages)
		}
		if _, err := grabber.ParseConvertFormats(s.ConvertImages); err != nil {
			t.Errorf("%q is not accepted by the CLI's parser: %v", value, err)
		}
	}
}

// a convert-images value the parser rejects must not reach the settings: it
// falls back to the default with a warning, instead of failing at Start
func TestLoadOptionsRejectsConvertImagesFallsBackToDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.json")
	if err := os.WriteFile(path, []byte(`{"convertImages":"jpeg","format":"raw"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var warnings []string
	notice.SetHandler(func(level notice.Level, msg string) {
		if level == notice.LevelWarn {
			warnings = append(warnings, msg)
		}
	})
	defer notice.SetHandler(nil)

	s := defaultSettings()
	if err := loadOptions(path, s); err != nil {
		t.Fatalf("an invalid convert-images is not a load error: %v", err)
	}
	if s.ConvertImages != grabber.ConvertImagesDefault {
		t.Errorf("convert-images %q, want the default %q", s.ConvertImages, grabber.ConvertImagesDefault)
	}
	if s.Format != "raw" {
		t.Errorf("the other keys were not loaded: format %q", s.Format)
	}
	if len(warnings) != 1 {
		t.Errorf("got %d warnings, want 1: %q", len(warnings), warnings)
	}
	if _, err := grabber.ParseConvertFormats(s.ConvertImages); err != nil {
		t.Errorf("the fallback is rejected by the parser: %v", err)
	}
}

func TestLoadOptionsRejectsReadingDirectionFallsBackToDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.json")
	if err := os.WriteFile(path, []byte(`{"readingDirection":"ttb","format":"epub"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var warnings []string
	notice.SetHandler(func(level notice.Level, msg string) {
		if level == notice.LevelWarn {
			warnings = append(warnings, msg)
		}
	})
	defer notice.SetHandler(nil)

	s := defaultSettings()
	if err := loadOptions(path, s); err != nil {
		t.Fatalf("an invalid reading-direction is not a load error: %v", err)
	}
	if s.ReadingDirection != "rtl" {
		t.Errorf("reading-direction %q, want the default rtl", s.ReadingDirection)
	}
	if s.Format != "epub" {
		t.Errorf("the other keys were not loaded: format %q", s.Format)
	}
	if len(warnings) != 1 {
		t.Errorf("got %d warnings, want 1: %q", len(warnings), warnings)
	}
}

// files saved before "Pack as" only had the bundle switch
func TestLoadOptionsMigratesBundle(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{`{"bundle":true}`, PackBundle},
		{`{"bundle":false}`, PackChapters},
		{`{"packAs":"volumes"}`, PackVolumes},
		{`{"packAs":"volumes","bundle":true}`, PackVolumes},
		{`{"packAs":"nonsense"}`, PackChapters},
		{`{}`, PackChapters},
	}
	notice.SetHandler(func(notice.Level, string) {})
	t.Cleanup(func() { notice.SetHandler(nil) })
	for _, tt := range tests {
		path := filepath.Join(t.TempDir(), "gui.json")
		if err := os.WriteFile(path, []byte(tt.file), 0600); err != nil {
			t.Fatal(err)
		}
		s := defaultSettings()
		if err := loadOptions(path, s); err != nil {
			t.Fatal(err)
		}
		if got := optionsOf(*s).PackAs; got != tt.want {
			t.Errorf("%s: pack-as %q, want %q", tt.file, got, tt.want)
		}
		if s.Bundle != (tt.want == PackBundle) || (s.Volumes != "") != (tt.want == PackVolumes) {
			t.Errorf("%s: settings bundle %v volumes %q", tt.file, s.Bundle, s.Volumes)
		}
	}
}

func TestApplyPackAsKeepsVolumes(t *testing.T) {
	s := defaultSettings()
	applyPackAs(s, PackVolumes)
	if s.Volumes != "auto" || s.Bundle {
		t.Fatalf("volumes: %q bundle %v", s.Volumes, s.Bundle)
	}
	s.Volumes = "1-7/8-16"
	applyPackAs(s, PackVolumes)
	if s.Volumes != "1-7/8-16" {
		t.Errorf("re-applying volumes replaced them with %q", s.Volumes)
	}
	applyPackAs(s, PackBundle)
	if s.Volumes != "" || !s.Bundle {
		t.Errorf("bundle: %q bundle %v", s.Volumes, s.Bundle)
	}
	applyPackAs(s, PackChapters)
	if s.Volumes != "" || s.Bundle {
		t.Errorf("chapters: %q bundle %v", s.Volumes, s.Bundle)
	}
}
