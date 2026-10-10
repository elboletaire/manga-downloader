//go:build smoke

// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

// TestSmokeRealSite drives the controller against a live site, like the
// makefile's grabber targets do for the CLI. It's skipped unless
// GUI_SMOKE_URL is set; GUI_SMOKE_RANGE picks the chapters and GUI_SMOKE_DIR
// is where the archives go.
//
//	GUI_SMOKE_URL=https://dynasty-scans.com/series/please_bully_me_miss_villainess \
//	GUI_SMOKE_RANGE=162 GUI_SMOKE_DIR=/tmp/md-fyne-ui/gui-smoke \
//	go test -tags "nodynamic smoke" -run TestSmokeRealSite -v ./
func TestSmokeRealSite(t *testing.T) {
	url := os.Getenv("GUI_SMOKE_URL")
	if url == "" {
		t.Skip("GUI_SMOKE_URL not set")
	}
	settings := defaultSettings()
	settings.OutputDir = os.Getenv("GUI_SMOKE_DIR")
	settings.MaxConcurrency = grabber.MaxConcurrency{Chapters: 2, Pages: 5}

	c := NewController(settings, nil)
	if err := c.Load(url); err != nil {
		t.Fatal(err)
	}
	t.Logf("loaded %q with %d chapters", c.Title(), len(c.Chapters()))
	if n, err := c.SelectRange(os.Getenv("GUI_SMOKE_RANGE")); err != nil || n == 0 {
		t.Fatalf("range selected %d chapters, err %v", n, err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	s := waitFinished(t, c)
	t.Logf("summary:\n%s", summaryText(s))
	if s.Saved == 0 || len(s.Failures) > 0 {
		t.Errorf("smoke download failed: %+v", s)
	}
}
