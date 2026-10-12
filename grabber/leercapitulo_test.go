// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestLeerCapituloSeriesURL(t *testing.T) {
	tests := []struct {
		name       string
		chapterURL string
		wantURL    string
		wantOK     bool
	}{
		{
			name:       "chapter URL",
			chapterURL: "https://www.leercapitulo.co/leer/0cj9hhn6di/kingdom/886/",
			wantURL:    "https://www.leercapitulo.co/manga/0cj9hhn6di/kingdom/",
			wantOK:     true,
		},
		{
			name:       "chapter URL without trailing slash",
			chapterURL: "https://www.leercapitulo.co/leer/abc123/test-manga/42",
			wantURL:    "https://www.leercapitulo.co/manga/abc123/test-manga/",
			wantOK:     true,
		},
		{
			name:       "decimal chapter number",
			chapterURL: "https://www.leercapitulo.co/leer/0cj9hhn6di/kingdom/733.5/",
			wantURL:    "https://www.leercapitulo.co/manga/0cj9hhn6di/kingdom/",
			wantOK:     true,
		},
		{
			name:       "chapter URL with query string",
			chapterURL: "https://www.leercapitulo.co/leer/0cj9hhn6di/kingdom/886/?page=2",
			wantURL:    "https://www.leercapitulo.co/manga/0cj9hhn6di/kingdom/",
			wantOK:     true,
		},
		{
			name:       "chapter URL with fragment",
			chapterURL: "https://www.leercapitulo.co/leer/0cj9hhn6di/kingdom/886/#top",
			wantURL:    "https://www.leercapitulo.co/manga/0cj9hhn6di/kingdom/",
			wantOK:     true,
		},
		{
			name:       "series URL returns false",
			chapterURL: "https://www.leercapitulo.co/manga/0cj9hhn6di/kingdom/",
			wantURL:    "",
			wantOK:     false,
		},
		{
			name:       "non-leercapitulo URL",
			chapterURL: "https://example.com/leer/id/slug/1/",
			wantURL:    "",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := leerCapituloSeriesURL(tt.chapterURL)
			if ok != tt.wantOK {
				t.Errorf("leerCapituloSeriesURL(%q) ok = %v, want %v", tt.chapterURL, ok, tt.wantOK)
			}
			if got != tt.wantURL {
				t.Errorf("leerCapituloSeriesURL(%q) = %q, want %q", tt.chapterURL, got, tt.wantURL)
			}
		})
	}
}
