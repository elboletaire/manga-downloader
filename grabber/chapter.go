// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"strings"

	"github.com/elboletaire/manga-downloader/ranges"
)

// Chapter represents a manga chapter
type Chapter struct {
	// Title is the chapter title
	Title string
	// Number is the chapter number
	Number float64
	// PagesCount is the number of pages in the chapter
	PagesCount int64
	// Pages is the list of pages in the chapter
	Pages []Page
	// Language is the chapter language
	Language string
}

// Page represents a chapter page
type Page struct {
	// Number is the page number
	Number int64
	// URL is the page URL
	URL string
	// Transform, if non-nil, post-processes the raw downloaded page bytes
	// before they're packed (e.g. undoing a site's client-side image
	// scrambling). Errors are retried the same as a failed download.
	Transform func([]byte) ([]byte, error)
}

// GetNumber returns the chapter number
func (c Chapter) GetNumber() float64 {
	return c.Number
}

// GetLanguage returns the chapter language
func (c Chapter) GetLanguage() string {
	return c.Language
}

// SkipPages drops the pages selected by rngs and returns how many it dropped.
// The bounds are 1-based positions in the chapter's own page list, negatives
// counting from the end (-1 being the last page), and they're resolved against
// this chapter alone, so -1 means each chapter's own last page.
//
// It's meant to run right after the chapter is fetched and *before* it's
// downloaded, which is the whole point of --skip-pages: the pages scanlation
// groups reserve for credits or ads are never worth fetching, and a page that
// 404s (tcbscans' trailing one, on occasion) otherwise fails the entire
// chapter with no way around it.
//
// Page.Number is deliberately left alone, so a download error still names the
// page the site itself numbers. The archive's own numbering stays contiguous
// regardless, as packer.namePages names pages by their slice index.
func (c *Chapter) SkipPages(rngs []ranges.Range) int {
	if len(rngs) == 0 || len(c.Pages) == 0 {
		return 0
	}

	// resolved against the pages actually listed, not PagesCount: a few
	// grabbers report a count the page list doesn't match, and it's the list
	// that gets downloaded
	skip := ranges.Resolve(rngs, len(c.Pages))
	if len(skip) == 0 {
		return 0
	}

	kept := make([]Page, 0, len(c.Pages)-len(skip))
	for i, page := range c.Pages {
		if skip[i+1] {
			continue
		}
		kept = append(kept, page)
	}

	skipped := len(c.Pages) - len(kept)
	c.Pages = kept
	// PagesCount drives the progress bar totals and the bundle's page budget;
	// left at the pre-skip count, every bar would end short of its total
	c.PagesCount = int64(len(kept))

	return skipped
}

// GetTitle returns the chapter title removing whitespace and newlines
func (c Chapter) GetTitle() string {
	title := strings.TrimSpace(c.Title)
	title = strings.ReplaceAll(title, "\n", " ")
	return title
}
