// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"errors"
	"fmt"
	"sync"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/fatih/color"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// progressConfig is what the terminal output depends on besides the events
type progressConfig struct {
	title        string
	bundle       bool
	bundleRange  string
	mangaLen     int
	chapterLen   int
	showLanguage bool
}

// progress draws core.Download's events as the progress bars and messages the
// CLI always printed. Events arrive from several goroutines, so the bars map
// and the bundle phase are guarded by mu.
type progress struct {
	p   *mpb.Progress
	cfg progressConfig

	blue *color.Color

	mu        sync.Mutex
	bars      map[*grabber.Chapter]*mpb.Bar
	bundleBar *mpb.Bar
	// volumes are the bars of a --volumes run, and chapterVolume the volume
	// each of its chapters goes into
	volumes       map[*core.Volume]*volumeBar
	chapterVolume map[*grabber.Chapter]*core.Volume
	// phase is the status shown by the bundle bar
	phase string
}

var _ core.Events = (*progress)(nil)

func newProgress(p *mpb.Progress, cfg progressConfig) *progress {
	return &progress{
		p:    p,
		cfg:  cfg,
		blue: color.New(color.FgBlue),
		bars: map[*grabber.Chapter]*mpb.Bar{},

		volumes:       map[*core.Volume]*volumeBar{},
		chapterVolume: map[*grabber.Chapter]*core.Volume{},
	}
}

// volumeScale is the total of a volume's bar, which shows the fraction
// VolumeProgress reports: a volume's page count isn't known until all of its
// chapters are fetched, and growing the total as they start pulled it back
const volumeScale = 1000

// volumeBar is the bar of one volume
type volumeBar struct {
	bar *mpb.Bar
	// archiving is set once the volume's archive is being written
	archiving bool
}

// volumeFor returns the bar of the volume c goes into, nil when c is packed
// on its own. Callers hold mu.
func (pr *progress) volumeFor(c *grabber.Chapter) *volumeBar {
	if v, ok := pr.chapterVolume[c]; ok {
		return pr.volumes[v]
	}
	return nil
}

// phaseText is read by the bundle bar's decorator from the render goroutine
func (pr *progress) phaseText() string {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	return pr.phase
}

func (pr *progress) setPhase(s string) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.phase = s
}

// barFor returns the bar a chapter's progress goes to: the shared bundle bar
// in bundle mode, the chapter's own bar otherwise. It's nil if the chapter
// never got one (its fetch, skip or filename failed).
func (pr *progress) barFor(c *grabber.Chapter) *mpb.Bar {
	if pr.cfg.bundle {
		return pr.bundleBar
	}
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if vb := pr.volumeFor(c); vb != nil {
		return vb.bar
	}
	return pr.bars[c]
}

// ownBar returns the chapter's own bar: nil in bundle mode, for a chapter in
// a volume, and for one that never got a bar
func (pr *progress) ownBar(c *grabber.Chapter) *mpb.Bar {
	if pr.cfg.bundle {
		return nil
	}
	pr.mu.Lock()
	defer pr.mu.Unlock()
	return pr.bars[c]
}

// BundleStart creates the single bar of a bundle, whose total is every page of
// every chapter, twice: once downloaded, once archived
func (pr *progress) BundleStart(pages int64) {
	pr.bundleBar = pr.p.AddBar(pages*2,
		mpb.PrependDecorators(
			// Dynamic status showing current chapter
			decor.Any(func(s decor.Statistics) string {
				return pr.blue.Sprintf("%-30s", pr.phaseText())
			}, decor.WCSyncWidthR),
			decor.CountersNoUnit("%d/%d", decor.WC{C: decor.DextraSpace}),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WC{W: 4}), // W: 4 to remove space before %
			// Status at the end to prevent shifting
			decor.Any(func(s decor.Statistics) string {
				if s.Current >= s.Total {
					return pr.blue.Sprintf(" bundling ")
				}
				return pr.blue.Sprintf(" downloading")
			}, decor.WC{W: 10}),
		),
	)
	pr.setPhase("Gathering info...")
}

func (pr *progress) FetchFailed(listed grabber.Filterable, err error) {
	red("- error fetching chapter %s: %s", listed.GetTitle(), err.Error())
}

func (pr *progress) Skipped(c *grabber.Chapter) {
	yellow("- skipping chapter %s: --skip-pages excludes all of its pages", c.GetTitle())
}

func (pr *progress) FilenameFailed(c *grabber.Chapter, err error) {
	red("- error creating filename for chapter %s: %s", c.GetTitle(), err.Error())
}

func (pr *progress) Started(c *grabber.Chapter) {
	if pr.cfg.bundle {
		pr.setPhase(fmt.Sprintf("Downloading %s", c.GetTitle()))
		return
	}

	pr.mu.Lock()
	vb := pr.volumeFor(c)
	pr.mu.Unlock()
	// a volume's bar moves with VolumeProgress
	if vb != nil {
		return
	}

	// For non-bundle mode, create a single bar per chapter that combines download + archive
	// (using the fetched chapter, whose title includes the chapter number)
	barTitle := chapterBarTitle(pr.cfg.title, c, pr.cfg.mangaLen, pr.cfg.chapterLen, pr.cfg.showLanguage)
	// Total steps = pages (download) + pages (archive)
	total := c.PagesCount * 2
	bar := pr.p.AddBar(total,
		mpb.PrependDecorators(
			decor.Name(barTitle, decor.WCSyncWidthR),
			decor.CountersNoUnit("%d/%d", decor.WC{C: decor.DextraSpace}),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WC{W: 4}), // W: 4 to remove space before %
			// Status at the end to prevent shifting
			decor.Any(func(s decor.Statistics) string {
				if s.Current >= total/2 {
					return pr.blue.Sprintf(" archiving ")
				}
				return pr.blue.Sprintf(" downloading") // Add space before status
			}, decor.WC{W: 10}),
		),
	)
	pr.mu.Lock()
	pr.bars[c] = bar
	pr.mu.Unlock()
}

func (pr *progress) Page(c *grabber.Chapter, idx int, err error) {
	if err != nil {
		red("- error downloading page %d: %s", idx+1, err.Error())
		return
	}
	// a volume's bar moves with VolumeProgress instead
	pr.mu.Lock()
	inVolume := pr.volumeFor(c) != nil
	pr.mu.Unlock()
	if inVolume {
		return
	}
	// Increment by 1 since we're processing one page at a time
	if bar := pr.barFor(c); bar != nil {
		bar.IncrBy(1)
	}
}

func (pr *progress) DownloadFailed(c *grabber.Chapter, err error) {
	red("- error downloading chapter %s: %s", c.GetTitle(), err.Error())
	// a volume's bar goes on with its other chapters
	if own := pr.ownBar(c); own != nil {
		own.Abort(false)
	}
}

func (pr *progress) Archived(c *grabber.Chapter) {
	pr.barFor(c).IncrBy(1) // Increment archive progress
}

func (pr *progress) Packed(c *grabber.Chapter, _ string, err error) {
	if err != nil {
		red(err.Error())
	}
	bar := pr.barFor(c)
	if !bar.Completed() {
		// the bar can no longer reach its total (packing failed or
		// pages were skipped); mark it done so p.Wait() won't hang
		bar.Abort(false)
	}
}

func (pr *progress) Downloaded(c *grabber.Chapter) {
	// a volume's bar moves with VolumeProgress instead
	if !pr.cfg.bundle {
		return
	}
	// For bundle mode, increment archive progress
	pr.bundleBar.IncrBy(int(c.PagesCount))
}

func (pr *progress) BundlePacking() {
	pr.setPhase(fmt.Sprintf("Creating bundle for chapters %s", pr.cfg.bundleRange))
}

func (pr *progress) BundleArchived(n int) {
	if pr.bundleBar != nil {
		pr.bundleBar.IncrBy(n)
	}
}

// finishBundle marks the bundle bar done if some chapters failed and left it
// short of its total, so p.Wait() won't hang
func (pr *progress) finishBundle() {
	if pr.bundleBar != nil && !pr.bundleBar.Completed() {
		pr.bundleBar.Abort(false)
	}
}

// VolumeStart adds the bar of a volume. It shows VolumeProgress's fraction,
// so it counts chapters rather than pages, which aren't known until the end.
func (pr *progress) VolumeStart(v *core.Volume) {
	vb := &volumeBar{}
	title := fmt.Sprintf("%s - %s:", truncateString(pr.cfg.title, pr.cfg.mangaLen), core.VolumeLabel(v.Number))
	vb.bar = pr.p.AddBar(volumeScale,
		mpb.PrependDecorators(
			decor.Name(title, decor.WCSyncWidthR),
			// where a chapter's bar has its page counter
			decor.Name(fmt.Sprintf("%d ch.", len(v.Chapters)), decor.WC{C: decor.DextraSpace}),
		),
		mpb.AppendDecorators(
			decor.Percentage(decor.WC{W: 4}),
			decor.Any(func(s decor.Statistics) string {
				pr.mu.Lock()
				archiving := vb.archiving
				pr.mu.Unlock()
				if archiving {
					return pr.blue.Sprintf(" archiving ")
				}
				return pr.blue.Sprintf(" downloading")
			}, decor.WC{W: 10}),
		),
	)
	pr.mu.Lock()
	pr.volumes[v] = vb
	pr.mu.Unlock()
}

func (pr *progress) VolumeChapter(v *core.Volume, c *grabber.Chapter) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.chapterVolume[c] = v
}

func (pr *progress) VolumePacking(v *core.Volume) {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if vb := pr.volumes[v]; vb != nil {
		vb.archiving = true
	}
}

func (pr *progress) VolumeProgress(v *core.Volume, fraction float64) {
	pr.mu.Lock()
	vb := pr.volumes[v]
	pr.mu.Unlock()
	if vb != nil {
		vb.bar.SetCurrent(int64(fraction * volumeScale))
	}
}

func (pr *progress) VolumePacked(v *core.Volume, name string, err error) {
	pr.mu.Lock()
	vb := pr.volumes[v]
	pr.mu.Unlock()
	if errors.Is(err, core.ErrSkippedVolume) {
		yellow("- skipping %s: %s", core.VolumeLabel(v.Number), err.Error())
		if vb != nil {
			vb.bar.Abort(false)
		}
		return
	}
	if err != nil {
		red("- error packing %s: %s", core.VolumeLabel(v.Number), err.Error())
		if vb != nil {
			vb.bar.Abort(false)
		}
		return
	}
	if vb != nil && !vb.bar.Completed() {
		vb.bar.SetCurrent(volumeScale)
	}
}
