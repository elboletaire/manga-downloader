// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "strings"

// Chapter represents a manga chapter
type Chapter struct {
	// Title is the chapter title
	Title string
	// Number is the chapter number
	Number float64
	// SortOrder is a secondary sort key for chapters that share a Number. Some
	// sites split a single chapter into several same-numbered entries (e.g.
	// baozimh's "065 资格" / "065 资格（下）", or "062 校园（上/中/下）"); the
	// downloader sorts by Number with an unstable sort.Slice, so without a
	// deterministic tiebreak those parts' relative order is arbitrary - and can
	// even differ between the fetch-time sort and the post-download re-sort in
	// bundle mode. Grabbers set it to the parts' reading order (base, 上, 中, 下).
	SortOrder float64
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

// GetSortOrder returns the secondary sort key (0 for most chapters)
func (c Chapter) GetSortOrder() float64 {
	return c.SortOrder
}

// GetTitle returns the chapter title removing whitespace and newlines
func (c Chapter) GetTitle() string {
	title := strings.TrimSpace(c.Title)
	title = strings.ReplaceAll(title, "\n", " ")
	return title
}
