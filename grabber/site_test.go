// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"testing"
)

func TestHostMatchesDomain(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		domain string
		want   bool
	}{
		// Exact domain match
		{
			name:   "exact domain",
			rawURL: "https://example.com/",
			domain: "example.com",
			want:   true,
		},
		// Subdomain match (wN. mirrors)
		{
			name:   "w1 subdomain",
			rawURL: "https://w1.example.com/manga/chapter-1/",
			domain: "example.com",
			want:   true,
		},
		{
			name:   "w99 subdomain",
			rawURL: "https://w99.example.com/",
			domain: "example.com",
			want:   true,
		},
		// Uppercase host must also match
		{
			name:   "uppercase host",
			rawURL: "https://EXAMPLE.COM/",
			domain: "example.com",
			want:   true,
		},
		{
			name:   "mixed-case subdomain",
			rawURL: "https://W7.Example.Com/path",
			domain: "example.com",
			want:   true,
		},
		// Lookalike: extra prefix before the domain
		{
			name:   "prefix lookalike",
			rawURL: "https://myexample.com/",
			domain: "example.com",
			want:   false,
		},
		// Lookalike: domain name only in path
		{
			name:   "domain in path",
			rawURL: "https://other.com/example.com/series",
			domain: "example.com",
			want:   false,
		},
		// Lookalike: domain name only in query string
		{
			name:   "domain in query",
			rawURL: "https://other.com/?url=example.com",
			domain: "example.com",
			want:   false,
		},
		// Unrelated domain
		{
			name:   "unrelated domain",
			rawURL: "https://mangadex.org/",
			domain: "example.com",
			want:   false,
		},
		// Hyphenated domain (heavenly-delusion.com, the-apothecary-diaries.com)
		{
			name:   "hyphenated domain exact",
			rawURL: "https://heavenly-delusion.com/",
			domain: "heavenly-delusion.com",
			want:   true,
		},
		{
			name:   "hyphenated domain subdomain",
			rawURL: "https://w7.heavenly-delusion.com/manga/chapter-1/",
			domain: "heavenly-delusion.com",
			want:   true,
		},
		{
			name:   "lookalike for hyphenated domain",
			rawURL: "https://myheavenly-delusion.com/",
			domain: "heavenly-delusion.com",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hostMatchesDomain(tt.rawURL, tt.domain)
			if got != tt.want {
				t.Errorf("hostMatchesDomain(%q, %q) = %v, want %v", tt.rawURL, tt.domain, got, tt.want)
			}
		})
	}
}
