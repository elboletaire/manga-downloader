// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/elboletaire/manga-downloader/packer"
)

// OptionsDTO are the options the window exposes. The frontend edits them and
// gui.json stores them, so the JSON names are part of the saved file.
type OptionsDTO struct {
	OutputDir        string `json:"outputDir"`
	Format           string `json:"format"`
	ReadingDirection string `json:"readingDirection"`
	PackAs           string `json:"packAs"`
	Language         string `json:"language"`
	Scanlator        string `json:"scanlator"`
	ConvertImages    string `json:"convertImages"`
	SkipPages        string `json:"skipPages"`
	FilenameTemplate string `json:"filenameTemplate"`
	BrowserVisible   bool   `json:"browserVisible"`
}

// PrefsDTO are the app-only preferences, which the CLI has no flag for, so they
// stay out of grabber.Settings. gui.json stores them next to OptionsDTO.
type PrefsDTO struct {
	// UILanguage is the window's language: "" follows the system, else en, es or ca
	UILanguage string `json:"uiLanguage,omitempty"`
	// CheckUpdates is whether the app checks for a newer release at startup
	CheckUpdates bool `json:"checkUpdates"`
}

// guiFile is what gui.json holds: the options and the preferences, flat
type guiFile struct {
	OptionsDTO
	PrefsDTO
}

// defaultPrefs are the preferences of a first run, and of any key gui.json leaves out
func defaultPrefs() PrefsDTO {
	return PrefsDTO{CheckUpdates: true}
}

// validUILanguage reports whether lang is one the window is translated into, or
// "" for the system's
func validUILanguage(lang string) bool {
	switch lang {
	case "", "en", "es", "ca":
		return true
	}

	return false
}

// How the chapters are packed: the window's "Pack as" choice
const (
	// PackChapters is one file per chapter
	PackChapters = "chapters"
	// PackParts is one file per chapter, its parts merged (--volumes parts)
	PackParts = "parts"
	// PackVolumes is one file per volume (--volumes)
	PackVolumes = "volumes"
	// PackBundle is one file for everything (--bundle)
	PackBundle = "bundle"
)

// packAsOf returns the "Pack as" choice the settings amount to
func packAsOf(s grabber.Settings) string {
	switch {
	case s.Bundle:
		return PackBundle
	case s.Volumes == core.VolumesParts:
		return PackParts
	case s.Volumes != "":
		return PackVolumes
	}
	return PackChapters
}

// applyPackAs sets the settings for a "Pack as" choice. Volumes keeps the
// volumes the settings already have; the Volumes box replaces them at Start,
// so "auto" only stands for the choice until then.
func applyPackAs(s *grabber.Settings, packAs string) {
	switch packAs {
	case PackBundle:
		s.Bundle, s.Volumes = true, ""
	case PackParts:
		s.Bundle, s.Volumes = false, core.VolumesParts
	case PackVolumes:
		s.Bundle = false
		if s.Volumes == "" || s.Volumes == core.VolumesParts {
			s.Volumes = core.VolumesAuto
		}
	default:
		s.Bundle, s.Volumes = false, ""
	}
}

// defaultSettings are the settings the CLI starts from, with the output
// directory the GUI picks for a first run
func defaultSettings() *grabber.Settings {
	return &grabber.Settings{
		MaxConcurrency:   grabber.MaxConcurrency{Chapters: 5, Pages: 10},
		Retry:            1,
		Format:           packer.FormatCBZ,
		ReadingDirection: packer.ReadingDirectionRTL,
		ConvertImages:    grabber.ConvertImagesDefault,
		FilenameTemplate: packer.FilenameTemplateDefault,
		OutputDir:        defaultOutputDir(),
	}
}

// defaultOutputDir is the Downloads folder when there is one, the home folder
// otherwise, and the working directory as a last resort
func defaultOutputDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./"
	}
	downloads := filepath.Join(home, "Downloads")
	if info, err := os.Stat(downloads); err == nil && info.IsDir() {
		return downloads
	}
	return home
}

// optionsOf returns the options the window shows for s
func optionsOf(s grabber.Settings) OptionsDTO {
	return OptionsDTO{
		OutputDir:        s.OutputDir,
		Format:           s.Format,
		ReadingDirection: s.ReadingDirection,
		PackAs:           packAsOf(s),
		Language:         s.Language,
		Scanlator:        s.Scanlator,
		ConvertImages:    s.ConvertImages,
		SkipPages:        s.SkipPages,
		FilenameTemplate: s.FilenameTemplate,
		BrowserVisible:   s.BrowserVisible,
	}
}

// applyOptions writes the window's options into s. Everything else in s, such
// as the concurrency the window doesn't expose, is left alone.
func applyOptions(s *grabber.Settings, o OptionsDTO) {
	s.OutputDir = o.OutputDir
	s.Format = o.Format
	s.ReadingDirection = o.ReadingDirection
	applyPackAs(s, o.PackAs)
	s.Language = o.Language
	s.Scanlator = o.Scanlator
	s.ConvertImages = o.ConvertImages
	s.SkipPages = o.SkipPages
	s.FilenameTemplate = o.FilenameTemplate
	s.BrowserVisible = o.BrowserVisible
}

// guiConfigPath is where the options are saved, next to the Manga Plus secret
func guiConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not find a user config dir for the saved options: %w", err)
	}

	return filepath.Join(dir, "manga-downloader", "gui.json"), nil
}

// loadOptions overlays the options saved at path on s. A missing file keeps s
// as it is; keys missing from the file keep their current value; a file that
// doesn't parse is an error and leaves s untouched.
func loadOptions(path string, s *grabber.Settings) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	o := optionsOf(*s)
	if err := json.Unmarshal(data, &o); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	// files from before "Pack as" saved the bundle switch on its own
	var legacy struct {
		Bundle *bool   `json:"bundle"`
		PackAs *string `json:"packAs"`
	}
	_ = json.Unmarshal(data, &legacy)
	if legacy.PackAs == nil && legacy.Bundle != nil {
		o.PackAs = PackChapters
		if *legacy.Bundle {
			o.PackAs = PackBundle
		}
	}
	switch o.PackAs {
	case PackChapters, PackParts, PackVolumes, PackBundle:
	default:
		notice.Warn("ignoring the saved pack-as %q, using %q", o.PackAs, PackChapters)
		o.PackAs = PackChapters
	}
	o.FilenameTemplate = packer.MigrateFilenameTemplate(o.FilenameTemplate)
	// an unknown format would only fail at Start, so fall back to the default now
	o.Format = strings.ToLower(strings.TrimSpace(o.Format))
	switch o.Format {
	case packer.FormatCBZ, packer.FormatCBR, packer.FormatRaw, packer.FormatEPUB:
	default:
		notice.Warn("ignoring the saved format %q, using %q", o.Format, packer.FormatCBZ)
		o.Format = packer.FormatCBZ
	}
	// the same normalisation core.Validate applies at Start, so a saved "RTL" is kept
	o.ReadingDirection = packer.NormalizeReadingDirection(o.ReadingDirection)
	switch o.ReadingDirection {
	case packer.ReadingDirectionRTL, packer.ReadingDirectionLTR:
	default:
		notice.Warn("ignoring the saved reading-direction %q, using %q", o.ReadingDirection, packer.ReadingDirectionRTL)
		o.ReadingDirection = packer.ReadingDirectionRTL
	}
	// a value the parser rejects would only fail at Start, once a series is
	// loaded, so fall back to the default now and say so
	if _, err := grabber.ParseConvertFormats(o.ConvertImages); err != nil {
		notice.Warn("ignoring the saved convert-images %q, using %q: %v", o.ConvertImages, grabber.ConvertImagesDefault, err)
		o.ConvertImages = grabber.ConvertImagesDefault
	}
	applyOptions(s, o)

	return nil
}

// loadPrefs reads the preferences saved at path. Keys missing from the file
// keep their defaults; a file that doesn't parse gives the defaults, which
// loadOptions reports.
func loadPrefs(path string) PrefsDTO {
	p := defaultPrefs()
	data, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	f := guiFile{PrefsDTO: p}
	if err := json.Unmarshal(data, &f); err != nil {
		return p
	}
	p = f.PrefsDTO
	if !validUILanguage(p.UILanguage) {
		notice.Warn("ignoring the saved ui-language %q, following the system", p.UILanguage)
		p.UILanguage = ""
	}

	return p
}

// saveOptions writes the options and the preferences to path, replacing the file
// in one step so a crash mid-write can't leave a truncated file behind. The file
// is private to the user, like the Manga Plus secret.
func saveOptions(path string, o OptionsDTO, p PrefsDTO) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(guiFile{OptionsDTO: o, PrefsDTO: p}, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	// sync before the rename, or a power cut can leave the new name pointing
	// at data that never reached the disk
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0600); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}
