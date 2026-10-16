// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/elboletaire/manga-downloader/ranges"
)

// errSeriesNotFound returns the standard error for a chapter URL that the
// grabber could not map to a series page.
func errSeriesNotFound(chapterURL string) error {
	return fmt.Errorf("couldn't find the series of chapter %s; try the series link instead", chapterURL)
}

// chapterPath parses rawURL and returns its lowercased host (www. stripped)
// and path (query and fragment dropped). Used by taiyo to match chapter paths
// transparently across www. variants and URLs with trailing query/fragment.
func chapterPath(rawURL string) (host, path string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	host = strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	path = u.Path
	return host, path, true
}

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
	// Group is the scanlation group (team) that made this release, empty when
	// the site doesn't say. Several groups are joined with ", ". Set it with
	// SetGroups, which keeps Groups in step
	Group string
	// Groups are the scanlation groups that made this release, one name each,
	// so a name that holds ", " stays whole. GetGroups falls back to splitting
	// Group when a grabber didn't set them
	Groups []string
	// ID is the source's own stable identifier for this release of the
	// chapter, empty when the grabber has none
	ID string
	// Volume is the volume the site files the chapter under, 0 when it
	// doesn't say (or says something that isn't a whole number)
	Volume int
	// Locked is set when the listing marks the chapter as locked (coins, early
	// access): a logged-out reader gets no pages, so it's failed unfetched.
	// Only grabbers whose listing says so set it
	Locked bool
	// Label is how ChapterLabel composes the label from Number and Title
	Label Label
}

// Page represents a chapter page
type Page struct {
	// Number is the page number
	Number int64
	// URL is the page URL
	URL string
	// Headers are extra headers this page is downloaded with, on top of the
	// default ones (e.g. a token a site mints per response and binds to its
	// own image URLs, which can't live in the shared http session without
	// chapters downloaded in parallel sending each other's token)
	Headers map[string]string
	// NoReferer downloads the page without the site's Referer, for image
	// hosts that refuse a request carrying one (their reader sets
	// referrerpolicy="no-referrer" on its images)
	NoReferer bool
	// RetryFlaky retries a 404 or 5xx several times, 1s apart, each with a
	// cache-busting query, for an origin that fails pages at random (a CDN
	// may even cache its 404)
	RetryFlaky bool
	// Transform, if non-nil, post-processes the raw downloaded page bytes
	// before they're packed (e.g. undoing a site's client-side image
	// scrambling). Errors are retried the same as a failed download.
	Transform func([]byte) ([]byte, error)
}

// GetNumber returns the chapter number
func (c Chapter) GetNumber() float64 {
	return c.Number
}

// GetGroup returns the scanlation group that made the chapter, "" when unknown
func (c Chapter) GetGroup() string {
	return c.Group
}

// GetGroups returns the scanlation groups that made the chapter, one name
// each: Groups when a grabber set them, else Group split on groupSeparator
func (c Chapter) GetGroups() []string {
	if len(c.Groups) > 0 {
		return c.Groups
	}

	return cleanGroups(strings.Split(c.Group, groupSeparator))
}

// SetGroups names the scanlation groups that made the release. Groups keeps
// the names apart, and Group is their display join. Blank and repeated names
// are dropped.
func (c *Chapter) SetGroups(names ...string) {
	c.Groups = cleanGroups(names)
	c.Group = joinGroups(names)
}

// GetVolume returns the chapter's volume, 0 when unknown
func (c Chapter) GetVolume() int {
	return c.Volume
}

// GetLocked reports whether the listing marks the chapter as locked
func (c Chapter) GetLocked() bool {
	return c.Locked
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
	// PagesCount drives the chapter progress bars; left at the pre-skip count,
	// they would end short of their total
	c.PagesCount = int64(len(kept))

	return skipped
}

// GetTitle returns the site's chapter title removing whitespace and newlines.
// Show ChapterLabel instead, which names a chapter the site gives no title.
func (c Chapter) GetTitle() string {
	return cleanTitle(c.Title)
}

// cleanTitle trims title and puts its newlines on one line
func cleanTitle(title string) string {
	return strings.ReplaceAll(strings.TrimSpace(title), "\n", " ")
}
