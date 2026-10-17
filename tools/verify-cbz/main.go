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
	"errors"
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

// headSize is what imgfmt.Sniff reads: AVIF and WebP are told apart by bytes 4-11.
const headSize = 12

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

// verify reads the archive at path and checks its entries one at a time.
func verify(path string) error {
	c := &checker{}
	var err error
	if strings.EqualFold(filepath.Ext(path), ".cbr") {
		err = readCBR(path, c.check)
	} else {
		err = readCBZ(path, c.check)
	}
	if err != nil {
		return err
	}

	return c.done()
}

// checker applies the archive checks to one entry at a time, so an archive
// never has to sit in memory.
type checker struct {
	images int
}

// check takes an entry's name and its leading bytes (at most headSize).
func (c *checker) check(name string, head []byte) error {
	if len(head) == 0 {
		return fmt.Errorf("entry %q is empty", name)
	}

	format := imgfmt.Sniff(head)
	if format == "" {
		return fmt.Errorf("entry %q is not a recognised image", name)
	}
	if format == "avif" && !*allowAvif {
		return fmt.Errorf("entry %q is AVIF; it should have been converted to JPEG (pass -allow-avif if it was downloaded with --convert-images=none)", name)
	}
	// a page whose name disagrees with its bytes breaks reader software in
	// confusing ways, and is exactly what a half-done conversion produces
	if ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), ".")); ext != format {
		return fmt.Errorf("entry %q is named %q but its content is %s", name, ext, format)
	}

	c.images++
	return nil
}

// done applies the MinPages rule once every entry has been checked.
func (c *checker) done() error {
	if c.images < MinPages {
		return fmt.Errorf("only %d image entries, expected at least %d", c.images, MinPages)
	}

	return nil
}

// readCBZ calls visit with the head of every non-directory entry of the zip
// at path, stopping at the first error.
func readCBZ(path string, visit func(name string, head []byte) error) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("cannot open as zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := visitEntry(f.Name, f.Open, false, visit); err != nil {
			return err
		}
	}

	return nil
}

// readCBR calls visit with the head of every non-directory entry of the RAR
// at path. Each entry is read to its end, so go-rar checks its CRC32; nothing
// but the head is kept. Compressed entries can't be opened, and that's a failure.
func readCBR(path string, visit func(name string, head []byte) error) error {
	r, err := rar.OpenReader(path)
	if err != nil {
		return fmt.Errorf("cannot open as rar: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := visitEntry(f.Name, f.Open, true, visit); err != nil {
			if errors.Is(err, rar.ErrAlgorithm) {
				return fmt.Errorf("%s is a compressed RAR; only stored archives can be checked: %w", path, err)
			}
			return err
		}
	}

	return nil
}

// visitEntry opens one entry and passes its head to visit. With drain set it
// then reads the rest, so go-rar checks the entry's CRC32 at EOF.
func visitEntry(name string, open func() (io.ReadCloser, error), drain bool, visit func(name string, head []byte) error) error {
	rc, err := open()
	if err != nil {
		return fmt.Errorf("entry %q: %w", name, err)
	}
	defer rc.Close()

	head := make([]byte, headSize)
	n, err := io.ReadFull(rc, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("entry %q: %w", name, err)
	}
	if err := visit(name, head[:n]); err != nil {
		return err
	}
	if !drain {
		return nil
	}

	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("entry %q: %w", name, err)
	}

	return nil
}
