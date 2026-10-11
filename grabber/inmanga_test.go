// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestInmangaSeriesURL(t *testing.T) {
	tests := []struct {
		name       string
		chapterURL string
		html       string
		wantURL    string
		wantOK     bool
	}{
		{
			name:       "chapter URL with mid in HTML",
			chapterURL: "https://inmanga.com/ver/manga/One-Piece/1195/a5ba0582-83c8-4556-bcfe-dc1d80142f58",
			html:       "var cid = 'a5ba0582-83c8-4556-bcfe-dc1d80142f58';\n        var mid = 'dfc7ecb5-e9b3-4aa5-a61b-a498993cd935';",
			wantURL:    "https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935",
			wantOK:     true,
		},
		{
			name:       "chapter URL with query string and mid in HTML",
			chapterURL: "https://inmanga.com/ver/manga/One-Piece/1195/a5ba0582-83c8-4556-bcfe-dc1d80142f58?page=33e53185-9b13-474a-9f1b-59f588db6957",
			html:       "var mid = 'dfc7ecb5-e9b3-4aa5-a61b-a498993cd935';",
			wantURL:    "https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935",
			wantOK:     true,
		},
		{
			name:       "uppercase chapter id",
			chapterURL: "https://inmanga.com/ver/manga/One-Piece/1195/A5BA0582-83C8-4556-BCFE-DC1D80142F58",
			html:       "var mid = 'dfc7ecb5-e9b3-4aa5-a61b-a498993cd935';",
			wantURL:    "https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935",
			wantOK:     true,
		},
		{
			name:       "series URL returns false",
			chapterURL: "https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935",
			html:       "var mid = 'dfc7ecb5-e9b3-4aa5-a61b-a498993cd935';",
			wantURL:    "",
			wantOK:     false,
		},
		{
			name:       "chapter URL without mid in HTML returns false",
			chapterURL: "https://inmanga.com/ver/manga/One-Piece/1195/a5ba0582-83c8-4556-bcfe-dc1d80142f58",
			html:       "some other html without var mid",
			wantURL:    "",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := inmangaSeriesURL(tt.chapterURL, tt.html)
			if ok != tt.wantOK {
				t.Errorf("inmangaSeriesURL(%q, ...) ok = %v, want %v", tt.chapterURL, ok, tt.wantOK)
			}
			if got != tt.wantURL {
				t.Errorf("inmangaSeriesURL(%q, ...) = %q, want %q", tt.chapterURL, got, tt.wantURL)
			}
		})
	}
}
