// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestWeebCentralChapterURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "chapter URL",
			url:  "https://weebcentral.com/chapters/01M20S9HSSGNNMVT849MH5V86R",
			want: true,
		},
		{
			name: "chapter URL with query",
			url:  "https://weebcentral.com/chapters/01M20S9HSSGNNMVT849MH5V86R?page=1",
			want: true,
		},
		{
			name: "www. prefix",
			url:  "https://www.weebcentral.com/chapters/01M20S9HSSGNNMVT849MH5V86R",
			want: true,
		},
		{
			name: "series URL",
			url:  "https://weebcentral.com/series/01J76XYDXH7KT6AABVG3JAT3ZP/Shangri-La-Frontier",
			want: false,
		},
		{
			name: "non-weebcentral URL",
			url:  "https://example.com/chapters/123",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weebCentralChapterURL(tt.url)
			if got != tt.want {
				t.Errorf("weebCentralChapterURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestWeebCentralChapterID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"chapter URL", "https://weebcentral.com/chapters/01M20S9HSSGNNMVT849MH5V86R", "01M20S9HSSGNNMVT849MH5V86R"},
		{"chapter URL with query", "https://weebcentral.com/chapters/01M20S9HSSGNNMVT849MH5V86R?page=1", "01M20S9HSSGNNMVT849MH5V86R"},
		{"www. prefix", "https://www.weebcentral.com/chapters/01M20S9HSSGNNMVT849MH5V86R", "01M20S9HSSGNNMVT849MH5V86R"},
		{"series URL", "https://weebcentral.com/series/01J76XYDXH7KT6AABVG3JAT3ZP/Shangri-La-Frontier", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weebCentralChapterID(tt.url); got != tt.want {
				t.Errorf("weebCentralChapterID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
