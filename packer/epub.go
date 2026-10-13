// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	// registers the GIF and PNG decoders with image.DecodeConfig (the JPEG and
	// WebP ones come with convert.go's imports)
	_ "image/gif"
	_ "image/png"
	"os"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/elboletaire/manga-downloader/notice"
)

// epubSingleFolder is the folder of a single chapter's pages: they have none
const epubSingleFolder = "."

// Reading directions of EPUB output (--reading-direction): the way pages turn
const (
	ReadingDirectionRTL = "rtl"
	ReadingDirectionLTR = "ltr"
)

// EPUBInfo is what an EPUB carries beyond its pages
type EPUBInfo struct {
	// Title is the book's title (the file name, without its extension)
	Title string
	// Series is the series' title, for belongs-to-collection
	Series string
	// Position is the book's number in the series: the volume's, or a single
	// chapter's. Empty leaves group-position out, as bundles have none
	Position string
	// ReadingDirection is ReadingDirectionRTL or ReadingDirectionLTR; empty
	// means RTL
	ReadingDirection string
	// Labels names the table of contents entries by folder ("Chapter 265 - Title").
	// A folder without a label is named after the folder, and a single chapter
	// (no folder) is keyed by epubSingleFolder
	Labels map[string]string
}

// epubMediaTypes are the image types EPUB 3 readers show, by file extension.
// AVIF isn't among them, which is why it's converted before packing.
var epubMediaTypes = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
}

// epubUUIDNamespace is the namespace book identifiers are derived in (the
// RFC 4122 URL namespace, as UUID v5 names it)
var epubUUIDNamespace = []byte{0x6b, 0xa7, 0xb8, 0x11, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}

// epubPage is one page of an EPUB, with its XHTML wrapper
type epubPage struct {
	// Title is the book's title, for the XHTML wrapper
	Title string
	// Num is the 1-based index of the page in the book, zero-padded to four digits
	Num       string
	Ext       string
	MediaType string
	Width     int
	Height    int
	// Cover is set on the first page only
	Cover bool
}

// epubChapter is one table of contents entry
type epubChapter struct {
	Label string
	// Page is the Num of the chapter's first page
	Page string
	// Order is the chapter's 1-based place in the table of contents
	Order int
}

// epubBook is the data the EPUB templates are executed with
type epubBook struct {
	EPUBInfo
	UUID     string
	Modified string
	Pages    []epubPage
	Chapters []epubChapter
}

// epubFuncs are the template helpers: esc XML-escapes text, for elements and
// attributes alike
var epubFuncs = template.FuncMap{
	"esc": func(s string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	},
}

var epubOPFTemplate = template.Must(template.New("opf").Funcs(epubFuncs).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="book-id" prefix="rendition: http://www.idpf.org/vocab/rendition/#">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="book-id">urn:uuid:{{.UUID}}</dc:identifier>
    <dc:title>{{esc .Title}}</dc:title>
    <dc:language>und</dc:language>
    <meta property="dcterms:modified">{{.Modified}}</meta>
    <meta property="rendition:layout">pre-paginated</meta>
{{- if .Series}}
    <meta id="collection" property="belongs-to-collection">{{esc .Series}}</meta>
    <meta refines="#collection" property="collection-type">series</meta>
{{- if .Position}}
    <meta refines="#collection" property="group-position">{{esc .Position}}</meta>
{{- end}}
{{- end}}
    <meta name="cover" content="img-{{(index .Pages 0).Num}}"/>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
{{- range .Pages}}
    <item id="page-{{.Num}}" href="xhtml/{{.Num}}.xhtml" media-type="application/xhtml+xml"/>
    <item id="img-{{.Num}}" href="images/{{.Num}}.{{.Ext}}" media-type="{{.MediaType}}"{{if .Cover}} properties="cover-image"{{end}}/>
{{- end}}
  </manifest>
  <spine toc="ncx" page-progression-direction="{{.ReadingDirection}}">
{{- range .Pages}}
    <itemref idref="page-{{.Num}}"{{if .Cover}} properties="rendition:page-spread-center"{{end}}/>
{{- end}}
  </spine>
</package>
`))

var epubNavTemplate = template.Must(template.New("nav").Funcs(epubFuncs).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head>
  <meta charset="UTF-8"/>
  <title>{{esc .Title}}</title>
</head>
<body>
  <nav epub:type="toc" id="toc">
    <ol>
{{- range .Chapters}}
      <li><a href="xhtml/{{.Page}}.xhtml">{{esc .Label}}</a></li>
{{- end}}
    </ol>
  </nav>
</body>
</html>
`))

var epubNCXTemplate = template.Must(template.New("ncx").Funcs(epubFuncs).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <head>
    <meta name="dtb:uid" content="urn:uuid:{{.UUID}}"/>
  </head>
  <docTitle>
    <text>{{esc .Title}}</text>
  </docTitle>
  <navMap>
{{- range .Chapters}}
    <navPoint id="navpoint-{{.Order}}" playOrder="{{.Order}}">
      <navLabel>
        <text>{{esc .Label}}</text>
      </navLabel>
      <content src="xhtml/{{.Page}}.xhtml"/>
    </navPoint>
{{- end}}
  </navMap>
</ncx>
`))

var epubPageTemplate = template.Must(template.New("page").Funcs(epubFuncs).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <meta charset="UTF-8"/>
  <title>{{esc .Title}}</title>
  <meta name="viewport" content="width={{.Width}}, height={{.Height}}"/>
</head>
<body style="margin: 0; padding: 0;">
  <img src="../images/{{.Num}}.{{.Ext}}" alt="" style="display: block; width: 100%; height: 100%;"/>
</body>
</html>
`))

const epubContainer = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`

// ArchiveEPUB archives the named files into a fixed layout EPUB 3, one page per
// image. Each folder of entry names is a chapter with its own table of contents
// entry; files without a folder are a single chapter, titled as the book is.
func ArchiveEPUB(filename string, info EPUBInfo, files []File, progress func(page, progress int)) error {
	if len(files) == 0 {
		return errors.New("no files to pack")
	}

	// everything is rendered before the file is created, so a page the reader
	// can't show leaves no half written book behind
	entries, skipped, err := epubEntries(info, files)
	if err != nil {
		return err
	}
	// a skipped page still counts towards the archive's progress, which is
	// set from the pages downloaded, not the pages that made it into the book
	for range skipped {
		progress(1, 0)
	}

	buff, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer buff.Close()
	w := zip.NewWriter(buff)

	// the mimetype entry has to come first, and stored uncompressed
	mimetype, err := w.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := mimetype.Write([]byte("application/epub+zip")); err != nil {
		return err
	}

	for _, entry := range entries {
		f, err := w.Create(entry.Name)
		if err != nil {
			return err
		}
		if _, err := f.Write(entry.Data); err != nil {
			return err
		}
		if entry.Image {
			progress(1, 0) // Report progress by single page increments
		}
	}

	return w.Close()
}

// epubEntry is a file of the EPUB, with its contents. Image marks the page
// images, which are what progress counts.
type epubEntry struct {
	Name  string
	Data  []byte
	Image bool
}

// epubEntries renders the entries of the EPUB after mimetype: the container,
// the package and navigation files, and each page's XHTML and image. Pages
// the reader can't show are left out with a warning, and counted in skipped.
func epubEntries(info EPUBInfo, files []File) (entries []epubEntry, skipped int, err error) {
	if info.ReadingDirection == "" {
		info.ReadingDirection = ReadingDirectionRTL
	}
	book := epubBook{
		EPUBInfo: info,
		UUID:     bookUUID(info.Series + "\x00" + info.Title),
		Modified: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}

	var pageEntries []epubEntry
	lastFolder := ""
	for _, file := range files {
		ext := strings.TrimPrefix(path.Ext(file.Name), ".")
		mediaType, ok := epubMediaTypes[ext]
		if !ok {
			notice.Warn("- warning: page %s: EPUB readers can't show %s, leaving it out", file.Name, ext)
			skipped++
			continue
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(file.Data))
		if err != nil {
			notice.Warn("- warning: page %s: reading its size: %s, leaving it out", file.Name, err)
			skipped++
			continue
		}

		page := epubPage{
			Title:     info.Title,
			Num:       fmt.Sprintf("%04d", len(book.Pages)+1),
			Ext:       ext,
			MediaType: mediaType,
			Width:     cfg.Width,
			Height:    cfg.Height,
			Cover:     len(book.Pages) == 0,
		}
		book.Pages = append(book.Pages, page)

		// a folder is a chapter, and its first page is where its entry points
		folder := path.Dir(file.Name)
		if folder != lastFolder {
			label := folder
			if folder == epubSingleFolder {
				label = info.Title
			}
			if l := info.Labels[folder]; l != "" {
				label = l
			}
			book.Chapters = append(book.Chapters, epubChapter{Label: label, Page: page.Num, Order: len(book.Chapters) + 1})
			lastFolder = folder
		}

		xhtml, err := render(epubPageTemplate, page)
		if err != nil {
			return nil, 0, err
		}
		pageEntries = append(pageEntries,
			epubEntry{Name: "OEBPS/xhtml/" + page.Num + ".xhtml", Data: xhtml},
			epubEntry{Name: "OEBPS/images/" + page.Num + "." + ext, Data: file.Data, Image: true},
		)
	}
	if len(book.Pages) == 0 {
		return nil, skipped, errors.New("none of the pages can be shown in an EPUB")
	}

	entries = []epubEntry{{Name: "META-INF/container.xml", Data: []byte(epubContainer)}}
	for _, t := range []struct {
		name string
		tmpl *template.Template
	}{
		{"OEBPS/content.opf", epubOPFTemplate},
		{"OEBPS/nav.xhtml", epubNavTemplate},
		{"OEBPS/toc.ncx", epubNCXTemplate},
	} {
		data, err := render(t.tmpl, book)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, epubEntry{Name: t.name, Data: data})
	}

	return append(entries, pageEntries...), skipped, nil
}

// render executes tmpl with data
func render(tmpl *template.Template, data any) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, data); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", tmpl.Name(), err)
	}
	return buf.Bytes(), nil
}

// bookUUID derives a UUID (version 5) from name, so repacking the same book
// keeps its identifier, as readers use it to tell books apart
func bookUUID(name string) string {
	h := sha1.New()
	h.Write(epubUUIDNamespace)
	h.Write([]byte(name))
	sum := h.Sum(nil)
	sum[6] = sum[6]&0x0f | 0x50 // version 5
	sum[8] = sum[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
