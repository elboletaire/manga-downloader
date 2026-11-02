// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/fatih/color"
)

// listedSeries is a series with two releases of chapter 1 (one per language,
// the English one with an ID and two groups, one of them named with a comma)
// and a locked chapter 2
func listedSeries() *core.Series {
	en := &grabber.Chapter{Number: 1, Title: "Start", Language: "en", ID: "a1", Volume: 1}
	en.SetGroups("Alpha, Beta Scans", "Gamma")
	return &core.Series{
		Site:  &grabber.Mangadex{},
		Title: "Some Series",
		Chapters: grabber.Filterables{
			en,
			&grabber.Chapter{Number: 1, Language: "es", Volume: 1},
			&grabber.Chapter{Number: 2, Language: "en", Locked: true},
		},
		Languages: &grabber.FilterOptions{Values: []string{"es", "en"}, All: true},
	}
}

func TestListingOf(t *testing.T) {
	s := listedSeries()

	l := listingOf(s)

	if l.Title != "Some Series" || l.Grabber != "Mangadex" {
		t.Errorf("title, grabber = %q, %q, want %q, %q", l.Title, l.Grabber, "Some Series", "Mangadex")
	}
	if len(l.Chapters) != 3 {
		t.Fatalf("chapters = %d, want 3", len(l.Chapters))
	}
	want := listedChapter{ID: "a1", Number: 1, Title: grabber.ChapterLabel(s.Chapters[0]), Language: "en",
		Group: "Alpha, Beta Scans, Gamma", Groups: []string{"Alpha, Beta Scans", "Gamma"}, Volume: 1}
	if !reflect.DeepEqual(l.Chapters[0], want) {
		t.Errorf("chapters[0] = %+v, want %+v", l.Chapters[0], want)
	}
	if c := l.Chapters[1]; c.ID != "" || c.Group != "" || c.Groups != nil || c.Language != "es" {
		t.Errorf("chapters[1] = %+v, want no ID nor group, in es", c)
	}
	if !l.Chapters[2].Locked {
		t.Errorf("chapters[2] = %+v, want locked", l.Chapters[2])
	}
	if l.Languages == nil || strings.Join(l.Languages.Values, ",") != "en,es" || !l.Languages.All {
		t.Errorf("languages = %+v, want en,es sorted and all", l.Languages)
	}
	if l.Scanlators != nil {
		t.Errorf("scanlators = %+v, want nil when the series has none", l.Scanlators)
	}
	if want := core.SiteVolumes(s.Chapters); l.Volumes != want {
		t.Errorf("volumes = %q, want %q", l.Volumes, want)
	}
}

func TestWriteListingJSON(t *testing.T) {
	var out bytes.Buffer

	if err := writeListingJSON(&out, listingOf(listedSeries())); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Title      string           `json:"title"`
		Chapters   []map[string]any `json:"chapters"`
		Languages  *listedFilter    `json:"languages"`
		Scanlators *listedFilter    `json:"scanlators"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output isn't JSON: %v\n%s", err, out.String())
	}
	if got.Title != "Some Series" || len(got.Chapters) != 3 {
		t.Errorf("title, chapters = %q, %d", got.Title, len(got.Chapters))
	}
	if got.Chapters[0]["id"] != "a1" {
		t.Errorf("chapters[0].id = %v, want a1", got.Chapters[0]["id"])
	}
	if _, ok := got.Chapters[1]["id"]; ok {
		t.Errorf("chapters[1] has an id, want it left out when the grabber sets none")
	}
	if got.Languages == nil || got.Scanlators != nil {
		t.Errorf("languages, scanlators = %+v, %+v, want a filter and null", got.Languages, got.Scanlators)
	}
}

func TestWriteListingTable(t *testing.T) {
	defer func(noColor bool) { color.NoColor = noColor }(color.NoColor)
	color.NoColor = true
	var out bytes.Buffer

	if err := writeListingTable(&out, listingOf(listedSeries())); err != nil {
		t.Fatal(err)
	}

	text := out.String()
	for _, want := range []string{
		"Some Series (Mangadex, 3 chapters)",
		"Alpha",
		"yes",
		"Languages (--language): en, es, all",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("table lacks %q:\n%s", want, text)
		}
	}
	if !regexp.MustCompile(`NUMBER +TITLE +LANGUAGE +GROUP +VOLUME +LOCKED`).MatchString(text) {
		t.Errorf("table lacks the header with every column:\n%s", text)
	}
	if strings.Contains(text, "Groups (--scanlator)") {
		t.Errorf("table lists groups for a series without a group filter:\n%s", text)
	}
}

func TestWriteListingTableLeavesOutEmptyColumns(t *testing.T) {
	defer func(noColor bool) { color.NoColor = noColor }(color.NoColor)
	color.NoColor = true
	var out bytes.Buffer
	l := listing{Title: "Plain", Grabber: "PlainHTML", Chapters: []listedChapter{{Number: 1, Title: "Chapter 1"}}}

	if err := writeListingTable(&out, l); err != nil {
		t.Fatal(err)
	}

	for _, column := range []string{"LANGUAGE", "GROUP", "VOLUME", "LOCKED"} {
		if strings.Contains(out.String(), column) {
			t.Errorf("table has the %s column with no chapter filling it:\n%s", column, out.String())
		}
	}
}

func TestGrabberName(t *testing.T) {
	if got := grabberName(&grabber.Mangadex{}); got != "Mangadex" {
		t.Errorf("grabberName = %q, want Mangadex", got)
	}
	if got := grabberName(nil); got != "" {
		t.Errorf("grabberName(nil) = %q, want empty", got)
	}
}
