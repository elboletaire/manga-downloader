// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledLinux(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(binDir, linuxApp)
	elsewhere := filepath.Join(t.TempDir(), linuxApp)
	for _, p := range []string{installed, elsewhere} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(installed, link); err != nil {
		t.Fatal(err)
	}
	toElsewhere := filepath.Join(binDir, "other")
	if err := os.Symlink(elsewhere, toElsewhere); err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct {
		exe  string
		want bool
	}{
		"installed":     {installed, true},
		"symlink to it": {link, true},
		"another copy":  {elsewhere, false},
		"other name":    {toElsewhere, false},
		"missing":       {filepath.Join(binDir, "gone"), false},
	}
	for name, tc := range cases {
		if got := installedLinux(tc.exe, home); got != tc.want {
			t.Errorf("%s: installedLinux = %v, want %v", name, got, tc.want)
		}
	}
	if installedLinux(installed, t.TempDir()) {
		t.Error("a home without .local/bin counts as installed")
	}
}
