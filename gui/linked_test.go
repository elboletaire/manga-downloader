// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

// linkedSite is the fake site loaded from a chapter link: it names the release
// with the ID linkedID, and its listing carries IDs ("c1", "c2", ...)
type linkedSite struct {
	*fakeSite
	linkedID string
}

func (s *linkedSite) LinkedID() string { return s.linkedID }

// idListing is a listed chapter with the release ID LinkedIndex matches
type idListing struct {
	listing
	id string
}

func (l idListing) GetID() string { return l.id }

func (s *linkedSite) FetchChapters() (grabber.Filterables, []error) {
	out := grabber.Filterables{}
	for _, c := range s.chapters {
		out = append(out, idListing{
			listing: listing{number: c.number, title: fmt.Sprintf("Chapter %g", c.number), locked: c.locked},
			id:      fmt.Sprintf("c%g", c.number),
		})
	}
	return out, nil
}

// loadLinked loads a fake series of chapters 1..3 (chapter 2 locked when
// lockLinked) from a link naming linkedID, through an App, so the chapter rows
// the window gets can be checked too
func loadLinked(t *testing.T, linkedID string, lockLinked bool) *App {
	t.Helper()
	c, site, _ := newTestController(t, false, nil,
		fakeChapter{number: 1, pages: 1}, fakeChapter{number: 2, pages: 1, locked: lockLinked}, fakeChapter{number: 3, pages: 1})
	linked := &linkedSite{fakeSite: site, linkedID: linkedID}
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return linked, nil
	}
	a := newApp(c, &fakeSink{}, filepath.Join(t.TempDir(), "gui.json"), "", "v1.2.3")
	a.interval = time.Millisecond
	c.notify = a.notify
	if err := a.Load("https://example.com/manga/test/chapter/2"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the load", func() bool { return !a.State().Loading })
	return a
}

// lastLog returns the key of the last log line's message
func lastLog(c *Controller) (string, LogLevel) {
	log := c.Log()
	last := log[len(log)-1]
	if last.Msg == nil {
		return last.Text, last.Level
	}
	return last.Msg.Key, last.Level
}

// TestLoadSelectsTheLinkedRelease is #465: a chapter or volume link lands on
// what it names, selected and flagged for the list to open on
func TestLoadSelectsTheLinkedRelease(t *testing.T) {
	a := loadLinked(t, "c2", false)
	c := a.ctrl

	if got := c.Linked(); got != 1 {
		t.Errorf("Linked() = %d, want 1", got)
	}
	if got := c.SelectedIndices(); !slices.Equal(got, []int{1}) {
		t.Errorf("selected %v, want only the linked chapter", got)
	}
	if key, level := lastLog(c); key != "log.linked" || level != LogInfo {
		t.Errorf("last log = %q (%v), want log.linked", key, level)
	}
	if text := c.Log()[len(c.Log())-1].Text; text != "Selected Chapter 2, the one the link names" {
		t.Errorf("log text = %q", text)
	}
	var flagged []int
	for _, row := range a.Chapters() {
		if row.Linked {
			flagged = append(flagged, row.Index)
		}
	}
	if !slices.Equal(flagged, []int{1}) {
		t.Errorf("rows flagged linked: %v, want [1]", flagged)
	}

	// clearing the series lets the linked release go with it
	if err := a.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := c.Linked(); got != -1 {
		t.Errorf("Linked() after Clear = %d, want -1", got)
	}
}

func TestLoadLinkedReleaseMissing(t *testing.T) {
	// another language's chapter: the series loads, with nothing selected
	a := loadLinked(t, "c99", false)
	c := a.ctrl
	if !c.Loaded() || c.Linked() != -1 || len(c.SelectedIndices()) != 0 {
		t.Errorf("loaded %v, linked %d, selected %v", c.Loaded(), c.Linked(), c.SelectedIndices())
	}
	if key, level := lastLog(c); key != "log.linkedMissing" || level != LogWarn {
		t.Errorf("last log = %q (%v), want the log.linkedMissing warning", key, level)
	}
}

func TestLoadLinkedReleaseLocked(t *testing.T) {
	// a locked chapter can't be selected, so the link selects nothing
	a := loadLinked(t, "c2", true)
	c := a.ctrl
	if c.Linked() != -1 || len(c.SelectedIndices()) != 0 {
		t.Errorf("linked %d, selected %v, want nothing", c.Linked(), c.SelectedIndices())
	}
	for _, row := range a.Chapters() {
		if row.Linked {
			t.Errorf("row %d flagged linked", row.Index)
		}
	}
}

func TestLoadSeriesLinkSelectsNothing(t *testing.T) {
	a := loadLinked(t, "", false)
	c := a.ctrl
	if c.Linked() != -1 || len(c.SelectedIndices()) != 0 {
		t.Errorf("linked %d, selected %v, want nothing", c.Linked(), c.SelectedIndices())
	}
	if key, _ := lastLog(c); key != "log.loaded" {
		t.Errorf("last log = %q, want log.loaded alone", key)
	}
}
