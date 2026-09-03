// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"testing"

	"github.com/elboletaire/manga-downloader/browser"
)

func TestMangafireHid(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://mangafire.to/title/dkw-one-piece", "dkw", false},
		{"https://mangafire.to/title/dkw-one-piece/chapter/9054304", "dkw", false},
		{"https://mangafire.to/manga/one-piecee.dkw", "dkw", false}, // legacy format
		{"https://mangafire.to/title/", "", true},
		{"https://mangafire.to/", "", true},
	}

	for _, c := range cases {
		m := Mangafire{Grabber: &Grabber{URL: c.url}}
		got, err := m.hid()
		if (err != nil) != c.wantErr {
			t.Errorf("hid(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("hid(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestParseMangafireChapters(t *testing.T) {
	const prefix = "/api/titles/9062q/chapters"
	page := func(page int, body string) browser.APIResponse {
		return browser.APIResponse{
			URL:  "https://mangafire.to" + prefix + "?language=en&sort=number&order=desc&page=" + string(rune('0'+page)) + "&limit=20&vrf=x",
			Body: body,
		}
	}

	cases := []struct {
		name      string
		responses []browser.APIResponse
		wantNums  []float64
		wantTypes map[float64]string
	}{
		{
			// the #169 regression: chapters spread over several pager pages
			// (sorted descending by the site) must all be collected, in order
			name: "multiple pages merge",
			responses: []browser.APIResponse{
				page(1, `{"items":[{"id":3,"number":3,"name":"C3","language":"en","type":"official"},{"id":2,"number":2,"name":"C2","language":"en","type":"official"}]}`),
				page(2, `{"items":[{"id":1,"number":1,"name":"C1","language":"en","type":"official"}]}`),
			},
			wantNums: []float64{3, 2, 1},
		},
		{
			name: "official preferred over unofficial regardless of order",
			responses: []browser.APIResponse{
				page(1, `{"items":[{"id":10,"number":1,"type":"unofficial"},{"id":11,"number":1,"type":"official"},{"id":21,"number":2,"type":"official"},{"id":20,"number":2,"type":"unofficial"}]}`),
			},
			wantNums:  []float64{1, 2},
			wantTypes: map[float64]string{1: "official", 2: "official"},
		},
		{
			name: "non-matching and malformed responses are skipped",
			responses: []browser.APIResponse{
				{URL: "https://mangafire.to/api/titles/9062q?vrf=x", Body: `{"data":{"title":"nope"}}`},
				page(1, `not json`),
				page(2, `{"items":[{"id":1,"number":1,"type":"official"}]}`),
			},
			wantNums: []float64{1},
		},
		{
			name:      "no responses yields empty list",
			responses: nil,
			wantNums:  []float64{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseMangafireChapters(c.responses, prefix)
			if len(got) != len(c.wantNums) {
				t.Fatalf("got %d chapters, want %d (%v)", len(got), len(c.wantNums), got)
			}
			for i, item := range got {
				if item.Number != c.wantNums[i] {
					t.Errorf("chapter[%d].Number = %v, want %v", i, item.Number, c.wantNums[i])
				}
				if want, ok := c.wantTypes[item.Number]; ok && item.Type != want {
					t.Errorf("chapter %v type = %q, want %q", item.Number, item.Type, want)
				}
			}
		})
	}
}
