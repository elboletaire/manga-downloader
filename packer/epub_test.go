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

	// mimetype first, stored uncompressed
	if entries[0].Name != "mimetype" || entries[0].Method != zip.Store {
		t.Errorf("first entry = %q (method %d), want mimetype stored", entries[0].Name, entries[0].Method)
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

func TestArchiveEPUBFailsWithoutShowablePages(t *testing.T) {
	warnings := captureWarnings(t)
	path := filepath.Join(t.TempDir(), "book.epub")
	files := []File{{Name: "000.avif", Data: []byte("x")}, {Name: "001.jpg", Data: []byte("y")}}
	if err := ArchiveEPUB(path, EPUBInfo{Title: epubTitle}, files, func(page, progress int) {}); err == nil {
		t.Fatal("expected an error when no page can be shown")
	}
	if len(*warnings) != 2 {
		t.Errorf("got %d warnings, want 2: %q", len(*warnings), *warnings)
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
