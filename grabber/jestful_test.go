// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "testing"

func TestJestfulChapterURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "chapter URL",
			url:  "https://jestful.net/bsaq-hwms-jitsu-wa-ore-saikyou-deshita-chapter-154.html",
			want: true,
		},
		{
			name: "chapter URL with decimal",
			url:  "https://jestful.net/manga-name-chapter-1.5.html",
			want: true,
		},
		{
			name: "www. prefix",
			url:  "https://www.jestful.net/bsaq-manga-name-chapter-154.html",
			want: true,
		},
		{
			name: "chapter URL with query",
			url:  "https://jestful.net/bsaq-manga-name-chapter-154.html?page=1",
			want: true,
		},
		{
			name: "chapter URL with fragment",
			url:  "https://jestful.net/bsaq-manga-name-chapter-154.html#top",
			want: true,
		},
		{
			name: "series URL",
			url:  "https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html",
			want: false,
		},
		{
			name: "non-jestful URL",
			url:  "https://example.com/manga-chapter-1.html",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jestfulChapterRe.MatchString(tt.url)
			if got != tt.want {
				t.Errorf("jestfulChapterRe.MatchString(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestJestfulResolveSeriesURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantURL string
	}{
		{
			name:    "new chapter URL format",
			url:     "https://jestful.net/bsaq-jitsu-wa-ore-saikyou-deshita-chapter-154.html",
			wantURL: "https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html",
		},
		{
			name:    "old chapter URL format with hwms- in filename",
			url:     "https://jestful.net/bsaq-hwms-jitsu-wa-ore-saikyou-deshita-chapter-154.html",
			wantURL: "https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html",
		},
		{
			name:    "chapter URL with decimal",
			url:     "https://jestful.net/bsaq-some-manga-chapter-1.5.html",
			wantURL: "https://jestful.net/hwms-some-manga.html",
		},
		{
			name:    "www. chapter URL",
			url:     "https://www.jestful.net/bsaq-jitsu-wa-ore-saikyou-deshita-chapter-154.html",
			wantURL: "https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html",
		},
		{
			name:    "chapter URL with query string",
			url:     "https://jestful.net/bsaq-jitsu-wa-ore-saikyou-deshita-chapter-154.html?page=1",
			wantURL: "https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html",
		},
		{
			name:    "series URL unchanged",
			url:     "https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html",
			wantURL: "https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := &Jestful{Grabber: &Grabber{URL: c.url}}
			if err := j.resolveSeriesURL(); err != nil {
				t.Fatalf("resolveSeriesURL() error = %v", err)
			}
			if j.URL != c.wantURL {
				t.Errorf("URL = %q, want %q", j.URL, c.wantURL)
			}
		})
	}
}
