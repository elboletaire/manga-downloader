// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// rarSignature is the RAR 4 marker block every RAR4 archive starts with
var rarSignature = []byte("Rar!\x1a\x07\x00")

// listRAR returns the entry names of the RAR at path, listed by bsdtar or
// 7z, whichever is on PATH. ok is false when neither is, so the caller skips.
func listRAR(t *testing.T, path string) (names []string, ok bool) {
	t.Helper()

	if bsdtar, err := exec.LookPath("bsdtar"); err == nil {
		out, err := exec.Command(bsdtar, "-tf", path).Output()
		if err != nil {
			t.Fatalf("bsdtar -tf: %v", err)
		}
		return lines(out), true
	}
	if sevenZip, err := exec.LookPath("7z"); err == nil {
		out, err := exec.Command(sevenZip, "l", "-slt", path).Output()
		if err != nil {
			t.Fatalf("7z l: %v", err)
		}
		// -slt prints one "Path = name" line per entry, the archive's own first
		var names []string
		for _, line := range lines(out) {
			if name, found := strings.CutPrefix(line, "Path = "); found {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			names = names[1:]
		}
		return names, true
	}
	return nil, false
}

// lines splits out into its lines: one entry per line, where strings.Fields would split a name at its spaces
func lines(out []byte) []string {
	var got []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		got = append(got, sc.Text())
	}
	return got
}

func TestArchiveCBRNoFiles(t *testing.T) {
	err := ArchiveCBR(filepath.Join(t.TempDir(), "empty.cbr"), []File{}, func(page, progress int) {})
	if err == nil {
		t.Fatal("expected an error when archiving zero files, got nil")
	}
}

func TestArchiveCBREntryNames(t *testing.T) {
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

	names, ok := listRAR(t, path)
	if !ok {
		t.Skip("neither bsdtar nor 7z is on PATH, so the entries can't be listed")
	}
	want := []string{"Chapter 0001/000.jpg", "Chapter 0001/001.jpg", "Chapter 0002/000.jpg"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("entries = %v, want %v", names, want)
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
