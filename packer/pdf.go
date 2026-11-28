// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
)

// pdfMaxPageDim is the PDF 1.4 page coordinate ceiling in points.
const pdfMaxPageDim = 14400.0

// pdfSupportedExt are the image extensions ArchivePDF can embed directly.
// AVIF and WebP are converted to JPEG beforehand via pageFormats.
var pdfSupportedExt = map[string]bool{
	"jpg":  true,
	"jpeg": true,
	"png":  true,
	"gif":  true,
}

type pdfImage struct {
	width      int
	height     int
	colorSpace string
	filter     string
	data       []byte
}

type pdfChapter struct {
	label  string
	pageID int
}

// ArchivePDF packs the named image files into a PDF 1.4 document, one page per
// image. Multi-chapter inputs (bundles, volumes, merged parts) get PDF outline
// bookmarks for each chapter folder.
func ArchivePDF(filename string, info EPUBInfo, files []File, progress func(page, progress int)) (err error) {
	if len(files) == 0 {
		return errors.New("no files to pack")
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(filename)
		}
	}()

	w := newPDFWriter(f)
	if err := w.writeHeader(); err != nil {
		return err
	}

	catalogID := w.reserve()
	pagesID := w.reserve()

	var (
		pageIDs    []int
		chapters   []pdfChapter
		skipped    []skippedPage
		folders    []string
		hasPages   = map[string]bool{}
		lastFolder = ""
	)

	for _, file := range files {
		folder := path.Dir(file.Name)
		if _, seen := hasPages[folder]; !seen {
			folders = append(folders, folder)
			hasPages[folder] = false
		}

		ext := strings.ToLower(strings.TrimPrefix(path.Ext(file.Name), "."))
		if !pdfSupportedExt[ext] {
			skipped = append(skipped, skippedPage{file.Name, fmt.Sprintf("PDF output can't embed %s", ext)})
			progress(1, 0)
			continue
		}

		img, encErr := encodePDFImage(file.Data, ext)
		if encErr != nil {
			skipped = append(skipped, skippedPage{file.Name, fmt.Sprintf("reading image: %s", encErr)})
			progress(1, 0)
			continue
		}

		pw, ph := pdfPageSize(img.width, img.height)
		imgDict := fmt.Sprintf(
			"/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter %s",
			img.width, img.height, img.colorSpace, img.filter,
		)
		imgID, err := w.addStreamObj(imgDict, img.data)
		if err != nil {
			return err
		}

		content := fmt.Sprintf("q\n%s 0 0 %s 0 0 cm\n/Im0 Do\nQ\n", pw, ph)
		contentID, err := w.addStreamObj("", []byte(content))
		if err != nil {
			return err
		}

		pageBody := fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
			pagesID, pw, ph, imgID, contentID,
		)
		pageID, err := w.addObj(pageBody)
		if err != nil {
			return err
		}

		pageIDs = append(pageIDs, pageID)
		hasPages[folder] = true

		if folder != lastFolder {
			chapters = append(chapters, pdfChapter{label: tocLabel(info, folder), pageID: pageID})
			lastFolder = folder
		}

		progress(1, 0)
	}

	warnSkipped(skipped)
	for _, folder := range folders {
		if !hasPages[folder] {
			notice.Warn("- warning: %s has no pages PDF can embed, leaving it out", tocLabel(info, folder))
		}
	}
	if len(pageIDs) == 0 {
		return errors.New("none of the pages can be shown in a PDF")
	}

	outlinesID := 0
	if len(chapters) > 1 {
		outlinesID = w.reserve()
		itemIDs := make([]int, len(chapters))
		for i := range chapters {
			itemIDs[i] = w.reserve()
		}
		for i, ch := range chapters {
			var b strings.Builder
			fmt.Fprintf(&b, "<< /Title %s /Parent %d 0 R /Dest [%d 0 R /Fit]", pdfString(ch.label), outlinesID, ch.pageID)
			if i > 0 {
				fmt.Fprintf(&b, " /Prev %d 0 R", itemIDs[i-1])
			}
			if i+1 < len(chapters) {
				fmt.Fprintf(&b, " /Next %d 0 R", itemIDs[i+1])
			}
			b.WriteString(" >>")
			if err := w.writeObj(itemIDs[i], b.String()); err != nil {
				return err
			}
		}
		outlinesBody := fmt.Sprintf(
			"<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>",
			itemIDs[0], itemIDs[len(itemIDs)-1], len(chapters),
		)
		if err := w.writeObj(outlinesID, outlinesBody); err != nil {
			return err
		}
	}

	infoBody := fmt.Sprintf("<< /Title %s /Creator (Manga Downloader) >>", pdfString(info.Title))
	infoID, err := w.addObj(infoBody)
	if err != nil {
		return err
	}

	var kids strings.Builder
	for i, id := range pageIDs {
		if i > 0 {
			kids.WriteByte(' ')
		}
		fmt.Fprintf(&kids, "%d 0 R", id)
	}
	if err := w.writeObj(pagesID, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pageIDs))); err != nil {
		return err
	}

	dir := "/R2L"
	if info.ReadingDirection == ReadingDirectionLTR {
		dir = "/L2R"
	}
	var catalog strings.Builder
	fmt.Fprintf(&catalog, "<< /Type /Catalog /Pages %d 0 R /ViewerPreferences << /Direction %s >>", pagesID, dir)
	if outlinesID > 0 {
		fmt.Fprintf(&catalog, " /Outlines %d 0 R /PageMode /UseOutlines", outlinesID)
	}
	catalog.WriteString(" >>")
	if err := w.writeObj(catalogID, catalog.String()); err != nil {
		return err
	}

	return w.finish(catalogID, infoID)
}

func encodePDFImage(data []byte, ext string) (pdfImage, error) {
	if ext == "jpg" || ext == "jpeg" {
		cfg, fmtName, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return pdfImage{}, err
		}
		if fmtName != "jpeg" {
			return pdfImage{}, fmt.Errorf("unexpected jpeg format %q", fmtName)
		}
		if cfg.Width <= 0 || cfg.Height <= 0 {
			return pdfImage{}, errors.New("invalid image dimensions")
		}
		switch cfg.ColorModel {
		case color.YCbCrModel:
			return pdfImage{width: cfg.Width, height: cfg.Height, colorSpace: "/DeviceRGB", filter: "/DCTDecode", data: data}, nil
		case color.GrayModel:
			return pdfImage{width: cfg.Width, height: cfg.Height, colorSpace: "/DeviceGray", filter: "/DCTDecode", data: data}, nil
		default:
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return pdfImage{}, err
			}
			buf := &bytes.Buffer{}
			if err := jpeg.Encode(buf, flattenAlpha(img), &jpeg.Options{Quality: grabber.JPEGQualityDefault}); err != nil {
				return pdfImage{}, err
			}
			return pdfImage{width: cfg.Width, height: cfg.Height, colorSpace: "/DeviceRGB", filter: "/DCTDecode", data: buf.Bytes()}, nil
		}
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return pdfImage{}, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return pdfImage{}, errors.New("invalid image dimensions")
	}

	img = flattenAlpha(img)
	colorSpace, raw := extractPDFPixels(img)

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(raw); err != nil {
		zw.Close()
		return pdfImage{}, err
	}
	if err := zw.Close(); err != nil {
		return pdfImage{}, err
	}

	return pdfImage{
		width:      w,
		height:     h,
		colorSpace: colorSpace,
		filter:     "/FlateDecode",
		data:       compressed.Bytes(),
	}, nil
}

func extractPDFPixels(img image.Image) (colorSpace string, raw []byte) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	switch src := img.(type) {
	case *image.Gray:
		raw = make([]byte, w*h)
		for y := 0; y < h; y++ {
			off := src.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			copy(raw[y*w:(y+1)*w], src.Pix[off:off+w])
		}
		return "/DeviceGray", raw
	case *image.RGBA:
		raw = make([]byte, w*h*3)
		dst := 0
		for y := 0; y < h; y++ {
			off := src.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			for x := 0; x < w; x++ {
				p := off + x*4
				raw[dst] = src.Pix[p]
				raw[dst+1] = src.Pix[p+1]
				raw[dst+2] = src.Pix[p+2]
				dst += 3
			}
		}
		return "/DeviceRGB", raw
	case *image.NRGBA:
		raw = make([]byte, w*h*3)
		dst := 0
		for y := 0; y < h; y++ {
			off := src.PixOffset(bounds.Min.X, bounds.Min.Y+y)
			for x := 0; x < w; x++ {
				p := off + x*4
				raw[dst] = src.Pix[p]
				raw[dst+1] = src.Pix[p+1]
				raw[dst+2] = src.Pix[p+2]
				dst += 3
			}
		}
		return "/DeviceRGB", raw
	default:
		raw = make([]byte, w*h*3)
		dst := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				raw[dst] = uint8(r >> 8)
				raw[dst+1] = uint8(g >> 8)
				raw[dst+2] = uint8(b >> 8)
				dst += 3
			}
		}
		return "/DeviceRGB", raw
	}
}

func pdfPageSize(width, height int) (string, string) {
	w, h := float64(width), float64(height)
	if w > pdfMaxPageDim || h > pdfMaxPageDim {
		scale := pdfMaxPageDim / w
		if h > w {
			scale = pdfMaxPageDim / h
		}
		w *= scale
		h *= scale
	}
	return formatPDFDim(w), formatPDFDim(h)
}

func formatPDFDim(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

func pdfString(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] > 126 {
			ascii = false
			break
		}
	}
	if ascii {
		var b strings.Builder
		b.WriteByte('(')
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case '\\', '(', ')':
				b.WriteByte('\\')
			}
			b.WriteByte(s[i])
		}
		b.WriteByte(')')
		return b.String()
	}

	units := utf16.Encode([]rune(s))
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range units {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteByte('>')
	return b.String()
}

type pdfWriter struct {
	bw      *bufio.Writer
	pos     int64
	offsets []int64 // 1-based object IDs stored at index id-1
}

func newPDFWriter(w io.Writer) *pdfWriter {
	return &pdfWriter{bw: bufio.NewWriter(w)}
}

func (p *pdfWriter) writeBytes(b []byte) error {
	n, err := p.bw.Write(b)
	p.pos += int64(n)
	return err
}

func (p *pdfWriter) writeStr(s string) error {
	n, err := p.bw.WriteString(s)
	p.pos += int64(n)
	return err
}

func (p *pdfWriter) writeHeader() error {
	return p.writeBytes([]byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n"))
}

func (p *pdfWriter) reserve() int {
	p.offsets = append(p.offsets, 0)
	return len(p.offsets)
}

func (p *pdfWriter) writeObj(id int, body string) error {
	p.offsets[id-1] = p.pos
	return p.writeStr(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", id, body))
}

func (p *pdfWriter) addObj(body string) (int, error) {
	id := p.reserve()
	return id, p.writeObj(id, body)
}

func (p *pdfWriter) addStreamObj(dict string, data []byte) (int, error) {
	id := p.reserve()
	p.offsets[id-1] = p.pos
	var header string
	if dict == "" {
		header = fmt.Sprintf("%d 0 obj\n<< /Length %d >>\nstream\n", id, len(data))
	} else {
		header = fmt.Sprintf("%d 0 obj\n<< %s /Length %d >>\nstream\n", id, dict, len(data))
	}
	if err := p.writeStr(header); err != nil {
		return 0, err
	}
	if err := p.writeBytes(data); err != nil {
		return 0, err
	}
	if err := p.writeStr("\nendstream\nendobj\n"); err != nil {
		return 0, err
	}
	return id, nil
}

func (p *pdfWriter) finish(catalogID, infoID int) error {
	xrefPos := p.pos
	count := len(p.offsets) + 1
	if err := p.writeStr(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", count)); err != nil {
		return err
	}
	for _, off := range p.offsets {
		if err := p.writeStr(fmt.Sprintf("%010d 00000 n \n", off)); err != nil {
			return err
		}
	}
	trailer := fmt.Sprintf(
		"trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		count, catalogID, infoID, xrefPos,
	)
	if err := p.writeStr(trailer); err != nil {
		return err
	}
	return p.bw.Flush()
}
