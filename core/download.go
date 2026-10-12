// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"context"
	"errors"
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

	// The volume events (--volumes) come on top of the chapter ones. A
	// chapter in a volume gets the same events as a bundled one (Started,
	// Page, Downloaded and the failures, never Archived or Packed), and is
	// told apart from the chapters packed on their own by VolumeChapter.

	// VolumeStart is called once per volume, in order, before any chapter is
	// downloaded. Its page total is only known as its chapters start.
	VolumeStart(v *Volume)
	// VolumeChapter reports that the fetched chapter c goes into v. It comes
	// right after the fetch, before any other event about c.
	VolumeChapter(v *Volume, c *grabber.Chapter)
	// VolumePacking is called once every chapter of v is done (downloaded or
	// failed) and its archive is about to be written
	VolumePacking(v *Volume)
	// VolumeArchived reports n pages written to v's archive
	VolumeArchived(v *Volume, n int)
	// VolumePacked reports the end of v, with the file it wrote and the error,
	// if any: a packing error, ErrEmptyVolume when none of its chapters could
	// be downloaded, or the context's error when the run was cancelled before
	// all of them started
	VolumePacked(v *Volume, name string, err error)
}

// NoEvents ignores every event. Embed it to implement only the events you need.
type NoEvents struct{}

func (NoEvents) BundleStart(int64)                       {}
func (NoEvents) FetchFailed(grabber.Filterable, error)   {}
func (NoEvents) Skipped(*grabber.Chapter)                {}
func (NoEvents) FilenameFailed(*grabber.Chapter, error)  {}
func (NoEvents) Started(*grabber.Chapter)                {}
func (NoEvents) Page(*grabber.Chapter, int, error)       {}
func (NoEvents) DownloadFailed(*grabber.Chapter, error)  {}
func (NoEvents) Archived(*grabber.Chapter)               {}
func (NoEvents) Packed(*grabber.Chapter, string, error)  {}
func (NoEvents) Downloaded(*grabber.Chapter)             {}
func (NoEvents) BundlePacking()                          {}
func (NoEvents) BundleArchived(int)                      {}
func (NoEvents) VolumeStart(*Volume)                     {}
func (NoEvents) VolumeChapter(*Volume, *grabber.Chapter) {}
func (NoEvents) VolumePacking(*Volume)                   {}
func (NoEvents) VolumeArchived(*Volume, int)             {}
func (NoEvents) VolumePacked(*Volume, string, error)     {}

// ErrEmptyVolume is the VolumePacked error of a volume none of whose chapters
// could be downloaded, which leaves nothing to pack
var ErrEmptyVolume = errors.New("none of its chapters could be downloaded")

// volumeRun tracks a volume through the download: it's packed by whichever
// of its chapters finishes last
type volumeRun struct {
	v *Volume

	mu         sync.Mutex
	remaining  int
	downloaded grabber.Filterables
	packed     bool
}

// finish records one of the volume's chapters as done, d being nil when it
// failed or was skipped, and reports whether it was the last one
func (r *volumeRun) finish(d *packer.DownloadedChapter) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d != nil {
		r.downloaded = append(r.downloaded, d)
	}
	r.remaining--
	if r.remaining == 0 {
		r.packed = true
		return true
	}
	return false
}

// Download fetches, downloads and packs the given chapters, which must already
// be filtered to the requested ranges. skip holds the --skip-pages ranges and
// rng the range text the bundle is named after. Output goes to the settings'
// output directory; bundle mode and the format come from the settings too.
//
// With the settings' Volumes, the chapters are grouped into volumes (see
// GroupVolumes), each packed into one file as soon as all of its chapters are
// done; the chapters outside every volume are packed one file per chapter.
//
// It returns the bundle's filename in bundle mode and an empty string
// otherwise. Chapter failures are reported through ev and don't stop the
// other chapters; only the bundle's own packing error is returned, or an
// invalid Volumes, before anything is fetched. Cancelling ctx stops starting
// new chapters and returns the context's error once the started ones have
// finished.
func (s *Series) Download(ctx context.Context, chapters grabber.Filterables, skip []ranges.Range, rng string, ev Events) (string, error) {
	if ev == nil {
		ev = NoEvents{}
	}
	// settings are read once, here, rather than from the goroutines below
	bundle := s.settings.Bundle
	outDir := s.settings.OutputDir

	// the volume each chapter goes into, nil when packed on its own
	var volumeOf map[grabber.Filterable]*volumeRun
	var runs []*volumeRun
	if !bundle {
		spec, err := ParseVolumes(s.settings.Volumes)
		if err != nil {
			return "", &SettingError{Setting: "volumes", Value: s.settings.Volumes, Err: err}
		}
		if spec != nil {
			volumes, _, err := GroupVolumes(chapters, spec)
			if err != nil {
				return "", &SettingError{Setting: "volumes", Value: s.settings.Volumes, Err: err}
			}
			// checked once, before anything is downloaded: unlike a chapter's,
			// a volume's name failing would waste every chapter in it
			templ := packer.VolumeTemplate(s.Site.GetFilenameTemplate())
			if _, err := packer.NewFilenameFromTemplate(templ, packer.FilenameTemplateParts{Series: s.Title, Number: "1", Title: VolumeLabel(1), Volume: 1}); err != nil {
				return "", &SettingError{Setting: "filename-template", Value: templ, Err: err}
			}
			volumeOf = map[grabber.Filterable]*volumeRun{}
			for _, v := range volumes {
				r := &volumeRun{v: v, remaining: len(v.Chapters)}
				runs = append(runs, r)
				for _, chap := range v.Chapters {
					volumeOf[chap] = r
				}
			}
		}
	}

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

	for _, r := range runs {
		ev.VolumeStart(r.v)
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

			run := volumeOf[chap]
			// what the chapter leaves for its volume, nil unless downloaded
			var kept *packer.DownloadedChapter
			if run != nil {
				defer func() {
					if run.finish(kept) {
						s.packVolume(run, outDir, ev)
					}
				}()
			}

			chapter, err := s.Site.FetchChapter(chap)
			if err != nil {
				ev.FetchFailed(chap, err)
				return
			}
			fillVolume(chapter, chap)
			if run != nil {
				ev.VolumeChapter(run.v, chapter)
			}

			// drop the excluded pages before anything is downloaded, so a page
			// that's only ads (or one that 404s) costs nothing and can't fail
			// the chapter
			if skipped := chapter.SkipPages(skip); skipped > 0 && len(chapter.Pages) == 0 {
				ev.Skipped(chapter)
				return
			}

			// the name itself is only needed by the packer, which builds it
			// again; this is the check that the template works for the
			// chapter. A volume's chapters are named by their volume instead,
			// whose template was checked up front.
			if run == nil {
				if _, err := packer.NewFilenameFromTemplate(s.Site.GetFilenameTemplate(), packer.NewChapterFileTemplateParts(s.Title, chapter)); err != nil {
					ev.FilenameFailed(chapter, err)
					return
				}
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

			switch {
			case run != nil:
				ev.Downloaded(chapter)
				kept = d
			case !bundle:
				name, err := packer.PackSingle(outDir, s.Site, d, func(_, _ int) {
					ev.Archived(chapter)
				})
				ev.Packed(chapter, name, err)
			default:
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

	// a volume some of whose chapters never started (the run was cancelled)
	// isn't packed: it would silently miss them
	for _, r := range runs {
		if !r.packed {
			ev.VolumePacked(r.v, "", ctx.Err())
		}
	}

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

// fillVolume gives a fetched chapter the volume its listing entry has, as
// most grabbers build the fetched chapter afresh, without one
func fillVolume(chapter *grabber.Chapter, listed grabber.Filterable) {
	if chapter.Volume == 0 {
		chapter.Volume = grabber.VolumeOf(listed)
	}
}

// packVolume packs a volume whose chapters are all done, or reports it empty
func (s *Series) packVolume(r *volumeRun, outDir string, ev Events) {
	if len(r.downloaded) == 0 {
		ev.VolumePacked(r.v, "", ErrEmptyVolume)
		return
	}
	downloaded := r.downloaded.SortByNumber()
	dc := make([]*packer.DownloadedChapter, len(downloaded))
	for i, d := range downloaded {
		dc[i] = d.(*packer.DownloadedChapter)
	}

	ev.VolumePacking(r.v)
	name, err := packer.PackVolume(outDir, s.Site, r.v.Number, dc, func(page, _ int) {
		ev.VolumeArchived(r.v, page)
	})
	ev.VolumePacked(r.v, name, err)
}
