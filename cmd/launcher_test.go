// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldOpenFromExplorer(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		fromExplorer bool
		want         bool
	}{
		{"double-click, no args", nil, true, true},
		{"terminal, no args", nil, false, false},
		{"double-click but with args", []string{"--help"}, true, false},
		{"terminal with args", []string{"https://example.com"}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldOpenFromExplorer(tt.args, tt.fromExplorer); got != tt.want {
				t.Errorf("shouldOpenFromExplorer(%v, %v) = %v, want %v", tt.args, tt.fromExplorer, got, tt.want)
			}
		})
	}
}

func TestFindGUIIn(t *testing.T) {
	dir := t.TempDir()

	if _, ok := findGUIIn(dir); ok {
		t.Fatalf("found a GUI in an empty directory")
	}

	// a directory with the GUI's name must not count as the binary
	if err := os.Mkdir(filepath.Join(dir, guiFileName()), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := findGUIIn(dir); ok {
		t.Fatalf("a directory named like the GUI was accepted")
	}
	if err := os.Remove(filepath.Join(dir, guiFileName())); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, guiFileName())
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := findGUIIn(dir)
	if !ok || got != want {
		t.Errorf("findGUIIn() = %q, %v; want %q, true", got, ok, want)
	}
}
