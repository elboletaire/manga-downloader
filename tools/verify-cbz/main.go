// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// verify-cbz checks that the given .cbz files are real chapter archives:
// they must exist, contain at least MinPages entries, and every entry must
// be a non-empty image (JPG/PNG/GIF/WebP/AVIF, checked by magic bytes).
//
// The downloader exits 0 even when individual chapters or pages fail, so
// smoke tests must inspect the produced archives to detect broken sites.
//
// Usage: go run ./tools/verify-cbz file1.cbz [file2.cbz ...]
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
)

// MinPages is the minimum number of image entries a chapter archive must
// contain to be considered valid.
const MinPages = 3

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: verify-cbz file1.cbz [file2.cbz ...]")
		os.Exit(2)
	}

	failed := false
	for _, path := range os.Args[1:] {
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
		ok, err := isImage(f)
		if err != nil {
			return fmt.Errorf("entry %q: %w", f.Name, err)
		}
		if !ok {
			return fmt.Errorf("entry %q is not a recognised image", f.Name)
		}
		images++
	}

	if images < MinPages {
		return fmt.Errorf("only %d image entries, expected at least %d", images, MinPages)
	}

	return nil
}

func isImage(f *zip.File) (bool, error) {
	rc, err := f.Open()
	if err != nil {
		return false, err
	}
	defer rc.Close()

	head := make([]byte, 12)
	n, err := io.ReadFull(rc, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		return false, err
	}
	head = head[:n]

	switch {
	case bytes.HasPrefix(head, []byte{0xff, 0xd8, 0xff}): // JPEG
		return true, nil
	case bytes.HasPrefix(head, []byte{0x89, 'P', 'N', 'G'}): // PNG
		return true, nil
	case bytes.HasPrefix(head, []byte("GIF8")): // GIF
		return true, nil
	case bytes.HasPrefix(head, []byte("RIFF")) && len(head) >= 12 && bytes.Equal(head[8:12], []byte("WEBP")): // WebP
		return true, nil
	// AVIF: ISOBMFF "ftyp" box with an avif/avis brand (same sniff as
	// packer's extFromContent; e.g. atsu.moe and mistscans serve AVIF pages)
	case len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")) &&
		(bytes.Equal(head[8:12], []byte("avif")) || bytes.Equal(head[8:12], []byte("avis"))):
		return true, nil
	}

	return false, nil
}
