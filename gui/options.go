// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"strconv"

	"fyne.io/fyne/v2"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// Preference keys, one per option the window exposes
const (
	prefOutputDir        = "output-dir"
	prefFormat           = "format"
	prefBundle           = "bundle"
	prefLanguage         = "language"
	prefScanlator        = "scanlator"
	prefConvertImages    = "convert-images"
	prefSkipPages        = "skip-pages"
	prefFilenameTemplate = "filename-template"
	prefBrowserVisible   = "browser-visible"
)

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

// loadOptions overlays the saved preferences on s. Missing keys keep the
// value s already has.
func loadOptions(p fyne.Preferences, s *grabber.Settings) {
	s.OutputDir = p.StringWithFallback(prefOutputDir, s.OutputDir)
	s.Format = p.StringWithFallback(prefFormat, s.Format)
	s.Bundle = p.BoolWithFallback(prefBundle, s.Bundle)
	s.Language = p.StringWithFallback(prefLanguage, s.Language)
	s.Scanlator = p.StringWithFallback(prefScanlator, s.Scanlator)
	s.ConvertImages = p.StringWithFallback(prefConvertImages, s.ConvertImages)
	s.SkipPages = p.StringWithFallback(prefSkipPages, s.SkipPages)
	s.FilenameTemplate = p.StringWithFallback(prefFilenameTemplate, s.FilenameTemplate)
	s.BrowserVisible = p.BoolWithFallback(prefBrowserVisible, s.BrowserVisible)
}

// saveOptions writes every option the window exposes
func saveOptions(p fyne.Preferences, s grabber.Settings) {
	p.SetString(prefOutputDir, s.OutputDir)
	p.SetString(prefFormat, s.Format)
	p.SetBool(prefBundle, s.Bundle)
	p.SetString(prefLanguage, s.Language)
	p.SetString(prefScanlator, s.Scanlator)
	p.SetString(prefConvertImages, s.ConvertImages)
	p.SetString(prefSkipPages, s.SkipPages)
	p.SetString(prefFilenameTemplate, s.FilenameTemplate)
	p.SetBool(prefBrowserVisible, s.BrowserVisible)
}

// plural renders a chapter count for the summary
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
