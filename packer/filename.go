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

// SanitizeFilename sanitizes a filename
func SanitizeFilename(filename string) string {
	sanitized := strings.Replace(filename, "/", "_", -1)
	sanitized = strings.Replace(sanitized, "\\", "_", -1)
	sanitized = strings.Replace(sanitized, ":", ";", -1)
	sanitized = strings.Replace(sanitized, "?", "¿", -1)
	sanitized = strings.Replace(sanitized, `"`, "'", -1)

	return sanitized
}
