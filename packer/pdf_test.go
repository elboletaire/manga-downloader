// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
)

func verifyPDF(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading pdf: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-1.4\n")) {
		t.Fatalf("pdf missing %%PDF-1.4 header")
	}
	if !bytes.HasSuffix(data, []byte("%%EOF\n")) {
		t.Fatalf("pdf missing %%%%EOF trailer")
	}

	reStartXref := regexp.MustCompile(`startxref\n(\d+)\n%%EOF\n$`)
	m := reStartXref.FindSubmatch(data)
	if m == nil {
		t.Fatalf("missing startxref")
	}
	xrefOff, err := strconv.Atoi(string(m[1]))
	if err != nil || xrefOff <= 0 || xrefOff >= len(data) {
		t.Fatalf("invalid startxref offset %q", m[1])
	}

	xrefText := string(data[xrefOff:])
	lines := strings.Split(xrefText, "\n")
	if len(lines) < 3 || lines[0] != "xref" {
		t.Fatalf("invalid xref header at %d: %q", xrefOff, xrefText)
	}
	var start, count int
	if _, err := fmt.Sscanf(lines[1], "%d %d", &start, &count); err != nil || start != 0 || count < 2 {
		t.Fatalf("invalid xref range line %q", lines[1])
	}
	if lines[2] != "0000000000 65535 f " {
		t.Fatalf("unexpected free xref entry %q", lines[2])
	}
	for id := 1; id < count; id++ {
		line := lines[2+id]
		if len(line) != 19 || !strings.HasSuffix(line, " 00000 n ") {
			t.Fatalf("invalid xref entry %d: %q", id, line)
		}
		off, err := strconv.Atoi(line[:10])
		if err != nil || off <= 0 || off >= xrefOff {
			t.Fatalf("invalid object %d offset %q", id, line[:10])
		}
		wantPrefix := fmt.Sprintf("%d 0 obj\n", id)
		if !bytes.HasPrefix(data[off:], []byte(wantPrefix)) {
			t.Errorf("object %d at offset %d does not start with %q", id, off, wantPrefix)
		}
	}
	return data
}

func grayImage(w, h int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetGray(x, y, color.Gray{Y: uint8((x + y) * 16)})
		}
	}
	return img
}

func transparentPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if x < 2 {
				img.SetNRGBA(x, y, color.NRGBA{})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
			}
		}
	}
	return pngBytes(t, img)
}

func TestArchivePDFImagesAndStructure(t *testing.T) {
	files := []File{
		{Name: "000.jpg", Data: jpegBytes(t, gradientImage(8, 12))},
		{Name: "001.jpg", Data: jpegBytes(t, grayImage(6, 10))},
		{Name: "002.png", Data: transparentPNGBytes(t)},
		{Name: "003.gif", Data: gifBytes(t, flatImage(5, 7))},
	}

	path := filepath.Join(t.TempDir(), "chapter.pdf")
	pages := 0
	err := ArchivePDF(path, EPUBInfo{Title: "One Piece 1 - Romance Dawn"}, files, func(page, _ int) {
		pages += page
	})
	if err != nil {
		t.Fatalf("ArchivePDF: %v", err)
	}
	if pages != len(files) {
		t.Errorf("progress reported %d pages, want %d", pages, len(files))
	}

	raw := verifyPDF(t, path)
	text := string(raw)
	for _, want := range []string{
		"/Filter /DCTDecode",
		"/Filter /FlateDecode",
		"/ColorSpace /DeviceRGB",
		"/ColorSpace /DeviceGray",
		"/MediaBox [0 0 8 12]",
		"/MediaBox [0 0 6 10]",
		"/MediaBox [0 0 4 4]",
		"/MediaBox [0 0 5 7]",
		"/Direction /R2L",
		"/Count 4",
		"/Title (One Piece 1 - Romance Dawn)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("pdf missing %q", want)
		}
	}
	if strings.Contains(text, "/Type /Outlines") {
		t.Errorf("single chapter pdf unexpectedly includes /Outlines")
	}
}

func TestArchivePDFReadingDirectionAndOutlines(t *testing.T) {
	files := []File{
		{Name: "Chapter 0001/000.png", Data: pngBytes(t, gradientImage(8, 12))},
		{Name: "Chapter 0001/001.jpg", Data: jpegBytes(t, gradientImage(6, 10))},
		{Name: "Chapter 0002/000.png", Data: pngBytes(t, gradientImage(6, 10))},
	}
	info := EPUBInfo{
		Title:            `Tom & Jerry (Vol 1) — 日本語`,
		ReadingDirection: ReadingDirectionLTR,
		Labels: map[string]string{
			"Chapter 0001": "Chapter 1 - Start",
			"Chapter 0002": "Chapter 2 - 日本語",
		},
	}
	path := filepath.Join(t.TempDir(), "bundle.pdf")
	if err := ArchivePDF(path, info, files, func(int, int) {}); err != nil {
		t.Fatalf("ArchivePDF: %v", err)
	}

	text := string(verifyPDF(t, path))
	for _, want := range []string{
		"/Direction /L2R",
		"/Type /Outlines",
		"/PageMode /UseOutlines",
		"/Count 2",
		"/Title (Chapter 1 - Start)",
		"<FEFF",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("pdf missing %q", want)
		}
	}
}

func TestPDFPageSizeScalesOversizedWebtoon(t *testing.T) {
	pw, ph := pdfPageSize(800, 28800)
	if pw != "400" || ph != "14400" {
		t.Errorf("pdfPageSize(800, 28800) = (%s, %s), want (400, 14400)", pw, ph)
	}
}

func TestPageFormatsConvertsAVIFAndWebPForPDF(t *testing.T) {
	always, whenSmaller := pageFormats(FormatPDF, grabber.ConvertFormats{}, false)
	if !always.Has(grabber.ConvertAVIF) || !always.Has(grabber.ConvertWebP) {
		t.Fatalf("pageFormats(FormatPDF) always = %v, want avif and webp", always)
	}
	if len(whenSmaller) != 0 {
		t.Fatalf("whenSmaller = %v, want empty when compress=false", whenSmaller)
	}
}

func TestPackSingleBundleVolumePDFWithAVIFAndWebP(t *testing.T) {
	avif := avifBytes(t, gradientImage(8, 8))
	png := pngBytes(t, gradientImage(8, 12))
	chapters := []*DownloadedChapter{
		{
			Chapter: &grabber.Chapter{Number: 1, Title: "Start"},
			Files: []*downloader.File{
				{Data: avif},
				{Data: tinyWebP},
			},
		},
		{
			Chapter: &grabber.Chapter{Number: 2, Title: "Next"},
			Files:   []*downloader.File{{Data: png}},
		},
	}
	site := &fakeSite{
		title:     "Test Series",
		template:  FilenameTemplateDefault,
		format:    FormatPDF,
		direction: ReadingDirectionRTL,
		convert:   grabber.ConvertFormats{}, // even with none, PDF converts AVIF & WebP
	}

	dir := t.TempDir()
	singleName, err := PackSingle(dir, site, chapters[0], func(int, int) {})
	if err != nil {
		t.Fatalf("PackSingle: %v", err)
	}
	if singleName != "Test Series 1 - Start.pdf" {
		t.Errorf("PackSingle = %q, want %q", singleName, "Test Series 1 - Start.pdf")
	}
	verifyPDF(t, filepath.Join(dir, singleName))

	bundleName, err := PackBundle(dir, site, chapters, "1-2", func(int, int) {})
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}
	if bundleName != "Test Series 1-2 - bundle.pdf" {
		t.Errorf("PackBundle = %q, want %q", bundleName, "Test Series 1-2 - bundle.pdf")
	}
	verifyPDF(t, filepath.Join(dir, bundleName))

	volName, err := PackVolume(dir, site, 1, "", chapters, func(int, int) {})
	if err != nil {
		t.Fatalf("PackVolume: %v", err)
	}
	if volName != "Test Series Vol. 01 (1-2).pdf" {
		t.Errorf("PackVolume = %q, want %q", volName, "Test Series Vol. 01 (1-2).pdf")
	}
	verifyPDF(t, filepath.Join(dir, volName))
}

func TestArchivePDFSkipsUnsupportedAndFailsWhenEmpty(t *testing.T) {
	warnings := captureWarnings(t)

	dir := t.TempDir()
	partialPath := filepath.Join(dir, "partial.pdf")
	files := []File{
		{Name: "Chapter 0001/000.avif", Data: []byte("not-converted")},
		{Name: "Chapter 0002/000.jpg", Data: []byte("corrupt-jpeg")},
		{Name: "Chapter 0003/000.png", Data: pngBytes(t, gradientImage(4, 4))},
	}
	pages := 0
	err := ArchivePDF(partialPath, EPUBInfo{Title: "Partial"}, files, func(p, _ int) { pages += p })
	if err != nil {
		t.Fatalf("ArchivePDF partial: %v", err)
	}
	if pages != 3 {
		t.Errorf("progress pages = %d, want 3 (including skipped)", pages)
	}
	if len(*warnings) < 2 {
		t.Errorf("expected warnings for skipped pages/chapters, got %v", *warnings)
	}
	verifyPDF(t, partialPath)

	emptyPath := filepath.Join(dir, "empty.pdf")
	err = ArchivePDF(emptyPath, EPUBInfo{Title: "Empty"}, []File{{Name: "000.avif", Data: []byte("bad")}}, func(int, int) {})
	if err == nil {
		t.Fatal("ArchivePDF succeeded with no valid pages")
	}
	if _, statErr := os.Stat(emptyPath); !os.IsNotExist(statErr) {
		t.Errorf("failed ArchivePDF left file %s behind", emptyPath)
	}
}
