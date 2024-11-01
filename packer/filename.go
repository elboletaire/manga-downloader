package packer

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/voxelost/manga-downloader/grabber"
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
	return FilenameTemplateParts{
		Series: SanitizeFilename(title),
		Number: strings.Replace(fmt.Sprintf("%.1f", chapter.GetNumber()), ".0", "", 1),
		Title:  SanitizeFilename(chapter.GetTitle()),
	}
}

// SanitizeFilename sanitizes a filename
func SanitizeFilename(filename string) string {
	sanitized := strings.ReplaceAll(filename, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, `\`, "_")
	sanitized = strings.ReplaceAll(sanitized, ":", ";")
	sanitized = strings.ReplaceAll(sanitized, "?", "¿")
	sanitized = strings.ReplaceAll(sanitized, `"`, "'")

	return sanitized
}
