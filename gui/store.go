// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// OptionsDTO are the options the window exposes. The frontend edits them and
// gui.json stores them, so the JSON names are part of the saved file.
type OptionsDTO struct {
	OutputDir        string `json:"outputDir"`
	Format           string `json:"format"`
	Bundle           bool   `json:"bundle"`
	Language         string `json:"language"`
	Scanlator        string `json:"scanlator"`
	ConvertImages    string `json:"convertImages"`
	SkipPages        string `json:"skipPages"`
	FilenameTemplate string `json:"filenameTemplate"`
	BrowserVisible   bool   `json:"browserVisible"`
}

// defaultSettings are the settings the CLI starts from, with the output
// directory the GUI picks for a first run
func defaultSettings() *grabber.Settings {
	return &grabber.Settings{
		MaxConcurrency:   grabber.MaxConcurrency{Chapters: 5, Pages: 10},
		Retry:            1,
		Format:           packer.FormatCBZ,
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
		Bundle:           s.Bundle,
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
	s.Bundle = o.Bundle
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
	applyOptions(s, o)

	return nil
}

// saveOptions writes o to path, replacing the file in one step so a crash
// mid-write can't leave a truncated file behind. The file is private to the
// user, like the Manga Plus secret.
func saveOptions(path string, o OptionsDTO) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(o, "", "  ")
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
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0600); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}
