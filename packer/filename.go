// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bytes"
	"fmt"
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
	// Title represents the chapter title (e.g. "The Beginning")
	Title string
	// Version placeholder appended to the title in case of duplicate filenames (e.g. "3")
	Version int
}

// FilenameTemplateDefault is the default filename template
const FilenameTemplateDefault = "{{.Series}} {{.Number}} - {{.Title}}{{if gt .Version 1}} v{{.Version}}{{end}}"

// NewFilenameFromTemplate returns a new filename from a series title, a chapter and a template
func NewFilenameFromTemplate(templ string, parts FilenameTemplateParts) (string, error) {
	tmpl, err := template.New("filename").Parse(templ)
	if err != nil {
		return "", err
	}

	buffer := new(bytes.Buffer)
	err = tmpl.Execute(buffer, parts)

	return buffer.String(), err
}

// NewChapterFileTemplateParts returns a new FilenameTemplateParts from a title and a chapter
func NewChapterFileTemplateParts(title string, chapter *grabber.Chapter) FilenameTemplateParts {
	return FilenameTemplateParts{
		Series: SanitizeFilename(title),
		Number: strings.Replace(fmt.Sprintf("%.1f", chapter.GetNumber()), ".0", "", 1),
		Title:  SanitizeFilename(chapter.GetTitle()),
	}
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
