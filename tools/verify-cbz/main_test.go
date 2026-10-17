// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/elboletaire/manga-downloader/packer"
)

// jpeg returns bytes that sniff as a JPEG, with a tag so entries differ.
func jpeg(tag string) []byte {
	return append([]byte{0xff, 0xd8, 0xff, 0xe0}, tag...)
}

// writeCBR archives files with packer.ArchiveCBR and returns its path.
func writeCBR(t *testing.T, files []packer.File) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chapter.cbr")
	if err := packer.ArchiveCBR(path, files, func(page, progress int) {}); err != nil {
		t.Fatalf("ArchiveCBR: %v", err)
	}
	return path
}

func TestVerifyCBR(t *testing.T) {
	good := []packer.File{
		{Name: "000.jpg", Data: jpeg("a")},
		{Name: "001.jpg", Data: jpeg("b")},
		{Name: "002.jpg", Data: jpeg("c")},
	}
	html := []byte("<html><body>Cloudflare</body></html>")
	avif := []byte("\x00\x00\x00\x1cftypavif\x00\x00\x00\x00avif")

	tests := []struct {
		name       string
		files      []packer.File
		allowAvif  bool
		wantFailed bool
	}{
		{name: "valid", files: good},
		{name: "html named as jpg", files: append(good[:2:2], packer.File{Name: "002.jpg", Data: html}), wantFailed: true},
		{name: "avif rejected", files: append(good[:2:2], packer.File{Name: "002.avif", Data: avif}), wantFailed: true},
		{name: "avif allowed", files: append(good[:2:2], packer.File{Name: "002.avif", Data: avif}), allowAvif: true},
		{name: "too few pages", files: good[:2], wantFailed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*allowAvif = tt.allowAvif
			defer func() { *allowAvif = false }()

			err := verify(writeCBR(t, tt.files))
			if (err != nil) != tt.wantFailed {
				t.Fatalf("verify() = %v, wantFailed = %v", err, tt.wantFailed)
			}
		})
	}
}

func TestVerifyCBZ(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"000.jpg", "001.jpg", "002.jpg"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(jpeg(name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "chapter.cbz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(path); err != nil {
		t.Fatalf("verify() = %v, want nil", err)
	}
}
