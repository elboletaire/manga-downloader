// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"testing"
)

func TestParseMangakChaptersList(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{
			name: "full list",
			raw: `{"success":true,"data":{"chapters":[` +
				`{"id":"2ZWkoJV8","url":"/nano-machine/chapter-328","name":"Chapter 328","slug":"chapter-328","number":397,"cv":1788422605659},` +
				`{"id":"YPVRLaj2","url":"/nano-machine/chapter-327","name":"Chapter 327","slug":"chapter-327","number":396,"cv":1788422605659},` +
				`{"id":"aBcDeFgH","url":"/nano-machine/chapter-0","name":"Chapter 0","slug":"chapter-0","number":1,"cv":1788422605659}]}}`,
			want: 3,
		},
		{name: "unsuccessful response", raw: `{"success":false,"message":"nope"}`, wantErr: true},
		{name: "missing data", raw: `{"success":true}`, wantErr: true},
		{name: "empty chapter list", raw: `{"success":true,"data":{"chapters":[]}}`, wantErr: true},
		{name: "not json", raw: `<!DOCTYPE html><html>an error page</html>`, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chapters, err := parseMangakChaptersList([]byte(c.raw))
			if c.wantErr {
				if err == nil {
					t.Fatal("parseMangakChaptersList() expected an error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseMangakChaptersList() error = %v", err)
			}
			if len(chapters) != c.want {
				t.Fatalf("parseMangakChaptersList() got %d chapters, want %d", len(chapters), c.want)
			}
		})
	}
}

func TestParseMangakChaptersListFields(t *testing.T) {
	raw := `{"success":true,"data":{"chapters":[{"id":"x","url":"/some-series/chapter-12.5","name":"Chapter 12.5","slug":"chapter-12.5","number":13}]}}`

	chapters, err := parseMangakChaptersList([]byte(raw))
	if err != nil {
		t.Fatalf("parseMangakChaptersList() error = %v", err)
	}
	if chapters[0].Name != "Chapter 12.5" || chapters[0].URL != "/some-series/chapter-12.5" {
		t.Errorf("parseMangakChaptersList()[0] = %+v, unexpected", chapters[0])
	}
}

func TestMangakNextDataSeriesPage(t *testing.T) {
	// a trimmed-down series page __NEXT_DATA__: the embedded chapters list
	// only carries the newest chapters, while stats.chaptersCount declares
	// the real total, and siteConfig carries the API base for the full list
	raw := `{"props":{"pageProps":{"siteConfig":{"apiUrl":"https://api.mangak.io"},` +
		`"initialManga":{"id":"jD6jV0DM","name":"Nano Machine","cv":1788422605659,` +
		`"chapters":[{"name":"Chapter 328","url":"/nano-machine/chapter-328"}],` +
		`"stats":{"chaptersCount":397}}}}}`

	data := &mangakNextData{}
	if err := json.Unmarshal([]byte(raw), data); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	manga := data.Props.PageProps.InitialManga
	if manga == nil {
		t.Fatal("initialManga is nil")
	}
	if manga.ID != "jD6jV0DM" || manga.CV != 1788422605659 {
		t.Errorf("manga id/cv = %q/%d, unexpected", manga.ID, manga.CV)
	}
	if manga.Stats.ChaptersCount != 397 {
		t.Errorf("chaptersCount = %d, want 397", manga.Stats.ChaptersCount)
	}
	if len(manga.Chapters) != 1 || manga.Chapters[0].URL != "/nano-machine/chapter-328" {
		t.Errorf("chapters = %+v, unexpected", manga.Chapters)
	}
	if data.Props.PageProps.SiteConfig.APIURL != "https://api.mangak.io" {
		t.Errorf("apiUrl = %q, unexpected", data.Props.PageProps.SiteConfig.APIURL)
	}
}
