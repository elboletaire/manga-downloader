// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestChapterURLID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"absolute url", "https://example.com/read/manga/chapter-1", "/read/manga/chapter-1"},
		{"relative href", "/read/manga/chapter-1", "/read/manga/chapter-1"},
		{"query dropped", "https://example.com/read/manga/chapter-1?utm_source=x", "/read/manga/chapter-1"},
		{"fragment dropped", "https://example.com/read/manga/chapter-1#page-2", "/read/manga/chapter-1"},
		{"host ignored", "https://www.example.com/read/manga/chapter-1", "/read/manga/chapter-1"},
		{"host change keeps id", "https://mirror.example.net/read/manga/chapter-1", "/read/manga/chapter-1"},
		{"trailing slash kept", "https://example.com/read/manga/chapter-1/", "/read/manga/chapter-1/"},
		{"bare host has no path", "https://example.com", ""},
		{"empty", "", ""},
		{"unparseable", "https://[::1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chapterURLID(tt.url); got != tt.want {
				t.Errorf("chapterURLID(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
