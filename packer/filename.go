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
	Series string
	Number string
	Title  string
}

// FilenameTemplateDefault is the default filename template
var FilenameTemplateDefault = "{{.Series}} {{.Number}} - {{.Title}}"

// NewFilenameFromTemplate returns a new filename from a series title, a chapter and a template
func NewFilenameFromTemplate(title string, chapter grabber.Chapter, templ string) (string, error) {
	tmpl, err := template.New("filename").Parse(templ)
	if err != nil {
		return "", err
	}

	buffer := new(bytes.Buffer)
	err = tmpl.Execute(buffer, NewFilenameTemplateParts(title, chapter))

	return buffer.String(), err
}

// NewFilenameTemplateParts returns a new FilenameTemplateParts from a title and a chapter
func NewFilenameTemplateParts(title string, chapter grabber.Chapter) FilenameTemplateParts {
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

	return sanitized
}
