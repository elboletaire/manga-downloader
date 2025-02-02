package packer

import (
	"bytes"
	"fmt"
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
}

// FilenameTemplateDefault is the default filename template
const FilenameTemplateDefault = "{{.Series}} {{.Number}} - {{.Title}}"

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
	return NewChapterFileTemplatePartsFromParts(title, chapter.GetNumber(), chapter.GetTitle())
}

// NewChapterFileTemplateParts returns a new FilenameTemplateParts from a series title, a number and a chapter title
func NewChapterFileTemplatePartsFromParts(series string, number float64, title string) FilenameTemplateParts {
	return FilenameTemplateParts{
		Series: SanitizeFilename(series),
		Number: strings.Replace(fmt.Sprintf("%.1f", number), ".0", "", 1),
		Title:  SanitizeFilename(title),
	}
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
