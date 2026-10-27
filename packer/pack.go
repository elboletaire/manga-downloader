// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
)

// Supported output formats (see grabber.Settings.Format)
const (
	FormatCBZ  = "cbz"
	FormatCBR  = "cbr"
	FormatRaw  = "raw"
	FormatEPUB = "epub"
)

// DownloadedChapter represents a downloaded chapter (a Chapter + Files)
type DownloadedChapter struct {
	*grabber.Chapter
	Files []*downloader.File
}

// PackSingle packs a single downloaded chapter
func PackSingle(outputdir string, s grabber.Site, chapter *DownloadedChapter, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	format := s.GetFormat()
	var info EPUBInfo
	if format == FormatEPUB {
		info = EPUBInfo{
			Series:           title,
			Position:         formatChapterNumber(chapter.GetNumber()),
			Identity:         "chapter " + chapterKey(chapter.Chapter),
			ReadingDirection: s.GetReadingDirection(),
			Labels:           map[string]string{epubSingleFolder: chapterLabel(chapter.Chapter)},
		}
	}
	always, whenSmaller := pageFormats(format, s.GetConvertImages(), s.GetCompress())
	files := namePages(chapter.Files, always, whenSmaller, s.GetJPEGQuality(), format)
	f := chapterFile(title, s.GetFilenameTemplate(), chapter.Chapter)
	return pack(outputdir, format, f.template, releaseName(chapter.Chapter), f.parts, files, info, progress)
}

// PackBundle packs a bundle of downloaded chapters, grouping each chapter's
// pages into its own folder inside the archive (Chapter 0001/000.jpg, ...)
// so chapter boundaries survive bundling instead of a single flat renumbering.
func PackBundle(outputdir string, s grabber.Site, chapters []*DownloadedChapter, rng string, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	format := s.GetFormat()
	files, labels := chapterFolders(chapters, format, s)
	var info EPUBInfo
	if format == FormatEPUB {
		info = EPUBInfo{Series: title, Identity: "bundle " + memberKeys(chapters), ReadingDirection: s.GetReadingDirection(), Labels: labels}
	}
	f := bundleFile(title, s.GetFilenameTemplate(), rng, chaptersOf(chapters))
	return pack(outputdir, format, f.template, "the bundle", f.parts, files, info, progress)
}

// PackVolume packs the downloaded chapters of a volume into one file, laid out as a
// bundle is, and named with VolumeTemplate. rng is the name's range, see
// NewVolumeFileTemplateParts; chapters must be sorted by number.
func PackVolume(outputdir string, s grabber.Site, volume int, rng string, chapters []*DownloadedChapter, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	format := s.GetFormat()
	files, labels := chapterFolders(chapters, format, s)
	var info EPUBInfo
	if format == FormatEPUB {
		info = EPUBInfo{
			Series:           title,
			Position:         strconv.Itoa(volume),
			Identity:         "volume " + strconv.Itoa(volume) + " " + memberKeys(chapters),
			ReadingDirection: s.GetReadingDirection(),
			Labels:           labels,
		}
	}
	f := volumeFile(title, s.GetFilenameTemplate(), volume, rng, chaptersOf(chapters))
	return pack(outputdir, format, f.template, "volume "+strconv.Itoa(volume), f.parts, files, info, progress)
}

// PackParts packs a chapter's parts, in the order given (their part order), into
// one file named as the chapter without its part.
func PackParts(outputdir string, s grabber.Site, chapters []*DownloadedChapter, progress func(page, progress int)) (string, error) {
	title, _ := s.FetchTitle()
	format := s.GetFormat()
	files, labels := chapterFolders(chapters, format, s)
	number := formatChapterNumber(chapters[0].GetNumber())
	var info EPUBInfo
	if format == FormatEPUB {
		info = EPUBInfo{
			Series:           title,
			Position:         number,
			Identity:         "parts " + memberKeys(chapters),
			ReadingDirection: s.GetReadingDirection(),
			Labels:           labels,
		}
	}
	f := partsFile(title, s.GetFilenameTemplate(), chaptersOf(chapters))
	return pack(outputdir, format, f.template, "chapter "+number, f.parts, files, info, progress)
}

// chaptersOf returns the chapters of downloaded ones, in the same order
func chaptersOf(chapters []*DownloadedChapter) []*grabber.Chapter {
	listed := make([]*grabber.Chapter, len(chapters))
	for i, c := range chapters {
		listed[i] = c.Chapter
	}
	return listed
}

// namedFile is how a file is named: the template it renders and the parts it
// renders with. The Pack* functions and the name helpers share it, so a name
// given before packing is the one packing gives.
type namedFile struct {
	template string
	parts    FilenameTemplateParts
}

// chapterFile names a chapter's file, as PackSingle does
func chapterFile(title, template string, c *grabber.Chapter) namedFile {
	return namedFile{template, NewChapterFileTemplateParts(title, c)}
}

// volumeFile names a volume's file, with VolumeTemplate as PackVolume does
func volumeFile(title, template string, volume int, rng string, chapters []*grabber.Chapter) namedFile {
	return namedFile{VolumeTemplate(template), NewVolumeFileTemplateParts(title, volume, rng, chapters)}
}

// partsFile names a chapter's merged parts with the chapter's template, as PackParts does
func partsFile(title, template string, chapters []*grabber.Chapter) namedFile {
	chapter := mergedChapter(chapters)
	return namedFile{template, NewChapterFileTemplateParts(title, &chapter)}
}

// bundleFile names a bundle's file, as PackBundle does
func bundleFile(title, template, rng string, chapters []*grabber.Chapter) namedFile {
	return namedFile{template, FilenameTemplateParts{
		Series: title,
		Number: rng,
		Title:  "bundle",
		Group:  sharedGroup(chapters),
	}}
}

// ChapterName returns the name PackSingle gives a chapter's first try, with its
// extension and without a " v2" suffix. It fails when the template does.
func ChapterName(title, template, format string, c *grabber.Chapter) (string, error) {
	f := chapterFile(title, template, c)
	return FirstName(format, f.template, f.parts)
}

// VolumeName returns the name PackVolume gives a volume's first try, as
// ChapterName does
func VolumeName(title, template, format string, volume int, rng string, chapters []*grabber.Chapter) (string, error) {
	f := volumeFile(title, template, volume, rng, chapters)
	return FirstName(format, f.template, f.parts)
}

// PartsName returns the name PackParts gives a chapter's parts' first try, as
// ChapterName does
func PartsName(title, template, format string, chapters []*grabber.Chapter) (string, error) {
	f := partsFile(title, template, chapters)
	return FirstName(format, f.template, f.parts)
}

// BundleName returns the name PackBundle gives a bundle's first try, as
// ChapterName does
func BundleName(title, template, format, rng string, chapters []*grabber.Chapter) (string, error) {
	f := bundleFile(title, template, rng, chapters)
	return FirstName(format, f.template, f.parts)
}

// FirstName returns the output name of a file's first try, the one without a
// " v2" suffix, with its extension (none for raw output)
func FirstName(format, template string, parts FilenameTemplateParts) (string, error) {
	filename, err := renderName(template, parts, 1)
	if err != nil {
		return "", err
	}
	return outputName(format, filename), nil
}

// Exists reports whether name is already in outDir: a file, or the folder of
// raw output
func Exists(outDir, name string) bool {
	_, err := os.Stat(filepath.Join(outDir, name))
	return err == nil
}

// renderName returns the filename a template gives the try-th attempt at a
// file, without its extension
func renderName(template string, parts FilenameTemplateParts, attempt int) (string, error) {
	parts.Version = attempt
	return NewFilenameFromTemplate(template, parts)
}

// outputName adds the extension of format to a filename: none for raw output,
// which is a folder
func outputName(format, filename string) string {
	switch format {
	case FormatRaw:
		return filename
	case FormatEPUB:
		return filename + ".epub"
	case FormatCBR:
		return filename + ".cbr"
	default:
		return filename + ".cbz"
	}
}

// mergedChapter is the chapter a merged file is named after: the parts' shared
// title without their parts, else the chapter's number alone, as a numbered chapter
func mergedChapter(chapters []*grabber.Chapter) grabber.Chapter {
	parts := make(grabber.Filterables, len(chapters))
	for i, c := range chapters {
		parts[i] = c
	}
	chapter := *chapters[0]
	chapter.Title = grabber.PartsTitle(parts)
	chapter.Label.Part = ""
	chapter.Label.Style = grabber.LabelNumbered
	return chapter
}

// releaseName names a chapter's release for messages: its chapter name, then
// its group in brackets when it has one, so two releases of a number can be told apart.
func releaseName(c *grabber.Chapter) string {
	if c.Group == "" {
		return grabber.ChapterName(c)
	}
	return grabber.ChapterName(c) + " [" + c.Group + "]"
}

// compressFormats are the source formats --compress re-encodes. GIF is left
// out: re-encoding an animated one keeps only its first frame.
var compressFormats = []string{"jpg", "png", "webp", "avif"}

// pageFormats returns the source formats to convert pages from, in two sets:
// always (--convert-images, and EPUB's AVIF) are converted whatever their size,
// whenSmaller (--compress) only when the JPEG isn't bigger. A format in both is always converted.
func pageFormats(format string, formats grabber.ConvertFormats, compress bool) (always, whenSmaller grabber.ConvertFormats) {
	always = grabber.ConvertFormats{}
	for f := range formats {
		always[f] = true
	}
	if format == FormatEPUB {
		always[grabber.ConvertAVIF] = true
	}

	whenSmaller = grabber.ConvertFormats{}
	if compress {
		for _, f := range compressFormats {
			if !always[f] {
				whenSmaller[f] = true
			}
		}
	}
	return always, whenSmaller
}

// chapterKey tells a chapter's releases apart: number, language and chapter ID, or else
// the group and title. Group-less keys keep their old form, so EPUB ids stay stable.
func chapterKey(chapter *grabber.Chapter) string {
	release := chapter.ID
	if release == "" {
		release = grabber.ChapterLabel(chapter)
		if chapter.Group != "" {
			release = chapter.Group + "\x00" + release
		}
	}
	return formatChapterNumber(chapter.GetNumber()) + "\x00" + chapter.Language + "\x00" + release
}

// memberKeys returns the chapterKeys of chapters, sorted by number and then by
// key, joined by \x01, so the same chapters give the same keys in any order
func memberKeys(chapters []*DownloadedChapter) string {
	type member struct {
		number float64
		key    string
	}
	members := make([]member, len(chapters))
	for i, c := range chapters {
		members[i] = member{c.GetNumber(), chapterKey(c.Chapter)}
	}
	sort.Slice(members, func(i, j int) bool {
		if members[i].number != members[j].number {
			return members[i].number < members[j].number
		}
		return members[i].key < members[j].key
	})
	keys := make([]string, len(members))
	for i, m := range members {
		keys[i] = m.key
	}
	return strings.Join(keys, "\x01")
}

// chapterFolders names the pages of several chapters for one archive, each
// chapter in its own folder (Chapter 0001/000.jpg, ...). It also returns the
// chapters' labels by folder, for the EPUB table of contents.
func chapterFolders(chapters []*DownloadedChapter, format string, s grabber.Site) ([]File, map[string]string) {
	files := []File{}
	labels := map[string]string{}
	always, whenSmaller := pageFormats(format, s.GetConvertImages(), s.GetCompress())
	// Several chapters can share a number (a site re-releasing the same chapter,
	// or two genuinely distinct chapters both numbered, e.g. a pair of
	// season-finale entries). Each chapter gets its own "Chapter 0001/" folder
	// so chapter boundaries survive bundling, so repeated numbers are
	// disambiguated with a " (2)", " (3)" suffix instead of silently merging
	// both chapters' pages into one folder (which would lose the first one's).
	usedFolders := map[string]int{}
	usedLabels := map[string]int{}
	for _, chapter := range chapters {
		base := SanitizeFilename(grabber.NumberTextLabel(paddedChapterNumber(chapter.GetNumber())))
		folder := base
		if n := usedFolders[base]; n > 0 {
			folder = fmt.Sprintf("%s (%d)", base, n+1)
		}
		usedFolders[base]++
		// only a repeated label gets the folder's suffix, as parts already differ by theirs
		label := chapterLabel(chapter.Chapter)
		if usedLabels[label] > 0 {
			label += strings.TrimPrefix(folder, base)
		}
		usedLabels[label]++
		labels[folder] = label

		for _, page := range namePages(chapter.Files, always, whenSmaller, s.GetJPEGQuality(), format) {
			files = append(files, File{
				Name: fmt.Sprintf("%s/%s", folder, page.Name),
				Data: page.Data,
			})
		}
	}

	return files, labels
}

// chapterLabel names a chapter the way the filename template does (no padding,
// decimals kept), with its title when the title says more than the number
func chapterLabel(chapter *grabber.Chapter) string {
	number := chapter.GetNumber()
	label := grabber.NumberLabel(number)
	if title := titleBeyondNumber(grabber.ChapterLabel(chapter), number); title != "" {
		label += " - " + title
	}
	return label
}

// titleBeyondNumber returns title without the chapter number it opens with,
// which the label already gives ("Chapter 0005: Foo" is "Foo"). It's empty when
// nothing else is left, as for "Ch. 5" or a title that's only the number.
func titleBeyondNumber(title string, number float64) string {
	if m := grabber.ChapterTitlePrefix.FindStringSubmatch(title); m != nil {
		if n, err := strconv.ParseFloat(m[1], 64); err == nil && n == number {
			title = title[len(m[0]):]
		}
	}
	title = strings.TrimSpace(title)
	if title == formatChapterNumber(number) {
		return ""
	}
	return title
}

// namePages names a chapter's pages sequentially (000.jpg, 001.png, ...),
// restarting at 000 for each call, with extensions detected from the image
// bytes. Pages in always or whenSmaller (see pageFormats) are transcoded to
// JPEG at quality first, so e-readers that can't render AVIF (or WebP) still
// get a readable archive. format is the output format: an EPUB leaves out the
// pages it can't show, and ArchiveEPUB warns about each of them.
func namePages(pages []*downloader.File, always, whenSmaller grabber.ConvertFormats, quality int, format string) []File {
	named := make([]File, len(pages))
	for i, page := range pages {
		data, ext := page.Data, extFromContent(page.Data)

		if always.Has(ext) || whenSmaller.Has(ext) {
			converted, err := convertToJPEG(data, quality)
			switch {
			case err != nil:
				// keep both the original bytes and the original extension:
				// aborting would lose the whole chapter (the whole bundle, when
				// bundling) over a single bad page, and naming undecodable
				// bytes .jpg would only make the failure more confusing
				_, shown := epubMediaTypes[ext]
				if format != FormatEPUB || shown {
					notice.Warn("- warning: page %03d: converting %s to jpeg: %s (keeping the original)", i, ext, err)
				}
			case !always.Has(ext) && len(converted) >= len(data):
				// a JPEG that isn't smaller would only cost quality, so the original stays
			default:
				data, ext = converted, "jpg"
			}
		}

		named[i] = File{
			Name: fmt.Sprintf("%03d.%s", i, ext),
			Data: data,
		}
	}
	return named
}

// maxPackAttempts caps the names pack tries for one file, so a name that
// keeps colliding fails instead of looping forever holding a download slot
const maxPackAttempts = 1000

func pack(outputdir, format, template, label string, parts FilenameTemplateParts, files []File, info EPUBInfo, progress func(page, progress int)) (string, error) {
	// base is the name the template gives the first try. A template that
	// doesn't use {{.Version}} gives that same name every time, so the later
	// tries get the " v2"-style suffix here, as the default template would
	var base string
	for attempt := 1; ; attempt++ {
		if attempt > maxPackAttempts {
			return "", fmt.Errorf("- error saving file for %s: no free name after %d tries", label, maxPackAttempts)
		}
		filename, err := renderName(template, parts, attempt)
		if err != nil {
			return "", fmt.Errorf("- error creating filename for %s: %s", label, err.Error())
		}
		if attempt == 1 {
			base = filename
		} else if filename == base {
			filename = fmt.Sprintf("%s v%d", base, attempt)
		}

		name := outputName(format, filename)
		if format == FormatRaw {
			err = SaveRaw(filepath.Join(outputdir, name), files, progress)
		} else if format == FormatEPUB {
			info.Title = filename
			err = ArchiveEPUB(filepath.Join(outputdir, name), info, files, progress)
		} else if format == FormatCBR {
			err = ArchiveCBR(filepath.Join(outputdir, name), files, progress)
		} else {
			err = ArchiveCBZ(filepath.Join(outputdir, name), files, progress)
		}

		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("- error saving file %s: %s", name, err.Error())
		}

		return name, nil
	}
}
