// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// toivoLiteEntryContentPages extracts page images from a Toivo Lite chapter
// post (plain <img src> in div.entry-content). Returns an error if the post
// contains "not available yet" text to avoid silently accepting a placeholder.
func toivoLiteEntryContentPages(doc *goquery.Document, chapterTitle string) ([]Page, error) {
	content := doc.Find("div.entry-content")

	if strings.Contains(strings.ToLower(content.Text()), "not available yet") {
		return nil, fmt.Errorf("chapter %s is not available yet", chapterTitle)
	}

	var pages []Page
	content.Find("img").Each(func(i int, s *goquery.Selection) {
		src := strings.TrimSpace(s.AttrOr("src", ""))
		if src == "" {
			return
		}
		pages = append(pages, Page{
			Number: int64(len(pages) + 1),
			URL:    src,
		})
	})

	if len(pages) == 0 {
		return nil, fmt.Errorf("no pages found for %s", chapterTitle)
	}

	return pages, nil
}
