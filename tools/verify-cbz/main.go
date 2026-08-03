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
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/elboletaire/manga-downloader/packer/imgfmt"
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

// imageFormat sniffs an entry's image format from its leading bytes via the
// shared packer/imgfmt sniff (the same one packer's extFromContent uses to
// name pages), returning "" when nothing is recognised.
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

	return imgfmt.Sniff(head), nil
}
