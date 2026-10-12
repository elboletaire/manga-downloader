// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestMgekoExtractSeriesURL(t *testing.T) {
	tests := []struct {
		name string
		html string
		base string
		want string
	}{
		{
			name: "series URL from h1 title link",
			html: `<html><head></head><body>
				<h1><a href="/manga/solo-leveling-mg1/">Solo Leveling</a></h1>
			</body></html>`,
			base: "https://www.mgeko.cc",
			want: "https://www.mgeko.cc/manga/solo-leveling-mg1/",
		},
		{
			name: "series URL from og:url fallback",
			html: `<html><head>
				<meta property="og:url" content="https://www.mgeko.cc/manga/solo-leveling-mg1/">
			</head><body>
				<h1>Solo Leveling</h1>
			</body></html>`,
			base: "https://www.mgeko.cc",
			want: "https://www.mgeko.cc/manga/solo-leveling-mg1/",
		},
		{
			name: "h1 link with /manga/ takes priority over og:url",
			html: `<html><head>
				<meta property="og:url" content="https://www.mgeko.cc/manga/solo-leveling-mg1/">
			</head><body>
				<h1><a href="/manga/solo-leveling-mg1/">Solo Leveling</a></h1>
			</body></html>`,
			base: "https://www.mgeko.cc",
			want: "https://www.mgeko.cc/manga/solo-leveling-mg1/",
		},
		{
			name: "h1 link without /manga/ falls back to og:url",
			html: `<html><head>
				<meta property="og:url" content="https://www.mgeko.cc/manga/solo-leveling-mg1/">
			</head><body>
				<h1><a href="/author/some-author/">By Author</a></h1>
			</body></html>`,
			base: "https://www.mgeko.cc",
			want: "https://www.mgeko.cc/manga/solo-leveling-mg1/",
		},
		{
			name: "no URL found returns empty string",
			html: `<html><body>
				<h1>Solo Leveling</h1>
				<p>some content</p>
			</body></html>`,
			base: "https://www.mgeko.cc",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("failed to parse HTML: %v", err)
			}
			if got := mgekoExtractSeriesURL(doc, tt.base); got != tt.want {
				t.Errorf("mgekoExtractSeriesURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
