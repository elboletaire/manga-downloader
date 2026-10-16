// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/elboletaire/manga-downloader/grabber"
)

// FilenameTemplateParts represents the parts of a filename
type FilenameTemplateParts struct {
	// Series represents the series name (e.g. "One Piece")
	Series string
	// Number represents the chapter number (e.g. "1.0")
	Number string
	// Title is the chapter's label (e.g. "The Beginning", or "Chapter 12" when
	// the site gives no title), see grabber.ChapterLabel
	Title string
	// SiteTitle is the site's own chapter title, empty when it gives none or
	// for a volume or bundle
	SiteTitle string
	// Group is the scanlation group that made the release (e.g. "Alpha"). For a
	// volume or a bundle, the group its chapters share, empty when they differ
	// or name none. Several groups of one release are joined with ", "
	Group string
	// Volume is the volume number: the chapter's own (0 when the site doesn't
	// say) or, for a volume file, the volume's (e.g. 3). It's an int, so
	// {{printf "%02d" .Volume}} pads it.
	Volume int
	// Version placeholder appended to the title in case of duplicate filenames (e.g. "3")
	Version int
}

// FilenameTemplateDefault is the default filename template
const FilenameTemplateDefault = "{{.Series}} {{.Number}} - {{.Title}}{{if .Group}} [{{.Group}}]{{end}}{{if gt .Version 1}} v{{.Version}}{{end}}"

// filenameTemplateLegacyDefault is the default before {{.Group}} was added. It's
// still saved in gui.json files, so it's still treated as the default
const filenameTemplateLegacyDefault = "{{.Series}} {{.Number}} - {{.Title}}{{if gt .Version 1}} v{{.Version}}{{end}}"

// FilenameVolumeTemplateDefault is the template volume files (--volumes) are
// named with while --filename-template is left at its default, which names
// chapters and would read oddly on a volume
const FilenameVolumeTemplateDefault = `{{.Series}} Vol. {{printf "%02d" .Volume}}{{if .Group}} [{{.Group}}]{{end}}{{if gt .Version 1}} v{{.Version}}{{end}}`

// MigrateFilenameTemplate returns FilenameTemplateDefault in place of the legacy
// default a saved template may still hold, and templ otherwise
func MigrateFilenameTemplate(templ string) string {
	if templ == filenameTemplateLegacyDefault {
		return FilenameTemplateDefault
	}
	return templ
}

// isDefaultTemplate reports whether templ is a chapter default: blank, the
// current default or the legacy one
func isDefaultTemplate(templ string) bool {
	templ = strings.TrimSpace(templ)
	return templ == "" || templ == FilenameTemplateDefault || templ == filenameTemplateLegacyDefault
}

// templateNamesFile matches an action that reads a field telling files apart,
// however it is spaced, trimmed or piped
var templateNamesFile = regexp.MustCompile(`\{\{-?\s*[^}]*\.(Number|Title|SiteTitle|Volume)\b`)

// TemplateWarning returns a warning when a per-chapter templ names no number,
// title or volume, so files only differ by a version suffix; "" when it's
// fine or blank (the default)
func TemplateWarning(templ string) string {
	if strings.TrimSpace(templ) == "" || templateNamesFile.MatchString(templ) {
		return ""
	}
	return "This template has no {{.Number}}, {{.Title}}, {{.SiteTitle}} or {{.Volume}}, so files only differ by a version suffix."
}

// VolumeTemplate returns the template volume files are named with: the
// volume default when the filename template is a chapter default (or
// unset), the user's own template otherwise
func VolumeTemplate(templ string) string {
	if isDefaultTemplate(templ) {
		return FilenameVolumeTemplateDefault
	}
	return templ
}

// NewFilenameFromTemplate returns a new filename from a series title, a chapter and a template.
// A blank template is the default.
func NewFilenameFromTemplate(templ string, parts FilenameTemplateParts) (string, error) {
	if strings.TrimSpace(templ) == "" {
		templ = FilenameTemplateDefault
	}
	tmpl, err := template.New("filename").Funcs(template.FuncMap{"pad": padNumber}).Parse(templ)
	if err != nil {
		return "", err
	}

	buffer := new(bytes.Buffer)
	err = tmpl.Execute(buffer, parts)

	return buffer.String(), err
}

// padNumber is the template's pad: number with its whole part zero-padded to
// width digits and its decimals kept ({{pad .Number 4}} is "0010.5" for 10.5)
func padNumber(number any, width int) string {
	s := fmt.Sprint(number)
	digits := len(s) - len(strings.TrimLeft(s, "0123456789"))
	if digits == 0 || digits >= width {
		return s
	}
	return strings.Repeat("0", width-digits) + s
}

// NewChapterFileTemplateParts returns a new FilenameTemplateParts from a title and a chapter
func NewChapterFileTemplateParts(title string, chapter *grabber.Chapter) FilenameTemplateParts {
	return FilenameTemplateParts{
		Series:    SanitizeFilename(title),
		Number:    strings.Replace(fmt.Sprintf("%.1f", chapter.GetNumber()), ".0", "", 1),
		Title:     SanitizeFilename(grabber.ChapterLabel(chapter)),
		SiteTitle: SanitizeFilename(chapter.GetTitle()),
		Group:     SanitizeFilename(chapter.Group),
		Volume:    chapter.Volume,
	}
}

// NewVolumeFileTemplateParts returns the FilenameTemplateParts of a volume
// file: Volume is its number, Number the chapters it holds ("1-7") and Title
// "Vol. 01", for templates written with chapters in mind
func NewVolumeFileTemplateParts(title string, volume int, chapters []*grabber.Chapter) FilenameTemplateParts {
	number := ""
	if len(chapters) > 0 {
		first, last := chapters[0].GetNumber(), chapters[len(chapters)-1].GetNumber()
		number = formatChapterNumber(first)
		if last != first {
			number += "-" + formatChapterNumber(last)
		}
	}
	return FilenameTemplateParts{
		Series: SanitizeFilename(title),
		Number: number,
		Title:  fmt.Sprintf("Vol. %02d", volume),
		Group:  sharedGroup(chapters),
		Volume: volume,
	}
}

// sharedGroup returns the scanlation group every chapter was made by, sanitized
// for a filename, and "" when they differ or none names one
func sharedGroup(chapters []*grabber.Chapter) string {
	if len(chapters) == 0 {
		return ""
	}
	group := chapters[0].Group
	for _, c := range chapters[1:] {
		if c.Group != group {
			return ""
		}
	}

	return SanitizeFilename(group)
}

// formatChapterNumber formats a chapter number as filenames show it: 1, 10.5
func formatChapterNumber(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// paddedChapterNumber formats a chapter number zero-padded to (at least) 4
// integer digits so lexicographic ordering matches numeric ordering (e.g.
// 1 -> "0001", 10.5 -> "0010.5", 1186 -> "1186"). Reuses the same
// "%.1f, strip trailing .0" idea as NewChapterFileTemplateParts.
func paddedChapterNumber(number float64) string {
	numStr := strings.Replace(fmt.Sprintf("%.1f", number), ".0", "", 1)

	intPart, fracPart := numStr, ""
	if i := strings.IndexByte(numStr, '.'); i >= 0 {
		intPart, fracPart = numStr[:i], numStr[i:]
	}

	intVal, err := strconv.Atoi(intPart)
	if err != nil {
		// Fall back to the unpadded representation if parsing ever fails.
		return numStr
	}

	return fmt.Sprintf("%04d%s", intVal, fracPart)
}

// SanitizeFilename sanitizes a filename
func SanitizeFilename(filename string) string {
	sanitized := strings.Replace(filename, "/", "_", -1)
	sanitized = strings.Replace(sanitized, "\\", "_", -1)
	sanitized = strings.Replace(sanitized, ":", ";", -1)
	sanitized = strings.Replace(sanitized, "?", "¿", -1)
	sanitized = strings.Replace(sanitized, `"`, "'", -1)

	return sanitized
}
