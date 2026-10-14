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
		{"query kept", "/user/page_direct?comic_id=a&chapter_slot=3", "/user/page_direct?chapter_slot=3&comic_id=a"},
		{"query params sorted", "https://example.com/user/page_direct?section_slot=0&chapter_slot=3&comic_id=a", "/user/page_direct?chapter_slot=3&comic_id=a&section_slot=0"},
		{"fragment dropped", "https://example.com/read/manga/chapter-1#page-2", "/read/manga/chapter-1"},
		{"host ignored", "https://www.example.com/read/manga/chapter-1", "/read/manga/chapter-1"},
		{"host change keeps id", "https://mirror.example.net/read/manga/chapter-1", "/read/manga/chapter-1"},
		{"baozimh reader link", "https://www.baozimh.com/user/page_direct?comic_id=zhihuo&section_slot=0&chapter_slot=108", "/user/page_direct?chapter_slot=108&comic_id=zhihuo&section_slot=0"},
		{"query kept on host change", "https://mirror.example.net/user/page_direct?comic_id=a&chapter_slot=3", "/user/page_direct?chapter_slot=3&comic_id=a"},
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
