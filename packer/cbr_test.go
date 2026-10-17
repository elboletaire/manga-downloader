// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	rar "github.com/elboletaire/go-rar"
)

// rarSignature is the RAR 4 marker block every RAR4 archive starts with
var rarSignature = []byte("Rar!\x1a\x07\x00")

func TestArchiveCBRNoFiles(t *testing.T) {
	err := ArchiveCBR(filepath.Join(t.TempDir(), "empty.cbr"), []File{}, func(page, progress int) {})
	if err == nil {
		t.Fatal("expected an error when archiving zero files, got nil")
	}
}

func TestArchiveCBREntries(t *testing.T) {
	files := []File{
		{Name: "Chapter 0001/000.jpg", Data: []byte("ch1-p0")},
		{Name: "Chapter 0001/001.jpg", Data: []byte("ch1-p1")},
		{Name: "Chapter 0002/000.jpg", Data: []byte("ch2-p0")},
	}

	path := filepath.Join(t.TempDir(), "bundle.cbr")
	pages := 0
	if err := ArchiveCBR(path, files, func(page, progress int) { pages += page }); err != nil {
		t.Fatalf("ArchiveCBR: %v", err)
	}
	if pages != len(files) {
		t.Errorf("progress reported %d pages, want %d", pages, len(files))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, rarSignature) {
		t.Fatalf("archive starts with %q, want the RAR4 signature %q", data[:min(len(data), len(rarSignature))], rarSignature)
	}

	r, err := rar.OpenReader(path)
	if err != nil {
		t.Fatalf("rar.OpenReader: %v", err)
	}
	defer r.Close()

	if len(r.File) != len(files) {
		t.Fatalf("archive has %d entries, want %d", len(r.File), len(files))
	}
	for i, want := range files {
		f := r.File[i]
		if f.Name != want.Name {
			t.Errorf("entry %d is %q, want %q", i, f.Name, want.Name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %q: %v", f.Name, err)
		}
		got, err := io.ReadAll(rc) // also checks the CRC32 at EOF
		rc.Close()
		if err != nil {
			t.Fatalf("read %q: %v", f.Name, err)
		}
		if !bytes.Equal(got, want.Data) {
			t.Errorf("entry %q holds %q, want %q", f.Name, got, want.Data)
		}
	}
}

func TestArchiveCBRRefusesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "taken.cbr")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	err := ArchiveCBR(path, []File{{Name: "000.jpg", Data: []byte("p")}}, func(page, progress int) {})
	if !os.IsExist(err) {
		t.Fatalf("ArchiveCBR over an existing file = %v, want an exists error", err)
	}
}
