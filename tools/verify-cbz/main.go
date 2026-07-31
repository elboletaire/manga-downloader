// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// verify-cbz checks that the given .cbz files are real chapter archives:
// they must exist, contain at least MinPages entries, and every entry must
// be a non-empty image (JPG/PNG/GIF/WebP, checked by magic bytes) whose
// extension matches its actual content.
//
// AVIF entries are a failure by default: the downloader converts AVIF pages
// to JPEG (--convert-images, on by default) precisely because no dedicated
// e-reader can render them, so an AVIF entry means that conversion silently
// didn't happen - the most likely cause being a build that lost its
// "-tags nodynamic". Pass -allow-avif to check an archive downloaded with
// --convert-images=none.
//
// The downloader exits 0 even when individual chapters or pages fail, so
// smoke tests must inspect the produced archives to detect broken sites.
//
// Usage: go run ./tools/verify-cbz [-allow-avif] file1.cbz [file2.cbz ...]
package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MinPages is the minimum number of image entries a chapter archive must
// contain to be considered valid.
const MinPages = 3

var allowAvif = flag.Bool("allow-avif", false, "accept AVIF entries (for archives downloaded with --convert-images=none)")

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: verify-cbz [-allow-avif] file1.cbz [file2.cbz ...]")
		os.Exit(2)
	}

	failed := false
	for _, path := range flag.Args() {
		if err := verify(path); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", path, err)
			failed = true
			continue
		}
		fmt.Printf("OK   %s\n", path)
	}

	if failed {
		os.Exit(1)
	}
}

func verify(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("cannot open as zip: %w", err)
	}
	defer r.Close()

	images := 0
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 == 0 {
			return fmt.Errorf("entry %q is empty", f.Name)
		}

		format, err := imageFormat(f)
		if err != nil {
			return fmt.Errorf("entry %q: %w", f.Name, err)
		}
		if format == "" {
			return fmt.Errorf("entry %q is not a recognised image", f.Name)
		}
		if format == "avif" && !*allowAvif {
			return fmt.Errorf("entry %q is AVIF; it should have been converted to JPEG (pass -allow-avif if it was downloaded with --convert-images=none)", f.Name)
		}
		// a page whose name disagrees with its bytes breaks reader software in
		// confusing ways, and is exactly what a half-done conversion produces
		if ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Name), ".")); ext != format {
			return fmt.Errorf("entry %q is named %q but its content is %s", f.Name, ext, format)
		}

		images++
	}

	if images < MinPages {
		return fmt.Errorf("only %d image entries, expected at least %d", images, MinPages)
	}

	return nil
}

// imageFormat sniffs an entry's image format from its magic bytes, returning
// the same names packer's extFromContent assigns as extensions (so the two can
// be compared directly), or "" when nothing is recognised.
func imageFormat(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	head := make([]byte, 12)
	n, err := io.ReadFull(rc, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}
	head = head[:n]

	switch {
	case bytes.HasPrefix(head, []byte{0xff, 0xd8, 0xff}):
		return "jpg", nil
	case bytes.HasPrefix(head, []byte{0x89, 'P', 'N', 'G'}):
		return "png", nil
	case bytes.HasPrefix(head, []byte("GIF8")):
		return "gif", nil
	case bytes.HasPrefix(head, []byte("RIFF")) && len(head) >= 12 && bytes.Equal(head[8:12], []byte("WEBP")):
		return "webp", nil
	// AVIF: ISOBMFF "ftyp" box with an avif/avis brand (same sniff as packer's
	// extFromContent; e.g. atsu.moe and mistscans serve AVIF pages)
	case len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")) &&
		(bytes.Equal(head[8:12], []byte("avif")) || bytes.Equal(head[8:12], []byte("avis"))):
		return "avif", nil
	}

	return "", nil
}
