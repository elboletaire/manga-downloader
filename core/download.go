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

// The progress bars split each page's step in two: DownloadWeight once the
// page is fetched and ArchiveWeight once it's written to the archive. A bar
// fills 80% while downloading and 20% while archiving, which is why archiving
// is the small share even when it's slow (AVIF conversion).
const (
	DownloadWeight = 4
	ArchiveWeight  = 1
)

// Events receives the progress of Download, in the order it happens. Calls
// come from several goroutines at once (chapters download concurrently), so
// implementations must be safe for concurrent use. A chapter is identified by
// the *grabber.Chapter its fetch returned; a chapter whose fetch failed is
// identified by its listing entry instead. Page events may arrive after
// DownloadFailed for the same chapter, since pages still in flight finish.
type Events interface {
	// BundleStart is called in bundle mode, before any chapter is downloaded,
	// with the page total the bundle will go through; it's called again when
	// a chapter's download finds other pages, with the corrected total
	BundleStart(pages int64)
	// FetchFailed reports a chapter that couldn't be fetched; it's skipped
	FetchFailed(listed grabber.Filterable, err error)
	// Skipped reports a chapter whose pages were all excluded by --skip-pages
	Skipped(c *grabber.Chapter)
	// FilenameFailed reports a chapter whose filename couldn't be built
	FilenameFailed(c *grabber.Chapter, err error)
	// Fetching reports a chapter whose page list is about to be fetched, then
	// FetchFailed or Fetched follows. Bundle mode's totals pass doesn't report
	// it: the bundle's row already says it is gathering info.
	Fetching(listed grabber.Filterable)
	// Fetched pairs a listing entry reported by Fetching with the chapter its
	// fetch returned, which the later events identify it by
	Fetched(listed grabber.Filterable, c *grabber.Chapter)
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
	// downloaded. Its page total is only known as its chapters start, so
	// its progress comes from VolumeProgress rather than from page counts.
	VolumeStart(v *Volume)
	// VolumeChapter reports that the fetched chapter c goes into v. It comes
	// right after the fetch, before any other event about c.
	VolumeChapter(v *Volume, c *grabber.Chapter)
	// VolumePacking is called once every chapter of v is done (downloaded or
	// failed) and its archive is about to be written
	VolumePacking(v *Volume)
	// VolumeProgress reports how far along v is, from 0 to 1: each of its
	// chapters weighs the same in the download share (by its pages once it
	// starts, in full once it's done or failed), and the archive's pages make
	// up the rest. It never decreases, and the calls for one volume
	// arrive in order, so renderers can show it as is. It doesn't reach 1 for
	// a volume that fails; VolumePacked says how it ended.
	VolumeProgress(v *Volume, fraction float64)
	// VolumePacked reports the end of v, with the file it wrote and the error,
	// if any: a packing error, ErrEmptyVolume when none of its chapters could
	// be downloaded, ErrSkippedVolume when --skip-pages emptied all of them
	// (already reported by Skipped, so not a failure), or the context's error when the run was cancelled before
	// all of them started
	VolumePacked(v *Volume, name string, err error)
}

// NoEvents ignores every event. Embed it to implement only the events you need.
type NoEvents struct{}

func (NoEvents) BundleStart(int64)                            {}
func (NoEvents) FetchFailed(grabber.Filterable, error)        {}
func (NoEvents) Skipped(*grabber.Chapter)                     {}
func (NoEvents) FilenameFailed(*grabber.Chapter, error)       {}
func (NoEvents) Fetching(grabber.Filterable)                  {}
func (NoEvents) Fetched(grabber.Filterable, *grabber.Chapter) {}
func (NoEvents) Started(*grabber.Chapter)                     {}
func (NoEvents) Page(*grabber.Chapter, int, error)            {}
func (NoEvents) DownloadFailed(*grabber.Chapter, error)       {}
func (NoEvents) Archived(*grabber.Chapter)                    {}
func (NoEvents) Packed(*grabber.Chapter, string, error)       {}
func (NoEvents) Downloaded(*grabber.Chapter)                  {}
func (NoEvents) BundlePacking()                               {}
func (NoEvents) BundleArchived(int)                           {}
func (NoEvents) VolumeStart(*Volume)                          {}
func (NoEvents) VolumeChapter(*Volume, *grabber.Chapter)      {}
func (NoEvents) VolumePacking(*Volume)                        {}
func (NoEvents) VolumeProgress(*Volume, float64)              {}
func (NoEvents) VolumePacked(*Volume, string, error)          {}

// ErrEmptyVolume is the VolumePacked error of a volume none of whose chapters
// could be downloaded, which leaves nothing to pack
var ErrEmptyVolume = errors.New("none of its chapters could be downloaded")

// ErrLocked is the FetchFailed error of a chapter the listing marks as locked.
// It's reported without fetching the chapter, as a reader of it gets no pages
var ErrLocked = errors.New("locked chapter")

// ErrSkippedVolume is the VolumePacked error of a volume whose chapters were
// all skipped by --skip-pages: there's nothing to pack, and nothing failed
var ErrSkippedVolume = errors.New("--skip-pages excludes every page of its chapters")

// downloadShare and archiveShare are the parts of a volume's fraction that
// its downloads and its archive fill: 0.8 and 0.2 with the weights above
const (
	downloadShare = float64(DownloadWeight) / (DownloadWeight + ArchiveWeight)
	archiveShare  = float64(ArchiveWeight) / (DownloadWeight + ArchiveWeight)
)

// volumeRun tracks a volume through the download: it's packed by whichever
// of its chapters finishes last
type volumeRun struct {
	v  *Volume
	ev Events

	mu         sync.Mutex
	remaining  int
	downloaded grabber.Filterables
	// skipped counts the chapters --skip-pages emptied
	skipped int
	packed  bool

	// chapters is the progress of each started or finished chapter, by
	// listing entry. archived and archiveTotal are the archive's pages, the
	// total known once packing starts. reported is the last VolumeProgress.
	chapters     map[grabber.Filterable]*chapterProgress
	archived     int
	archiveTotal int
	reported     float64
}

// chapterProgress is a volume chapter's share of the download share
type chapterProgress struct {
	done, total int
	finished    bool
}

func (p *chapterProgress) fraction() float64 {
	switch {
	case p.finished:
		return 1
	case p.total <= 0:
		return 0
	}
	return float64(p.done) / float64(p.total)
}

func newVolumeRun(v *Volume, ev Events) *volumeRun {
	return &volumeRun{v: v, ev: ev, remaining: len(v.Chapters), chapters: map[grabber.Filterable]*chapterProgress{}}
}

// chapter returns the progress of a listed chapter. Callers hold mu.
func (r *volumeRun) chapter(listed grabber.Filterable) *chapterProgress {
	p := r.chapters[listed]
	if p == nil {
		p = &chapterProgress{}
		r.chapters[listed] = p
	}
	return p
}

// report sends VolumeProgress when the volume moved forward. It's called
// with mu held, which keeps one volume's reports in order: a page finishing
// in another goroutine can't overtake it with an older value.
func (r *volumeRun) report() {
	sum := 0.0
	for _, p := range r.chapters {
		sum += p.fraction()
	}
	f := sum / float64(len(r.v.Chapters)) * downloadShare
	if r.archiveTotal > 0 {
		f += float64(r.archived) / float64(r.archiveTotal) * archiveShare
	}
	// the map's order is random, so the same progress can sum to a value a
	// rounding error above the last one: that isn't a step forward
	if f > r.reported+1e-9 {
		r.reported = f
		r.ev.VolumeProgress(r.v, f)
	}
}

// start records the page count of a listed chapter about to download
func (r *volumeRun) start(listed grabber.Filterable, pages int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chapter(listed).total = pages
}

// page records one downloaded page of a listed chapter
func (r *volumeRun) page(listed grabber.Filterable) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p := r.chapter(listed); !p.finished && p.done < p.total {
		p.done++
		r.report()
	}
}

// finish records one of the volume's chapters as done, d being nil when it
// failed or was skipped, and reports whether it was the last one
func (r *volumeRun) finish(listed grabber.Filterable, d *packer.DownloadedChapter, skipped bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d != nil {
		r.downloaded = append(r.downloaded, d)
	}
	if skipped {
		r.skipped++
	}
	r.chapter(listed).finished = true
	r.report()
	r.remaining--
	if r.remaining == 0 {
		r.packed = true
		return true
	}
	return false
}

// packing sets the archive's page total, before its pages are written
func (r *volumeRun) packing(pages int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.archiveTotal = pages
}

// archive records one page written to the volume's archive
func (r *volumeRun) archive() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.archived < r.archiveTotal {
		r.archived++
		r.report()
	}
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
				r := newVolumeRun(v, ev)
				runs = append(runs, r)
				for _, chap := range v.Chapters {
					volumeOf[chap] = r
				}
			}
		}
	}

	// the bundle's page total and each chapter's share of it: a chapter's
	// download may find other pages, or fail, and recount corrects the share
	// under bundleMu, which keeps the BundleStart totals in order
	var bundleMu sync.Mutex
	var totalPages int64
	counted := map[grabber.Filterable]int64{}
	if bundle {
		// same skipping the download below applies, so the total matches
		// what actually gets downloaded. A failed fetch counts 0 here; it's
		// fetched again below, where the failure is reported
		for _, chap := range chapters {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if grabber.LockedOf(chap) {
				continue
			}
			chapter, err := s.Site.FetchChapter(chap)
			if err == nil && chapter != nil {
				chapter.SkipPages(skip)
				counted[chap] = int64(len(chapter.Pages))
				totalPages += counted[chap]
			}
		}
		ev.BundleStart(totalPages)
	}

	// recount sets chap's pages in the bundle total to pages, the count its
	// own download found. A failed chapter keeps its count: its share stays
	// in the total, which is how the bar shows the failure.
	recount := func(chap grabber.Filterable, pages int64) {
		bundleMu.Lock()
		defer bundleMu.Unlock()
		if pages == counted[chap] {
			return
		}
		totalPages += pages - counted[chap]
		counted[chap] = pages
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
			skippedAll := false
			if run != nil {
				defer func() {
					if run.finish(chap, kept, skippedAll) {
						s.packVolume(run, outDir, ev)
					}
				}()
			}

			// a listed lock is failed here, before the reader page is fetched
			if grabber.LockedOf(chap) {
				ev.FetchFailed(chap, ErrLocked)
				return
			}
			ev.Fetching(chap)
			chapter, err := s.Site.FetchChapter(chap)
			if err != nil {
				ev.FetchFailed(chap, err)
				return
			}
			fillVolume(chapter, chap)
			ev.Fetched(chap, chapter)
			if run != nil {
				ev.VolumeChapter(run.v, chapter)
			}

			// drop the excluded pages before anything is downloaded, so a page
			// that's only ads (or one that 404s) costs nothing and can't fail
			// the chapter
			skipped := chapter.SkipPages(skip)
			if bundle {
				recount(chap, int64(len(chapter.Pages)))
			}
			if skipped > 0 && len(chapter.Pages) == 0 {
				ev.Skipped(chapter)
				skippedAll = true
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
			if run != nil {
				run.start(chap, len(chapter.Pages))
			}

			files, err := downloader.FetchChapter(ctx, s.Site, chapter, func(_ int, idx int, err error) {
				ev.Page(chapter, idx, err)
				if run != nil && err == nil {
					run.page(chap)
				}
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
		err := ErrEmptyVolume
		if r.skipped == len(r.v.Chapters) {
			err = ErrSkippedVolume
		}
		ev.VolumePacked(r.v, "", err)
		return
	}
	downloaded := r.downloaded.SortByNumber()
	dc := make([]*packer.DownloadedChapter, len(downloaded))
	for i, d := range downloaded {
		dc[i] = d.(*packer.DownloadedChapter)
	}

	pages := 0
	for _, d := range dc {
		pages += len(d.Files)
	}
	r.packing(pages)

	ev.VolumePacking(r.v)
	name, err := packer.PackVolume(outDir, s.Site, r.v.Number, dc, func(_, _ int) {
		r.archive()
	})
	ev.VolumePacked(r.v, name, err)
}
