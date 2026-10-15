// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
)

// epubTitle has every character XML needs escaped, so a missing escape fails
// the parse rather than passing silently
const epubTitle = `Tom & Jerry <"Vol">`

// epubBundleFiles is a bundle of two chapters, three pages in all, the first
// one 8x12 and the others 6x10
func epubBundleFiles(t *testing.T) []File {
	t.Helper()
	return []File{
		{Name: "Chapter 0001/000.png", Data: pngBytes(t, gradientImage(8, 12))},
		{Name: "Chapter 0001/001.jpg", Data: jpegBytes(t, gradientImage(6, 10))},
		{Name: "Chapter 0002/000.png", Data: pngBytes(t, gradientImage(6, 10))},
	}
}

// readEPUB opens the EPUB at path and returns its entries in order, with
// their contents by name
func readEPUB(t *testing.T, path string) ([]*zip.File, map[string][]byte) {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening epub: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	contents := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening entry %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("reading entry %s: %v", f.Name, err)
		}
		contents[f.Name] = data
	}
	return r.File, contents
}

// packEPUB writes the files as an EPUB in a temporary directory and returns
// its path
func packEPUB(t *testing.T, info EPUBInfo, files []File) string {
	t.Helper()

	if info.Title == "" {
		info.Title = epubTitle
	}
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := ArchiveEPUB(path, info, files, func(page, progress int) {}); err != nil {
		t.Fatalf("ArchiveEPUB: %v", err)
	}
	return path
}

// opfPackage is the part of the OPF the tests read back
type opfPackage struct {
	Metadata struct {
		Title      string `xml:"title"`
		Identifier string `xml:"identifier"`
		Language   string `xml:"language"`
		Metas      []meta `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Direction string `xml:"page-progression-direction,attr"`
		Itemrefs  []struct {
			IDRef      string `xml:"idref,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// meta is a <meta> element of an OPF, or the name/content pair of a
// "meta name" one
type meta struct {
	Name     string `xml:"name,attr"`
	Content  string `xml:"content,attr"`
	Property string `xml:"property,attr"`
	Refines  string `xml:"refines,attr"`
	Value    string `xml:",chardata"`
}

func TestArchiveEPUBPackage(t *testing.T) {
	path := packEPUB(t, EPUBInfo{Series: epubTitle, Position: "", ReadingDirection: ReadingDirectionLTR}, epubBundleFiles(t))
	entries, contents := readEPUB(t, path)

	// mimetype first, stored uncompressed, with no data descriptor
	if entries[0].Name != "mimetype" || entries[0].Method != zip.Store {
		t.Errorf("first entry = %q (method %d), want mimetype stored", entries[0].Name, entries[0].Method)
	}
	if entries[0].Flags&0x8 != 0 {
		t.Errorf("mimetype has the data descriptor flag set (flags %#x)", entries[0].Flags)
	}
	if got := string(contents["mimetype"]); got != "application/epub+zip" {
		t.Errorf("mimetype = %q", got)
	}

	// the container points at the package document
	var container struct {
		RootFiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	mustUnmarshal(t, "container.xml", contents["META-INF/container.xml"], &container)
	if len(container.RootFiles) != 1 || container.RootFiles[0].FullPath != "OEBPS/content.opf" {
		t.Fatalf("container rootfiles = %+v, want OEBPS/content.opf", container.RootFiles)
	}

	var opf opfPackage
	mustUnmarshal(t, "content.opf", contents["OEBPS/content.opf"], &opf)

	if opf.Metadata.Title != epubTitle {
		t.Errorf("title = %q, want %q", opf.Metadata.Title, epubTitle)
	}
	if !strings.HasPrefix(opf.Metadata.Identifier, "urn:uuid:") {
		t.Errorf("identifier = %q, want a urn:uuid", opf.Metadata.Identifier)
	}
	if opf.Metadata.Language == "" {
		t.Error("no dc:language")
	}
	if !hasMeta(opf.Metadata.Metas, "dcterms:modified") {
		t.Error("no dcterms:modified")
	}
	if !hasMeta(opf.Metadata.Metas, "rendition:layout") {
		t.Error("no rendition:layout")
	}

	if opf.Spine.Direction != "ltr" {
		t.Errorf("page-progression-direction = %q, want ltr", opf.Spine.Direction)
	}

	// the manifest lists every file of the package, and nothing else
	manifest := map[string]string{}
	for _, item := range opf.Manifest.Items {
		manifest["OEBPS/"+item.Href] = item.ID
		if _, ok := contents["OEBPS/"+item.Href]; !ok {
			t.Errorf("manifest item %s is not in the archive", item.Href)
		}
	}
	for name := range contents {
		if name == "mimetype" || name == "META-INF/container.xml" || name == "OEBPS/content.opf" {
			continue
		}
		if _, ok := manifest[name]; !ok {
			t.Errorf("%s is not in the manifest", name)
		}
	}

	// the spine reads the pages in order, the first one as the cover
	var spine []string
	for _, ref := range opf.Spine.Itemrefs {
		spine = append(spine, ref.IDRef)
	}
	wantSpine := []string{"page-0001", "page-0002", "page-0003"}
	if !reflect.DeepEqual(spine, wantSpine) {
		t.Errorf("spine = %v, want %v", spine, wantSpine)
	}
	if opf.Spine.Itemrefs[0].Properties != "rendition:page-spread-center" {
		t.Errorf("cover itemref properties = %q", opf.Spine.Itemrefs[0].Properties)
	}
	if !hasMeta(opf.Metadata.Metas, "cover") {
		t.Error("no EPUB 2 cover meta")
	}
	for _, item := range opf.Manifest.Items {
		isCover := strings.Contains(item.Properties, "cover-image")
		if isCover != (item.ID == "img-0001") {
			t.Errorf("item %s cover-image = %v", item.ID, isCover)
		}
		if strings.HasPrefix(item.ID, "img-") && item.MediaType != "image/png" && item.MediaType != "image/jpeg" {
			t.Errorf("item %s media type = %q", item.ID, item.MediaType)
		}
	}

	// a page is an XHTML wrapper whose viewport is the image's size
	wantSizes := map[string]string{
		"OEBPS/xhtml/0001.xhtml": "width=8, height=12",
		"OEBPS/xhtml/0002.xhtml": "width=6, height=10",
		"OEBPS/xhtml/0003.xhtml": "width=6, height=10",
	}
	for name, want := range wantSizes {
		var page struct {
			Metas []meta `xml:"head>meta"`
		}
		mustUnmarshal(t, name, contents[name], &page)
		if got := metaContent(page.Metas, "viewport"); got != want {
			t.Errorf("%s viewport = %q, want %q", name, got, want)
		}
	}

	// navigation: one entry per chapter, named by its folder
	var nav struct {
		Items []struct {
			Href string `xml:"href,attr"`
			Text string `xml:",chardata"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	wantNav := []string{"Chapter 0001|xhtml/0001.xhtml", "Chapter 0002|xhtml/0003.xhtml"}
	var gotNav []string
	for _, item := range nav.Items {
		gotNav = append(gotNav, item.Text+"|"+item.Href)
	}
	if !reflect.DeepEqual(gotNav, wantNav) {
		t.Errorf("nav = %v, want %v", gotNav, wantNav)
	}

	var ncx struct {
		DocTitle string `xml:"docTitle>text"`
		Points   []struct {
			PlayOrder string `xml:"playOrder,attr"`
			Label     string `xml:"navLabel>text"`
			Content   struct {
				Src string `xml:"src,attr"`
			} `xml:"content"`
		} `xml:"navMap>navPoint"`
	}
	mustUnmarshal(t, "toc.ncx", contents["OEBPS/toc.ncx"], &ncx)
	if ncx.DocTitle != epubTitle {
		t.Errorf("ncx docTitle = %q", ncx.DocTitle)
	}
	if len(ncx.Points) != 2 || ncx.Points[0].Label != "Chapter 0001" || ncx.Points[1].Content.Src != "xhtml/0003.xhtml" {
		t.Errorf("ncx points = %+v, want one per chapter", ncx.Points)
	}
	// readers expect playOrder to count up from 1
	if len(ncx.Points) == 2 && (ncx.Points[0].PlayOrder != "1" || ncx.Points[1].PlayOrder != "2") {
		t.Errorf("ncx playOrder = %q, %q, want 1, 2", ncx.Points[0].PlayOrder, ncx.Points[1].PlayOrder)
	}
}

func TestArchiveEPUBSeriesMetadata(t *testing.T) {
	// a volume is a collection member numbered by its volume
	path := packEPUB(t, EPUBInfo{Series: epubTitle, Position: "3", ReadingDirection: ReadingDirectionRTL}, epubBundleFiles(t))
	_, contents := readEPUB(t, path)

	var opf opfPackage
	mustUnmarshal(t, "content.opf", contents["OEBPS/content.opf"], &opf)
	if got := metaValue(opf.Metadata.Metas, "belongs-to-collection"); got != epubTitle {
		t.Errorf("belongs-to-collection = %q, want %q", got, epubTitle)
	}
	if got := metaValue(opf.Metadata.Metas, "collection-type"); got != "series" {
		t.Errorf("collection-type = %q, want series", got)
	}
	if got := metaValue(opf.Metadata.Metas, "group-position"); got != "3" {
		t.Errorf("group-position = %q, want 3", got)
	}
	if opf.Spine.Direction != "rtl" {
		t.Errorf("page-progression-direction = %q, want rtl", opf.Spine.Direction)
	}
}

func TestArchiveEPUBBundleHasNoGroupPosition(t *testing.T) {
	path := packEPUB(t, EPUBInfo{Series: "Series"}, epubBundleFiles(t))
	_, contents := readEPUB(t, path)

	var opf opfPackage
	mustUnmarshal(t, "content.opf", contents["OEBPS/content.opf"], &opf)
	if hasMeta(opf.Metadata.Metas, "group-position") {
		t.Error("a bundle has a group-position")
	}
	if opf.Spine.Direction != "rtl" {
		t.Errorf("an empty reading direction should default to rtl, got %q", opf.Spine.Direction)
	}
}

func TestArchiveEPUBSingleChapter(t *testing.T) {
	// files without a folder are one chapter, titled as the book is
	files := []File{{Name: "000.jpg", Data: jpegBytes(t, gradientImage(4, 4))}}
	path := packEPUB(t, EPUBInfo{Series: "Series", Position: "10.5"}, files)
	_, contents := readEPUB(t, path)

	var nav struct {
		Items []struct {
			Text string `xml:",chardata"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	if len(nav.Items) != 1 || nav.Items[0].Text != epubTitle {
		t.Errorf("nav = %+v, want the single entry titled as the book", nav.Items)
	}

	var opf opfPackage
	mustUnmarshal(t, "content.opf", contents["OEBPS/content.opf"], &opf)
	if got := metaValue(opf.Metadata.Metas, "group-position"); got != "10.5" {
		t.Errorf("group-position = %q, want 10.5", got)
	}
}

func TestArchiveEPUBEscapesMetadata(t *testing.T) {
	path := packEPUB(t, EPUBInfo{Series: epubTitle, Position: "1"}, epubBundleFiles(t))
	_, contents := readEPUB(t, path)

	// every document parses, which a raw & or < in a title wouldn't
	for _, name := range []string{"OEBPS/content.opf", "OEBPS/nav.xhtml", "OEBPS/toc.ncx"} {
		if err := xml.Unmarshal(contents[name], new(any)); err != nil {
			t.Errorf("%s is not well formed: %v", name, err)
		}
	}
	if !bytes.Contains(contents["OEBPS/content.opf"], []byte("Tom &amp; Jerry &lt;&#34;Vol&#34;&gt;")) {
		t.Error("the title isn't escaped in the OPF")
	}
}

func TestArchiveEPUBRefusesUnreadableImages(t *testing.T) {
	cases := map[string][]File{
		// AVIF isn't an image EPUB readers show; the packer converts it first
		"avif": {{Name: "000.avif", Data: []byte("not really avif")}},
		// bytes that don't decode have no size to put in the viewport
		"undecodable": {{Name: "000.jpg", Data: []byte("not an image")}},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "book.epub")
			err := ArchiveEPUB(path, EPUBInfo{Title: epubTitle}, files, func(page, progress int) {})
			if err == nil {
				t.Fatal("expected an error")
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Errorf("a refused book left a file behind: %v", statErr)
			}
		})
	}
}

func TestArchiveEPUBProgressPerPage(t *testing.T) {
	pages := 0
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := ArchiveEPUB(path, EPUBInfo{Title: epubTitle}, epubBundleFiles(t), func(page, _ int) { pages += page }); err != nil {
		t.Fatalf("ArchiveEPUB: %v", err)
	}
	if pages != 3 {
		t.Errorf("progress reported %d pages, want 3", pages)
	}
}

func TestPageFormatsConvertsAVIFForEPUB(t *testing.T) {
	tests := []struct {
		format string
		in     grabber.ConvertFormats
		want   grabber.ConvertFormats
	}{
		{FormatEPUB, grabber.ConvertFormats{}, grabber.ConvertFormats{grabber.ConvertAVIF: true}},
		{FormatEPUB, grabber.ConvertFormats{grabber.ConvertWebP: true}, grabber.ConvertFormats{grabber.ConvertAVIF: true, grabber.ConvertWebP: true}},
		{FormatCBZ, grabber.ConvertFormats{}, grabber.ConvertFormats{}},
		{FormatRaw, grabber.ConvertFormats{grabber.ConvertWebP: true}, grabber.ConvertFormats{grabber.ConvertWebP: true}},
	}
	for _, tt := range tests {
		if got := pageFormats(tt.format, tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("pageFormats(%q, %v) = %v, want %v", tt.format, tt.in, got, tt.want)
		}
	}
	// the caller's set is left alone
	in := grabber.ConvertFormats{}
	pageFormats(FormatEPUB, in)
	if len(in) != 0 {
		t.Errorf("pageFormats changed its input: %v", in)
	}
}

func TestPackBundleEPUBConvertsAVIF(t *testing.T) {
	// --convert-images none still converts AVIF for EPUB, so the page is a JPEG
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: FormatEPUB, direction: ReadingDirectionLTR, convert: grabber.ConvertFormats{}}
	chapters := []*DownloadedChapter{
		{
			Chapter: &grabber.Chapter{Number: 1},
			Files:   []*downloader.File{{Data: avifBytes(t, gradientImage(16, 16))}},
		},
	}

	dir := t.TempDir()
	filename, err := PackBundle(dir, site, chapters, "1", func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}
	if filename != "Test Series 1 - bundle.epub" {
		t.Errorf("filename = %q", filename)
	}

	_, contents := readEPUB(t, filepath.Join(dir, filename))
	if _, ok := contents["OEBPS/images/0001.jpg"]; !ok {
		t.Fatalf("the AVIF page wasn't converted to a jpeg: %v", keys(contents))
	}
	var opf opfPackage
	mustUnmarshal(t, "content.opf", contents["OEBPS/content.opf"], &opf)
	if !strings.Contains(fmt.Sprint(opf.Manifest.Items), "image/jpeg") {
		t.Errorf("manifest has no jpeg: %+v", opf.Manifest.Items)
	}
}

// mustUnmarshal parses data as XML into v, failing the test on malformed input
func mustUnmarshal(t *testing.T, name string, data []byte, v any) {
	t.Helper()
	if err := xml.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// hasMeta reports whether metas has a meta with the given property or name
func hasMeta(metas []meta, key string) bool {
	for _, m := range metas {
		if m.Property == key || m.Name == key {
			return true
		}
	}
	return false
}

// metaValue returns the text of the meta with the given property
func metaValue(metas []meta, property string) string {
	for _, m := range metas {
		if m.Property == property {
			return m.Value
		}
	}
	return ""
}

// metaContent returns the content of the meta with the given name
func metaContent(metas []meta, name string) string {
	for _, m := range metas {
		if m.Name == name {
			return m.Content
		}
	}
	return ""
}

// keys returns the keys of m, for error messages
func keys(m map[string][]byte) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

// captureWarnings collects the warnings notice emits while the test runs
func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	var warnings []string
	notice.SetHandler(func(level notice.Level, msg string) {
		if level == notice.LevelWarn {
			warnings = append(warnings, msg)
		}
	})
	t.Cleanup(func() { notice.SetHandler(nil) })
	return &warnings
}

func TestArchiveEPUBSkipsUnshowablePages(t *testing.T) {
	warnings := captureWarnings(t)
	files := []File{
		{Name: "Chapter 0001/000.png", Data: pngBytes(t, gradientImage(8, 12))},
		// AVIF that didn't convert, and bytes that don't decode
		{Name: "Chapter 0001/001.avif", Data: []byte("not really avif")},
		{Name: "Chapter 0002/000.jpg", Data: []byte("not an image")},
		{Name: "Chapter 0002/001.jpg", Data: jpegBytes(t, gradientImage(6, 10))},
	}

	pages := 0
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := ArchiveEPUB(path, EPUBInfo{Title: epubTitle}, files, func(page, _ int) { pages += page }); err != nil {
		t.Fatalf("ArchiveEPUB: %v", err)
	}
	// the skipped pages still count, so the archive's progress reaches its total
	if pages != len(files) {
		t.Errorf("progress reported %d pages, want %d", pages, len(files))
	}
	if len(*warnings) != 2 {
		t.Errorf("got %d warnings, want 2: %q", len(*warnings), *warnings)
	}

	_, contents := readEPUB(t, path)
	if _, ok := contents["OEBPS/xhtml/0003.xhtml"]; ok {
		t.Error("the book has more pages than the showable ones")
	}
	if _, ok := contents["OEBPS/xhtml/0002.xhtml"]; !ok {
		t.Error("the showable page of the second chapter is missing")
	}

	var nav struct {
		Items []struct {
			Href string `xml:"href,attr"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	if len(nav.Items) != 2 || nav.Items[1].Href != "xhtml/0002.xhtml" {
		t.Errorf("nav = %+v, want both chapters, the second starting at its first shown page", nav.Items)
	}
}

func TestArchiveEPUBChapterWithoutShowablePagesIsWarned(t *testing.T) {
	warnings := captureWarnings(t)
	files := []File{
		{Name: "Chapter 0001/000.png", Data: pngBytes(t, gradientImage(8, 12))},
		{Name: "Chapter 0002/000.avif", Data: []byte("not really avif")},
		{Name: "Chapter 0002/001.avif", Data: []byte("not really avif")},
	}
	info := EPUBInfo{Title: epubTitle, Labels: map[string]string{"Chapter 0001": "Chapter 1", "Chapter 0002": "Chapter 2"}}
	path := packEPUB(t, info, files)
	_, contents := readEPUB(t, path)

	var nav struct {
		Items []struct {
			Text string `xml:",chardata"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	if len(nav.Items) != 1 || nav.Items[0].Text != "Chapter 1" {
		t.Errorf("nav = %+v, want only the chapter with pages", nav.Items)
	}

	// the two pages, and the chapter that has none left
	if len(*warnings) != 3 {
		t.Fatalf("got %d warnings, want 3: %q", len(*warnings), *warnings)
	}
	chapterWarnings := 0
	for _, w := range *warnings {
		if strings.Contains(w, "no pages") {
			chapterWarnings++
			if !strings.Contains(w, "Chapter 2") {
				t.Errorf("chapter warning = %q, want it to name Chapter 2", w)
			}
		}
	}
	if chapterWarnings != 1 {
		t.Errorf("got %d chapter warnings, want 1: %q", chapterWarnings, *warnings)
	}
}

func TestArchiveEPUBNCXHead(t *testing.T) {
	path := packEPUB(t, EPUBInfo{Series: "Series"}, epubBundleFiles(t))
	_, contents := readEPUB(t, path)

	var ncx struct {
		Metas []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"head>meta"`
	}
	mustUnmarshal(t, "toc.ncx", contents["OEBPS/toc.ncx"], &ncx)
	got := map[string]string{}
	for _, m := range ncx.Metas {
		got[m.Name] = m.Content
	}
	// the metas NCX readers require, for a flat table of contents
	want := map[string]string{"dtb:depth": "1", "dtb:totalPageCount": "0", "dtb:maxPageNumber": "0"}
	for name, content := range want {
		if got[name] != content {
			t.Errorf("ncx %s = %q, want %q", name, got[name], content)
		}
	}
}

func TestNamePagesEPUBWarnsOncePerFailedPage(t *testing.T) {
	warnings := captureWarnings(t)
	formats := grabber.ConvertFormats{grabber.ConvertAVIF: true, grabber.ConvertWebP: true}

	// an AVIF that won't convert is left out by ArchiveEPUB, which warns about
	// it: namePages stays quiet rather than warning twice
	avif := append([]byte{0x00, 0x00, 0x00, 0x1c, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, []byte("truncated")...)
	namePages([]*downloader.File{{Data: avif}}, formats, FormatEPUB)
	if len(*warnings) != 0 {
		t.Errorf("an AVIF failure warned %d times for EPUB, want none: %q", len(*warnings), *warnings)
	}

	// a WebP EPUB can show keeps its original, which is still a warning
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), []byte("truncated")...)
	named := namePages([]*downloader.File{{Data: webp}}, formats, FormatEPUB)
	if len(*warnings) != 1 || named[0].Name != "000.webp" {
		t.Errorf("a WebP failure: %d warnings, named %q; want one warning and 000.webp", len(*warnings), named[0].Name)
	}
}

func TestArchiveEPUBFailsWithoutShowablePages(t *testing.T) {
	warnings := captureWarnings(t)
	path := filepath.Join(t.TempDir(), "book.epub")
	files := []File{{Name: "000.avif", Data: []byte("x")}, {Name: "001.jpg", Data: []byte("y")}}
	if err := ArchiveEPUB(path, EPUBInfo{Title: epubTitle}, files, func(page, progress int) {}); err == nil {
		t.Fatal("expected an error when no page can be shown")
	}
	// the two pages, and the one chapter they belong to
	if len(*warnings) != 3 {
		t.Errorf("got %d warnings, want 3: %q", len(*warnings), *warnings)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a book with no pages left a file behind: %v", err)
	}
}

func TestArchiveEPUBLabelsAndPageTitles(t *testing.T) {
	info := EPUBInfo{
		Title:  epubTitle,
		Labels: map[string]string{"Chapter 0001": "Chapter 1 - Start", "Chapter 0002": "Chapter 2"},
	}
	path := packEPUB(t, info, epubBundleFiles(t))
	_, contents := readEPUB(t, path)

	var nav struct {
		Items []struct {
			Text string `xml:",chardata"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	if len(nav.Items) != 2 || nav.Items[0].Text != "Chapter 1 - Start" || nav.Items[1].Text != "Chapter 2" {
		t.Errorf("nav labels = %+v, want the chapter labels", nav.Items)
	}

	var page struct {
		Title string `xml:"head>title"`
	}
	mustUnmarshal(t, "page", contents["OEBPS/xhtml/0002.xhtml"], &page)
	if page.Title != epubTitle {
		t.Errorf("page title = %q, want the book title %q", page.Title, epubTitle)
	}
}

func TestArchiveEPUBSingleChapterLabel(t *testing.T) {
	files := []File{{Name: "000.jpg", Data: jpegBytes(t, gradientImage(4, 4))}}
	path := packEPUB(t, EPUBInfo{Series: "Series", Labels: map[string]string{epubSingleFolder: "Chapter 266 - Final Choice"}}, files)
	_, contents := readEPUB(t, path)

	var nav struct {
		Items []struct {
			Text string `xml:",chardata"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	if len(nav.Items) != 1 || nav.Items[0].Text != "Chapter 266 - Final Choice" {
		t.Errorf("nav = %+v, want the one chapter label", nav.Items)
	}
}

func TestChapterLabel(t *testing.T) {
	tests := []struct {
		number float64
		title  string
		want   string
	}{
		{265, "Final Choice", "Chapter 265 - Final Choice"},
		{10.5, "", "Chapter 10.5"},
		{1, "Chapter 1", "Chapter 1"},
		{2, "2", "Chapter 2"},
		{3, "  Trimmed\nTitle ", "Chapter 3 - Trimmed Title"},
		// MangaDex titles come as "Chapter %04d <title>", and a title that's
		// only the number leaves nothing behind
		{5, "Chapter 0005 Foo", "Chapter 5 - Foo"},
		{5, "Chapter 0005 ", "Chapter 5"},
		{5, "Chapter 0005", "Chapter 5"},
		{5, "Chapter 5: Foo", "Chapter 5 - Foo"},
		{5, "Ch. 5", "Chapter 5"},
		{5, "Ch.5 - Foo", "Chapter 5 - Foo"},
		{5, "Chapter 5th Child", "Chapter 5 - Chapter 5th Child"},
		{5, "Chapter 5 − Foo", "Chapter 5 - Foo"},
		{5, "Chapter 005", "Chapter 5"},
		{5, "Episode 5 - Foo", "Chapter 5 - Foo"},
		{10.5, "Chapter 10.50: Foo", "Chapter 10.5 - Foo"},
		{10.5, "Chapter 0010.5 Foo", "Chapter 10.5 - Foo"},
		// a number that isn't the chapter's stays in the title
		{5, "Chapter 50", "Chapter 5 - Chapter 50"},
		{5, "Chapter 5.5", "Chapter 5 - Chapter 5.5"},
		{0, "Chapter 0: Prologue", "Chapter 0 - Prologue"},
		// localized and volume-led prefixes are the number too
		{12, "Capítulo 12: Foo", "Chapter 12 - Foo"},
		{7, "Chapitre 7 : Foo", "Chapter 7 - Foo"},
		{12, "Vol. 3 Chapter 12: Foo", "Chapter 12 - Foo"},
	}
	for _, tt := range tests {
		got := chapterLabel(&grabber.Chapter{Number: tt.number, Title: tt.title})
		if got != tt.want {
			t.Errorf("chapterLabel(%v, %q) = %q, want %q", tt.number, tt.title, got, tt.want)
		}
	}
}

func TestPackBundleEPUBLabelsChapters(t *testing.T) {
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: FormatEPUB}
	chapters := []*DownloadedChapter{
		{Chapter: &grabber.Chapter{Number: 265, Title: "Final Choice"}, Files: []*downloader.File{{Data: pngBytes(t, gradientImage(4, 4))}}},
		{Chapter: &grabber.Chapter{Number: 266, Title: "266"}, Files: []*downloader.File{{Data: pngBytes(t, gradientImage(4, 4))}}},
	}

	dir := t.TempDir()
	filename, err := PackBundle(dir, site, chapters, "265-266", func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}
	_, contents := readEPUB(t, filepath.Join(dir, filename))

	var nav struct {
		Items []struct {
			Text string `xml:",chardata"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	if len(nav.Items) != 2 || nav.Items[0].Text != "Chapter 265 - Final Choice" || nav.Items[1].Text != "Chapter 266" {
		t.Errorf("nav = %+v, want the labels of both chapters", nav.Items)
	}
}

func TestPackBundleEPUBRetryReportsOnce(t *testing.T) {
	warnings := captureWarnings(t)
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: FormatEPUB}
	chapters := []*DownloadedChapter{
		{
			Chapter: &grabber.Chapter{Number: 1},
			Files: []*downloader.File{
				{Data: pngBytes(t, gradientImage(4, 4))},
				{Data: []byte("not an image")},
			},
		},
	}

	// the first name is taken, so pack retries with a version suffix
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Test Series 1 - bundle.epub"), []byte("taken"), 0644); err != nil {
		t.Fatal(err)
	}
	pages := 0
	filename, err := PackBundle(dir, site, chapters, "1", func(page, _ int) { pages += page })
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}
	if filename != "Test Series 1 - bundle v2.epub" {
		t.Errorf("filename = %q, want the retried name", filename)
	}
	if pages != 2 {
		t.Errorf("progress reported %d pages, want the 2 files", pages)
	}
	if len(*warnings) != 1 {
		t.Errorf("got %d warnings across the retry, want 1: %q", len(*warnings), *warnings)
	}
}

func TestPackBundleEPUBLabelsRepeatedNumbers(t *testing.T) {
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: FormatEPUB}
	chapters := []*DownloadedChapter{
		{Chapter: &grabber.Chapter{Number: 1, Title: "Start"}, Files: []*downloader.File{{Data: pngBytes(t, gradientImage(4, 4))}}},
		{Chapter: &grabber.Chapter{Number: 1, Title: "Start"}, Files: []*downloader.File{{Data: pngBytes(t, gradientImage(4, 4))}}},
	}

	dir := t.TempDir()
	filename, err := PackBundle(dir, site, chapters, "1", func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}
	_, contents := readEPUB(t, filepath.Join(dir, filename))

	var nav struct {
		Items []struct {
			Text string `xml:",chardata"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	if len(nav.Items) != 2 || nav.Items[0].Text != "Chapter 1 - Start" || nav.Items[1].Text != "Chapter 1 - Start (2)" {
		t.Errorf("nav = %+v, want the repeated chapter labelled like its folder", nav.Items)
	}
}

// readOPF returns the package document of an EPUB's contents
func readOPF(t *testing.T, contents map[string][]byte) opfPackage {
	t.Helper()
	var opf opfPackage
	mustUnmarshal(t, "content.opf", contents["OEBPS/content.opf"], &opf)
	return opf
}

// navEntries returns the table of contents as "label|href" strings
func navEntries(t *testing.T, contents map[string][]byte) []string {
	t.Helper()
	var nav struct {
		Items []struct {
			Href string `xml:"href,attr"`
			Text string `xml:",chardata"`
		} `xml:"body>nav>ol>li>a"`
	}
	mustUnmarshal(t, "nav.xhtml", contents["OEBPS/nav.xhtml"], &nav)
	var entries []string
	for _, item := range nav.Items {
		entries = append(entries, item.Text+"|"+item.Href)
	}
	return entries
}

// bookIdentifier returns the dc:identifier of the EPUB at path
func bookIdentifier(t *testing.T, dir, name string) string {
	t.Helper()
	_, contents := readEPUB(t, filepath.Join(dir, name))
	return readOPF(t, contents).Metadata.Identifier
}

func TestEPUBIdentifierDiffersForSameTitle(t *testing.T) {
	// a template without the chapter's number gives two books the same file
	// name, which the identifier must still tell apart
	site := &fakeSite{title: "Test Series", template: "{{.Series}}", format: FormatEPUB}
	chapter := func(number float64) *DownloadedChapter {
		return &DownloadedChapter{
			Chapter: &grabber.Chapter{Number: number, Title: "Start"},
			Files:   []*downloader.File{{Data: pngBytes(t, gradientImage(4, 4))}},
		}
	}

	first, second := t.TempDir(), t.TempDir()
	name1, err := PackSingle(first, site, chapter(1), func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackSingle: %v", err)
	}
	name2, err := PackSingle(second, site, chapter(2), func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackSingle: %v", err)
	}
	if name1 != name2 {
		t.Fatalf("file names %q and %q differ, the test needs them to match", name1, name2)
	}
	if bookIdentifier(t, first, name1) == bookIdentifier(t, second, name2) {
		t.Error("two different chapters with the same file name share an identifier")
	}
}

func TestEPUBIdentifierStableAcrossRepacks(t *testing.T) {
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: FormatEPUB}
	chapter := &DownloadedChapter{
		Chapter: &grabber.Chapter{Number: 5, Title: "Start"},
		Files:   []*downloader.File{{Data: pngBytes(t, gradientImage(4, 4))}},
	}

	// packed twice into the same folder, the second copy is named with a
	// version suffix: the names differ, the books don't
	dir := t.TempDir()
	name1, err := PackSingle(dir, site, chapter, func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackSingle: %v", err)
	}
	name2, err := PackSingle(dir, site, chapter, func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackSingle: %v", err)
	}
	if name1 == name2 {
		t.Fatalf("both copies are named %q, the test needs the second to get a suffix", name1)
	}
	if got, want := bookIdentifier(t, dir, name2), bookIdentifier(t, dir, name1); got != want {
		t.Errorf("repacking the same chapter changed its identifier: %q, want %q", got, want)
	}
}

func TestArchiveEPUBIdentifierFallsBackToTitle(t *testing.T) {
	// without an Identity the identifier is the series and title, as before
	// Identity existed, so direct callers of a series don't share one
	first, second, third := t.TempDir(), t.TempDir(), t.TempDir()
	packEPUB := func(dir string, title string) string {
		path := filepath.Join(dir, "book.epub")
		if err := ArchiveEPUB(path, EPUBInfo{Series: "Series", Title: title}, epubBundleFiles(t), func(page, progress int) {}); err != nil {
			t.Fatalf("ArchiveEPUB: %v", err)
		}
		return bookIdentifier(t, dir, "book.epub")
	}
	a := packEPUB(first, "Book A")
	if again := packEPUB(second, "Book A"); again != a {
		t.Errorf("the same title gave %q and %q", a, again)
	}
	if b := packEPUB(third, "Book B"); b == a {
		t.Errorf("different titles in one series share the identifier %q", a)
	}
}

func TestEPUBIdentifierTellsKindsApart(t *testing.T) {
	// chapter 5, volume 5 and the bundle of chapter 5 are three books
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: FormatEPUB}
	chapters := []*DownloadedChapter{{
		Chapter: &grabber.Chapter{Number: 5, Title: "Start"},
		Files:   []*downloader.File{{Data: pngBytes(t, gradientImage(4, 4))}},
	}}
	progress := func(page, progress int) {}

	dirs := map[string]string{"chapter": t.TempDir(), "volume": t.TempDir(), "bundle": t.TempDir()}
	chapterName, err := PackSingle(dirs["chapter"], site, chapters[0], progress)
	if err != nil {
		t.Fatalf("PackSingle: %v", err)
	}
	volumeName, err := PackVolume(dirs["volume"], site, 5, chapters, progress)
	if err != nil {
		t.Fatalf("PackVolume: %v", err)
	}
	bundleName, err := PackBundle(dirs["bundle"], site, chapters, "5", progress)
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}

	ids := map[string]string{
		"chapter": bookIdentifier(t, dirs["chapter"], chapterName),
		"volume":  bookIdentifier(t, dirs["volume"], volumeName),
		"bundle":  bookIdentifier(t, dirs["bundle"], bundleName),
	}
	if ids["chapter"] == ids["volume"] {
		t.Errorf("chapter 5 and volume 5 share the identifier %q", ids["chapter"])
	}
	if ids["chapter"] == ids["bundle"] {
		t.Errorf("chapter 5 and its bundle share the identifier %q", ids["chapter"])
	}
	if ids["volume"] == ids["bundle"] {
		t.Errorf("volume 5 and its bundle share the identifier %q", ids["volume"])
	}
}

func TestPackVolumeEPUB(t *testing.T) {
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: FormatEPUB, direction: ReadingDirectionLTR}
	chapters := []*DownloadedChapter{
		{
			Chapter: &grabber.Chapter{Number: 1, Title: "Start"},
			Files: []*downloader.File{
				{Data: pngBytes(t, gradientImage(8, 12))},
				{Data: jpegBytes(t, gradientImage(6, 10))},
			},
		},
		{
			Chapter: &grabber.Chapter{Number: 2, Title: "2"},
			Files:   []*downloader.File{{Data: pngBytes(t, gradientImage(6, 10))}},
		},
	}

	dir := t.TempDir()
	name, err := PackVolume(dir, site, 2, chapters, func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackVolume: %v", err)
	}
	if name != "Test Series Vol. 02.epub" {
		t.Errorf("name = %q, want the volume's name", name)
	}

	_, contents := readEPUB(t, filepath.Join(dir, name))
	opf := readOPF(t, contents)
	if opf.Metadata.Title != "Test Series Vol. 02" {
		t.Errorf("title = %q, want the file name", opf.Metadata.Title)
	}
	if got := metaValue(opf.Metadata.Metas, "belongs-to-collection"); got != "Test Series" {
		t.Errorf("belongs-to-collection = %q, want the series", got)
	}
	if got := metaValue(opf.Metadata.Metas, "group-position"); got != "2" {
		t.Errorf("group-position = %q, want the volume number 2", got)
	}
	if opf.Spine.Direction != ReadingDirectionLTR {
		t.Errorf("page-progression-direction = %q, want ltr", opf.Spine.Direction)
	}

	// one table of contents entry per chapter, each at its first page
	wantNav := []string{"Chapter 1 - Start|xhtml/0001.xhtml", "Chapter 2|xhtml/0003.xhtml"}
	if got := navEntries(t, contents); !reflect.DeepEqual(got, wantNav) {
		t.Errorf("nav = %v, want %v", got, wantNav)
	}
}

func TestPackSingleEPUB(t *testing.T) {
	// no reading direction set means right to left
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault, format: FormatEPUB}
	chapter := &DownloadedChapter{
		Chapter: &grabber.Chapter{Number: 10.5, Title: "Final Choice"},
		Files: []*downloader.File{
			{Data: pngBytes(t, gradientImage(8, 12))},
			{Data: jpegBytes(t, gradientImage(6, 10))},
		},
	}

	dir := t.TempDir()
	name, err := PackSingle(dir, site, chapter, func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackSingle: %v", err)
	}
	if name != "Test Series 10.5 - Final Choice.epub" {
		t.Errorf("name = %q", name)
	}

	_, contents := readEPUB(t, filepath.Join(dir, name))
	opf := readOPF(t, contents)
	if got := metaValue(opf.Metadata.Metas, "group-position"); got != "10.5" {
		t.Errorf("group-position = %q, want 10.5", got)
	}
	if opf.Spine.Direction != ReadingDirectionRTL {
		t.Errorf("page-progression-direction = %q, want rtl", opf.Spine.Direction)
	}

	// a single chapter's pages have no folder, so its one entry is the chapter's label
	wantNav := []string{"Chapter 10.5 - Final Choice|xhtml/0001.xhtml"}
	if got := navEntries(t, contents); !reflect.DeepEqual(got, wantNav) {
		t.Errorf("nav = %v, want %v", got, wantNav)
	}
	for _, want := range []string{"OEBPS/images/0001.png", "OEBPS/images/0002.jpg"} {
		if _, ok := contents[want]; !ok {
			t.Errorf("the archive has no %s", want)
		}
	}
}

// editionChapter is a downloaded chapter with a page, for identity tests
func editionChapter(t *testing.T, number float64, title string) *DownloadedChapter {
	t.Helper()
	return &DownloadedChapter{
		Chapter: &grabber.Chapter{Number: number, Title: title},
		Files:   []*downloader.File{{Data: pngBytes(t, gradientImage(4, 4))}},
	}
}

// identityOf packs with a template that ignores the chapters, so every book gets
// the same file name, and returns the book's identifier.
func identityOf(t *testing.T, pack func(dir string, site *fakeSite) (string, error)) string {
	t.Helper()
	dir := t.TempDir()
	site := &fakeSite{title: "Test Series", template: "{{.Series}}", format: FormatEPUB}
	name, err := pack(dir, site)
	if err != nil {
		t.Fatalf("packing: %v", err)
	}
	return bookIdentifier(t, dir, name)
}

func TestEPUBIdentifierBundlePermutation(t *testing.T) {
	a, b := editionChapter(t, 1, "Start"), editionChapter(t, 2, "Next")
	forward := identityOf(t, func(dir string, site *fakeSite) (string, error) {
		return PackBundle(dir, site, []*DownloadedChapter{a, b}, "1-2", func(page, progress int) {})
	})
	backward := identityOf(t, func(dir string, site *fakeSite) (string, error) {
		return PackBundle(dir, site, []*DownloadedChapter{b, a}, "1-2", func(page, progress int) {})
	})
	if forward != backward {
		t.Errorf("the same chapters in another order gave %q, want %q", backward, forward)
	}
}

func TestEPUBIdentifierBundleMembership(t *testing.T) {
	a, b := editionChapter(t, 1, "Start"), editionChapter(t, 2, "Next")
	both := identityOf(t, func(dir string, site *fakeSite) (string, error) {
		return PackBundle(dir, site, []*DownloadedChapter{a, b}, "1-2", func(page, progress int) {})
	})
	only := identityOf(t, func(dir string, site *fakeSite) (string, error) {
		return PackBundle(dir, site, []*DownloadedChapter{a}, "1", func(page, progress int) {})
	})
	if both == only {
		t.Error("a bundle of two chapters and a bundle of one share the identifier")
	}
}

func TestEPUBIdentifierEditionsDiffer(t *testing.T) {
	// the same number with another title is another release of it
	plain := editionChapter(t, 1, "Start")
	group := editionChapter(t, 1, "Start [Other Group]")

	single := func(c *DownloadedChapter) string {
		return identityOf(t, func(dir string, site *fakeSite) (string, error) {
			return PackSingle(dir, site, c, func(page, progress int) {})
		})
	}
	if single(plain) == single(group) {
		t.Error("two releases of chapter 1 share an identifier as singles")
	}

	bundle := func(c *DownloadedChapter) string {
		return identityOf(t, func(dir string, site *fakeSite) (string, error) {
			return PackBundle(dir, site, []*DownloadedChapter{c}, "1", func(page, progress int) {})
		})
	}
	if bundle(plain) == bundle(group) {
		t.Error("two releases of chapter 1 share an identifier in a bundle")
	}

	volume := func(c *DownloadedChapter) string {
		return identityOf(t, func(dir string, site *fakeSite) (string, error) {
			return PackVolume(dir, site, 1, []*DownloadedChapter{c}, func(page, progress int) {})
		})
	}
	if volume(plain) == volume(group) {
		t.Error("two releases of chapter 1 share an identifier in a volume")
	}
}

func TestEPUBIdentifierVolumeMembers(t *testing.T) {
	// the same volume number with other chapters is another volume
	first := []*DownloadedChapter{editionChapter(t, 1, "A"), editionChapter(t, 2, "B")}
	second := []*DownloadedChapter{editionChapter(t, 3, "C"), editionChapter(t, 4, "D")}
	idOf := func(chapters []*DownloadedChapter) string {
		return identityOf(t, func(dir string, site *fakeSite) (string, error) {
			return PackVolume(dir, site, 2, chapters, func(page, progress int) {})
		})
	}
	if idOf(first) == idOf(second) {
		t.Error("volume 2 with other chapters shares the identifier")
	}
}

// releaseChapter is chapter 1 with the given source ID and display title
func releaseChapter(t *testing.T, id, title string) *DownloadedChapter {
	c := editionChapter(t, 1, title)
	c.ID = id
	return c
}

// identifiersOf packs c as a single, a bundle and a volume
func identifiersOf(t *testing.T, c *DownloadedChapter) [3]string {
	t.Helper()
	progress := func(page, progress int) {}
	return [3]string{
		identityOf(t, func(dir string, site *fakeSite) (string, error) { return PackSingle(dir, site, c, progress) }),
		identityOf(t, func(dir string, site *fakeSite) (string, error) {
			return PackBundle(dir, site, []*DownloadedChapter{c}, "1", progress)
		}),
		identityOf(t, func(dir string, site *fakeSite) (string, error) {
			return PackVolume(dir, site, 1, []*DownloadedChapter{c}, progress)
		}),
	}
}

func TestEPUBIdentifierSourceID(t *testing.T) {
	kinds := []string{"single", "bundle", "volume"}
	a := identifiersOf(t, releaseChapter(t, "id-a", "Same"))
	b := identifiersOf(t, releaseChapter(t, "id-b", "Same"))
	tagged := identifiersOf(t, releaseChapter(t, "id-a", "Same [Group]"))
	for i, kind := range kinds {
		if a[i] == b[i] {
			t.Errorf("%s: releases with other IDs and the same title share an identifier", kind)
		}
		if a[i] != tagged[i] {
			t.Errorf("%s: the same ID with another display title changed the identifier", kind)
		}
	}
}

func TestEPUBIdentifierWithoutIDUsesTitle(t *testing.T) {
	plain := identifiersOf(t, releaseChapter(t, "", "Same"))
	group := identifiersOf(t, releaseChapter(t, "", "Same [Group]"))
	again := identifiersOf(t, releaseChapter(t, "", "Same"))
	for i := range plain {
		if plain[i] == group[i] {
			t.Errorf("kind %d: other titles without an ID share an identifier", i)
		}
		if plain[i] != again[i] {
			t.Errorf("kind %d: the same title without an ID changed the identifier", i)
		}
	}
}

func TestChapterKeys(t *testing.T) {
	ch := func(n float64, lang, id, title string) *DownloadedChapter {
		return &DownloadedChapter{Chapter: &grabber.Chapter{Number: n, Language: lang, ID: id, Title: title}}
	}
	cases := []struct {
		name     string
		a, b     []*DownloadedChapter
		wantSame bool
	}{
		{"decimal differs from whole", []*DownloadedChapter{ch(1, "en", "", "T")}, []*DownloadedChapter{ch(1.5, "en", "", "T")}, false},
		{"decimals 10.5 and 10.50 are one number", []*DownloadedChapter{ch(10.5, "en", "x", "")}, []*DownloadedChapter{ch(10.50, "en", "x", "")}, true},
		{"duplicates count", []*DownloadedChapter{ch(1, "en", "x", "")}, []*DownloadedChapter{ch(1, "en", "x", ""), ch(1, "en", "x", "")}, false},
		{"language only", []*DownloadedChapter{ch(1, "en", "x", "")}, []*DownloadedChapter{ch(1, "es", "x", "")}, false},
		{
			"same-number permutation",
			[]*DownloadedChapter{ch(2, "en", "b", ""), ch(1, "en", "z", ""), ch(1, "en", "a", "")},
			[]*DownloadedChapter{ch(1, "en", "a", ""), ch(1, "en", "z", ""), ch(2, "en", "b", "")},
			true,
		},
		{"ID wins over title", []*DownloadedChapter{ch(1, "en", "x", "A")}, []*DownloadedChapter{ch(1, "en", "x", "B")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := memberKeys(c.a) == memberKeys(c.b); got != c.wantSame {
				t.Errorf("same keys = %v, want %v", got, c.wantSame)
			}
		})
	}
	if chapterKey(ch(1, "en", "", "T").Chapter) == chapterKey(ch(1, "en", "id", "T").Chapter) {
		t.Error("a key from the title equals one from an ID")
	}
}

// a chapter with neither an ID nor a group keeps the key master gave it, so the
// identifiers of books from sites that name no groups don't change
func TestChapterKeyUnchangedWithoutIDOrGroup(t *testing.T) {
	c := &grabber.Chapter{Number: 1, Language: "en", Title: "Chapter 1"}
	if got, want := chapterKey(c), "1\x00en\x00Chapter 1"; got != want {
		t.Errorf("chapterKey() = %q, want %q", got, want)
	}
}

// without an ID, the group tells two releases of one chapter apart, even when
// they carry the same title
func TestChapterKeyWithoutIDUsesTheGroup(t *testing.T) {
	release := func(group, id string) *grabber.Chapter {
		return &grabber.Chapter{Number: 1, Language: "en", Title: "Chapter 1", Group: group, ID: id}
	}
	alpha, delta := chapterKey(release("Alpha", "")), chapterKey(release("Delta", ""))
	if alpha == delta {
		t.Error("two groups' releases with the same title share a key")
	}
	if alpha != chapterKey(release("Alpha", "")) {
		t.Error("the key isn't deterministic")
	}
	// an ID still wins over the group
	if chapterKey(release("Alpha", "x")) != chapterKey(release("Delta", "x")) {
		t.Error("releases with the same ID must share a key, whatever their group")
	}
}
