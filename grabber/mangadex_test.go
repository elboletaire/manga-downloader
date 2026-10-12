// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestMangadexChapterURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "chapter URL",
			url:  "https://mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79",
			want: true,
		},
		{
			name: "chapter URL with query",
			url:  "https://mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79?page=5",
			want: true,
		},
		{
			name: "www. prefix (308 to apex)",
			url:  "https://www.mangadex.org/chapter/c780a907-8cf7-4d64-97fd-750519f8ff79",
			want: true,
		},
		{
			name: "uppercase UUID",
			url:  "https://mangadex.org/chapter/C780A907-8CF7-4D64-97FD-750519F8FF79",
			want: true,
		},
		{
			name: "series URL",
			url:  "https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece",
			want: false,
		},
		{
			name: "non-mangadex URL",
			url:  "https://example.com/chapter/123",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mangadexChapterURL(tt.url)
			if got != tt.want {
				t.Errorf("mangadexChapterURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
