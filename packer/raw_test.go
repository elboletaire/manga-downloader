// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveRaw(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chapter")

	jpeg := append([]byte{0xff, 0xd8, 0xff}, []byte("rest-of-jpeg-data")...)
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("rest-of-png-data")...)

	files := []File{
		{Name: "000.jpg", Data: jpeg},
		{Name: "001.png", Data: png},
	}

	if err := SaveRaw(dir, files, func(page, progress int) {}); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	expected := map[string][]byte{
		"000.jpg": jpeg,
		"001.png": png,
	}

	for name, data := range expected {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("expected file %s to exist: %s", name, err)
		}
		if string(got) != string(data) {
			t.Errorf("file %s content mismatch", name)
		}
	}
}

func TestSaveRawBundleFolders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")

	files := []File{
		{Name: "Chapter 0001/000.jpg", Data: []byte("ch1-p0")},
		{Name: "Chapter 0002/000.jpg", Data: []byte("ch2-p0")},
	}

	if err := SaveRaw(dir, files, func(page, progress int) {}); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	for _, file := range files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file.Name)))
		if err != nil {
			t.Fatalf("expected file %s to exist: %s", file.Name, err)
		}
		if string(got) != string(file.Data) {
			t.Errorf("file %s content mismatch", file.Name)
		}
	}
}

func TestSaveRawExistingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chapter")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %s", err)
	}

	files := []File{
		{Name: "000.jpg", Data: []byte{0xff, 0xd8, 0xff}},
	}

	err := SaveRaw(dir, files, func(page, progress int) {})
	if !os.IsExist(err) {
		t.Fatalf("expected an os.IsExist-compatible error, got: %v", err)
	}
}

func TestSaveRawNoFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chapter")
	if err := SaveRaw(dir, nil, func(page, progress int) {}); err == nil {
		t.Fatal("expected an error when no files are provided")
	}
}

func TestExtFromContent(t *testing.T) {
	cases := []struct {
		data []byte
		want string
	}{
		{append([]byte{0xff, 0xd8, 0xff}, []byte("jpeg-data")...), "jpg"},
		{append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("png-data")...), "png"},
		{[]byte("GIF89a-gif-data"), "gif"},
		{[]byte("not a recognisable image"), "jpg"}, // unrecognised content falls back to jpg
	}

	for _, c := range cases {
		if got := extFromContent(c.data); got != c.want {
			t.Errorf("extFromContent(%q...) = %q, want %q", c.data[:4], got, c.want)
		}
	}
}
