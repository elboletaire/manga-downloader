// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// verify-cbz checks that the given .cbz and .cbr files are real chapter
// archives: they must exist, contain at least MinPages entries, and every
// entry must be a non-empty image (JPG/PNG/GIF/WebP, checked by magic bytes)
// whose extension matches its actual content. Both formats go through the
// same checks; a .cbr is read with go-rar, anything else as a zip.
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
// Usage: go run ./tools/verify-cbz [-allow-avif] file1.cbz [file2.cbr ...]
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	rar "github.com/elboletaire/go-rar"
	"github.com/elboletaire/manga-downloader/packer/imgfmt"
)

// MinPages is the minimum number of image entries a chapter archive must
// contain to be considered valid.
const MinPages = 3

// entry is one file of an archive, read into memory for the checks.
type entry struct {
	name string
	data []byte
}

var allowAvif = flag.Bool("allow-avif", false, "accept AVIF entries (for archives downloaded with --convert-images=none)")

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: verify-cbz [-allow-avif] file1.cbz [file2.cbr ...]")
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

// verify reads the archive at path and checks its entries.
func verify(path string) error {
	var (
		entries []entry
		err     error
	)
	if strings.EqualFold(filepath.Ext(path), ".cbr") {
		entries, err = readCBR(path)
	} else {
		entries, err = readCBZ(path)
	}
	if err != nil {
		return err
	}

	return checkEntries(entries)
}

// readCBZ reads every non-directory entry of the zip at path.
func readCBZ(path string) ([]entry, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open as zip: %w", err)
	}
	defer r.Close()

	var entries []entry
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		data, err := readEntry(f.Name, f.Open)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry{name: f.Name, data: data})
	}

	return entries, nil
}

// readCBR reads every non-directory entry of the RAR at path. Compressed
// entries fail to open (go-rar only reads stored ones), which is a failure too.
func readCBR(path string) ([]entry, error) {
	r, err := rar.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open as rar: %w", err)
	}
	defer r.Close()

	var entries []entry
	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		data, err := readEntry(f.Name, f.Open)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry{name: f.Name, data: data})
	}

	return entries, nil
}

// readEntry reads one entry to its end, so the archive's own checksum (go-rar
// verifies CRC32 at EOF) is checked too.
func readEntry(name string, open func() (io.ReadCloser, error)) ([]byte, error) {
	rc, err := open()
	if err != nil {
		return nil, fmt.Errorf("entry %q: %w", name, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("entry %q: %w", name, err)
	}

	return data, nil
}

// checkEntries applies the archive checks to entries read from either format.
func checkEntries(entries []entry) error {
	images := 0
	for _, e := range entries {
		if len(e.data) == 0 {
			return fmt.Errorf("entry %q is empty", e.name)
		}

		format := imgfmt.Sniff(e.data)
		if format == "" {
			return fmt.Errorf("entry %q is not a recognised image", e.name)
		}
		if format == "avif" && !*allowAvif {
			return fmt.Errorf("entry %q is AVIF; it should have been converted to JPEG (pass -allow-avif if it was downloaded with --convert-images=none)", e.name)
		}
		// a page whose name disagrees with its bytes breaks reader software in
		// confusing ways, and is exactly what a half-done conversion produces
		if ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(e.name), ".")); ext != format {
			return fmt.Errorf("entry %q is named %q but its content is %s", e.name, ext, format)
		}

		images++
	}

	if images < MinPages {
		return fmt.Errorf("only %d image entries, expected at least %d", images, MinPages)
	}

	return nil
}
