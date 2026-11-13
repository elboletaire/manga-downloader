// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

// linkSite is a listSite loaded from a link naming the release linkedID
type linkSite struct {
	listSite
	linkedID string
}

func (f linkSite) LinkedID() string { return f.linkedID }

// TestSeriesLinked: the release a link names is found once for both front ends,
// in the listing as filtered, and never a locked one (#465)
func TestSeriesLinked(t *testing.T) {
	chapters := grabber.Filterables{
		&grabber.Chapter{Number: 1, ID: "a", Group: "Alpha"},
		&grabber.Chapter{Number: 1, ID: "b", Group: "Delta"},
		&grabber.Chapter{Number: 2, ID: "c", Locked: true},
	}
	cases := []struct {
		name        string
		site        grabber.Site
		scanlator   string
		wantLinked  int
		wantMissing bool
	}{
		{"a listed release", linkSite{listSite{chapters: chapters}, "b"}, "", 1, false},
		{"found in the filtered listing", linkSite{listSite{chapters: chapters}, "b"}, "delta", 0, false},
		{"filtered out", linkSite{listSite{chapters: chapters}, "b"}, "alpha", -1, true},
		{"locked", linkSite{listSite{chapters: chapters}, "c"}, "", -1, true},
		{"not in the listing", linkSite{listSite{chapters: chapters}, "z"}, "", -1, true},
		{"a series link", linkSite{listSite{chapters: chapters}, ""}, "", -1, false},
		{"a site that names no release", listSite{chapters: chapters}, "", -1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			series, err := Open(c.site, &grabber.Settings{Scanlator: c.scanlator})
			if err != nil {
				t.Fatal(err)
			}
			if series.Linked != c.wantLinked || series.LinkMissing != c.wantMissing {
				t.Errorf("Linked = %d, LinkMissing = %v; want %d, %v", series.Linked, series.LinkMissing, c.wantLinked, c.wantMissing)
			}
		})
	}
}
