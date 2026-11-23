// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

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

func TestWeebCentralRowNumber(t *testing.T) {
	// row mirrors a full-chapter-list anchor: the label span holds the number,
	// and the datetime digits must never be read as one
	row := func(label string) string {
		return `<a href="/chapters/01J76XZ0KMM9VF7W2V2AX5HQ9Q" class="flex-1">` +
			`<span class="grow flex items-center gap-2"><span class="">` + label + `</span>` +
			`<span class="flex gap-1"><span class="hidden md:inline">Last Read</span></span></span>` +
			`<time datetime="2024-09-07T17:04:15.717Z">2024-09-07T17:04:15.717343Z</time></a>`
	}

	tests := []struct {
		name   string
		label  string
		want   float64
		wantOk bool
	}{
		{"hash label", "# 261", 261, true},
		{"night label with decimal", "Night 200.8", 200.8, true},
		{"chapter word", "Chapter 274", 274, true},
		{"chapter decimal", "Chapter 12.5", 12.5, true},
		{"season and episode", "Season 2 Episode 3", 3, true},
		{"no number", "Special", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(row(tt.label)))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := weebCentralRowNumber(doc.Find("a"))
			if ok != tt.wantOk || got != tt.want {
				t.Errorf("weebCentralRowNumber(%q) = (%v, %v), want (%v, %v)", tt.label, got, ok, tt.want, tt.wantOk)
			}
		})
	}
}

func TestWeebCentralRowNumberWithoutLabelSpan(t *testing.T) {
	// no label span: the anchor's text is read, but not its <time> datetime
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<a href="/chapters/01J76XZ0KMM9VF7W2V2AX5HQ9Q">Night 200.8` +
			`<time datetime="2025-08-26T22:28:53.646Z">2025-08-26T22:28:53.646067Z</time></a>`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := weebCentralRowNumber(doc.Find("a"))
	if !ok || got != 200.8 {
		t.Errorf("weebCentralRowNumber() = (%v, %v), want (200.8, true)", got, ok)
	}
}
