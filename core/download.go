// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"context"
	"sync"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/ranges"
)

// Events receives the progress of Download, in the order it happens. Calls
// come from several goroutines at once (chapters download concurrently), so
// implementations must be safe for concurrent use. A chapter is identified by
// the *grabber.Chapter its fetch returned; a chapter whose fetch failed is
// identified by its listing entry instead. Page events may arrive after
// DownloadFailed for the same chapter, since pages still in flight finish.
type Events interface {
	// BundleStart is called once in bundle mode, before any chapter is
	// downloaded, with the page total the bundle will go through
	BundleStart(pages int64)
	// FetchFailed reports a chapter that couldn't be fetched; it's skipped
	FetchFailed(listed grabber.Filterable, err error)
	// Skipped reports a chapter whose pages were all excluded by --skip-pages
	Skipped(c *grabber.Chapter)
	// FilenameFailed reports a chapter whose filename couldn't be built
	FilenameFailed(c *grabber.Chapter, err error)
	// Started reports a chapter about to download its pages
	Started(c *grabber.Chapter)
	// Page reports the outcome of one page: err is nil on success, and idx is
	// the page's index in c.Pages
	Page(c *grabber.Chapter, idx int, err error)
	// DownloadFailed reports a chapter whose download was aborted
	DownloadFailed(c *grabber.Chapter, err error)
	// Archived reports one page written to the chapter's archive (not bundle mode)
	Archived(c *grabber.Chapter)
	// Packed reports the end of a chapter's packing (not bundle mode), with
	// the file it wrote and the error, if any
	Packed(c *grabber.Chapter, name string, err error)
	// Downloaded reports a chapter kept for the bundle (bundle mode)
	Downloaded(c *grabber.Chapter)
	// BundlePacking is called once all chapters are downloaded and the
	// bundle archive is about to be written (bundle mode)
	BundlePacking()
	// BundleArchived reports n pages written to the bundle archive (bundle mode)
	BundleArchived(n int)
}

// NoEvents ignores every event. Embed it to implement only the events you need.
type NoEvents struct{}

func (NoEvents) BundleStart(int64)                      {}
func (NoEvents) FetchFailed(grabber.Filterable, error)  {}
func (NoEvents) Skipped(*grabber.Chapter)               {}
func (NoEvents) FilenameFailed(*grabber.Chapter, error) {}
func (NoEvents) Started(*grabber.Chapter)               {}
func (NoEvents) Page(*grabber.Chapter, int, error)      {}
func (NoEvents) DownloadFailed(*grabber.Chapter, error) {}
func (NoEvents) Archived(*grabber.Chapter)              {}
func (NoEvents) Packed(*grabber.Chapter, string, error) {}
func (NoEvents) Downloaded(*grabber.Chapter)            {}
func (NoEvents) BundlePacking()                         {}
func (NoEvents) BundleArchived(int)                     {}

// Download fetches, downloads and packs the given chapters, which must already
// be filtered to the requested ranges. skip holds the --skip-pages ranges and
// rng the range text the bundle is named after. Output goes to the settings'
// output directory; bundle mode and the format come from the settings too.
//
// It returns the bundle's filename in bundle mode and an empty string
// otherwise. Chapter failures are reported through ev and don't stop the
// other chapters; only the bundle's own packing error is returned. Cancelling
// ctx stops starting new chapters and returns the context's error once the
// started ones have finished.
func (s *Series) Download(ctx context.Context, chapters grabber.Filterables, skip []ranges.Range, rng string, ev Events) (string, error) {
	if ev == nil {
		ev = NoEvents{}
	}
	// settings are read once, here, rather than from the goroutines below
	bundle := s.settings.Bundle
	outDir := s.settings.OutputDir

	if bundle {
		// same skipping the download below applies, so the total matches
		// what actually gets downloaded. Errors are ignored here: the
		// chapter is fetched again below, where they're reported
		totalPages := int64(0)
		for _, chap := range chapters {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			chapter, err := s.Site.FetchChapter(chap)
			if err == nil && chapter != nil {
				chapter.SkipPages(skip)
				totalPages += chapter.PagesCount
			}
		}
		ev.BundleStart(totalPages)
	}

	wg := sync.WaitGroup{}
	g := make(chan struct{}, s.Site.GetMaxConcurrency().Chapters)
	// downloaded is appended to from the chapter goroutines
	var mu sync.Mutex
	downloaded := grabber.Filterables{}

loop:
	for _, chap := range chapters {
		// checked first: select picks at random when both cases are ready,
		// which could start one more chapter after a cancellation
		if ctx.Err() != nil {
			break loop
		}
		select {
		case <-ctx.Done():
			break loop
		case g <- struct{}{}:
		}
		wg.Add(1)

		go func(chap grabber.Filterable) {
			defer wg.Done()
			defer func() { <-g }()

			chapter, err := s.Site.FetchChapter(chap)
			if err != nil {
				ev.FetchFailed(chap, err)
				return
			}

			// drop the excluded pages before anything is downloaded, so a page
			// that's only ads (or one that 404s) costs nothing and can't fail
			// the chapter
			if skipped := chapter.SkipPages(skip); skipped > 0 && len(chapter.Pages) == 0 {
				ev.Skipped(chapter)
				return
			}

			// the name itself is only needed by the packer, which builds it
			// again; this is the check that the template works for the chapter
			if _, err := packer.NewFilenameFromTemplate(s.Site.GetFilenameTemplate(), packer.NewChapterFileTemplateParts(s.Title, chapter)); err != nil {
				ev.FilenameFailed(chapter, err)
				return
			}

			ev.Started(chapter)

			files, err := downloader.FetchChapter(s.Site, chapter, func(_ int, idx int, err error) {
				ev.Page(chapter, idx, err)
			})
			if err != nil {
				ev.DownloadFailed(chapter, err)
				return
			}

			d := &packer.DownloadedChapter{
				Chapter: chapter,
				Files:   files,
			}

			if !bundle {
				name, err := packer.PackSingle(outDir, s.Site, d, func(_, _ int) {
					ev.Archived(chapter)
				})
				ev.Packed(chapter, name, err)
			} else {
				ev.Downloaded(chapter)
				// kept in memory only for the bundle, which is packed at the end
				mu.Lock()
				downloaded = append(downloaded, d)
				mu.Unlock()
			}
		}(chap)
	}
	// wait for all routines to finish
	wg.Wait()

	// a cancelled run doesn't pack what it has: a bundle would silently miss
	// the chapters that never started
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !bundle {
		return "", nil
	}

	// resort downloaded
	downloaded = downloaded.SortByNumber()

	dc := []*packer.DownloadedChapter{}
	// convert slice back to DownloadedChapter
	for _, d := range downloaded {
		dc = append(dc, d.(*packer.DownloadedChapter))
	}

	ev.BundlePacking()
	return packer.PackBundle(outDir, s.Site, dc, rng, func(page, _ int) {
		ev.BundleArchived(page)
	})
}
