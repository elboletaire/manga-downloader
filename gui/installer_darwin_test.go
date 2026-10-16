// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tempRoot is t.TempDir with its symlinks resolved (/var is /private/var on
// macOS), since installedDarwin compares resolved paths
func tempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return root
}

// makeApp writes dir/Manga Downloader.app, its executable holding body and an
// Info.plist naming bundleID, and returns the bundle's path
func makeApp(t *testing.T, dir, body, bundleID string) string {
	t.Helper()
	app := filepath.Join(dir, darwinAppName)
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, darwinExe), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + bundleID + `</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}

	return app
}

// zipBundle zips app the way the release does, into a new directory of its own
func zipBundle(t *testing.T, app string) string {
	t.Helper()
	zip := filepath.Join(t.TempDir(), "update.zip")
	if out, err := exec.Command("/usr/bin/ditto", "-c", "-k", "--keepParent", app, zip).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v: %s", err, out)
	}

	return zip
}

func TestInstalledDarwin(t *testing.T) {
	root := tempRoot(t)
	apps := filepath.Join(root, "Applications")
	app := makeApp(t, apps, "x", darwinBundleID)
	exe := filepath.Join(app, darwinExe)
	link := filepath.Join(root, "link")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	portable := filepath.Join(root, "portable", "manga-downloader-gui")
	if err := os.MkdirAll(filepath.Dir(portable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(portable, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	translocated := makeApp(t, filepath.Join(root, "AppTranslocation", "ID", "d"), "x", darwinBundleID)
	cask := filepath.Join(root, "Caskroom", "manga-downloader-app")
	if err := os.MkdirAll(cask, 0o755); err != nil {
		t.Fatal(err)
	}
	missingCask := filepath.Join(root, "Caskroom", "manga-downloader")

	all := func(string) bool { return true }
	cases := map[string]struct {
		exe      string
		casks    []string
		writable func(string) bool
		want     bool
	}{
		"installed":          {exe, nil, all, true},
		"symlink to it":      {link, nil, all, true},
		"no app bundle":      {portable, nil, all, false},
		"translocated":       {filepath.Join(translocated, darwinExe), nil, all, false},
		"homebrew caskroom":  {exe, []string{cask}, all, false},
		"caskroom not there": {exe, []string{missingCask}, all, true},
		"parent not writable": {exe, nil, func(p string) bool {
			return p != apps
		}, false},
		"bundle not writable": {exe, nil, func(p string) bool {
			return p != app
		}, false},
		"missing": {filepath.Join(apps, darwinAppName, darwinExe, "gone"), nil, all, false},
	}
	for name, tc := range cases {
		if got := installedDarwin(tc.exe, tc.casks, tc.writable); got != tc.want {
			t.Errorf("%s: installedDarwin = %v, want %v", name, got, tc.want)
		}
	}
}

// refused checks that installBundle turned zip away: the target still holds
// the old app, and the swap left nothing beside it
func refused(t *testing.T, zip, target string) {
	t.Helper()
	if err := installBundle(zip, target); err == nil {
		t.Fatal("installBundle accepted the zip")
	}
	if data, _ := os.ReadFile(filepath.Join(target, darwinExe)); string(data) != "old" {
		t.Errorf("the target changed to %q", data)
	}
	if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
		t.Errorf("the parent holds %d entries after a refusal, want the app only", len(entries))
	}
}

func TestInstallBundle(t *testing.T) {
	root := tempRoot(t)
	target := makeApp(t, filepath.Join(root, "Applications"), "old", darwinBundleID)

	t.Run("replaces the bundle", func(t *testing.T) {
		src := makeApp(t, t.TempDir(), "new", darwinBundleID)
		if err := installBundle(zipBundle(t, src), target); err != nil {
			t.Fatal(err)
		}
		if data, _ := os.ReadFile(filepath.Join(target, darwinExe)); string(data) != "new" {
			t.Errorf("the target holds %q, want the new app", data)
		}
		if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
			t.Errorf("the parent holds %d entries, want the app only", len(entries))
		}
		// the next cases refuse a zip over this bundle
		if err := os.WriteFile(filepath.Join(target, darwinExe), []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("wrong bundle id", func(t *testing.T) {
		src := makeApp(t, t.TempDir(), "new", "com.example.other")
		refused(t, zipBundle(t, src), target)
	})

	t.Run("symlink out of the bundle", func(t *testing.T) {
		src := makeApp(t, t.TempDir(), "new", darwinBundleID)
		secret := filepath.Join(filepath.Dir(src), "secret")
		if err := os.WriteFile(secret, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		resources := filepath.Join(src, "Contents", "Resources")
		if err := os.MkdirAll(resources, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../../../secret", filepath.Join(resources, "out")); err != nil {
			t.Fatal(err)
		}
		refused(t, zipBundle(t, src), target)
	})

	t.Run("symlink inside the bundle", func(t *testing.T) {
		src := makeApp(t, t.TempDir(), "new", darwinBundleID)
		resources := filepath.Join(src, "Contents", "Resources")
		if err := os.MkdirAll(resources, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../Info.plist", filepath.Join(resources, "plist")); err != nil {
			t.Fatal(err)
		}
		refused(t, zipBundle(t, src), target)
	})
}

// failRename makes the swap's second rename (and any rollback after it) fail
// with failures counted from the first rename, the others going through
func failRename(t *testing.T, failures ...int) {
	t.Helper()
	calls := 0
	renamePath = func(from, to string) error {
		calls++
		for _, f := range failures {
			if calls == f {
				return errors.New("disk full")
			}
		}

		return os.Rename(from, to)
	}
	t.Cleanup(func() { renamePath = os.Rename })
}

func TestInstallBundleRollsBack(t *testing.T) {
	root := tempRoot(t)
	parent := filepath.Join(root, "Applications")
	target := makeApp(t, parent, "old", darwinBundleID)
	zip := zipBundle(t, makeApp(t, t.TempDir(), "new", darwinBundleID))

	failRename(t, 2)
	if err := installBundle(zip, target); err == nil {
		t.Fatal("installBundle succeeded though the second rename failed")
	}
	if data, _ := os.ReadFile(filepath.Join(target, darwinExe)); string(data) != "old" {
		t.Errorf("the rollback left %q at the target, want the old app", data)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 1 {
		t.Errorf("the parent holds %d entries after a rollback, want the app only", len(entries))
	}
}

func TestInstallBundleNamesTheOldAppWhenRollbackFails(t *testing.T) {
	root := tempRoot(t)
	parent := filepath.Join(root, "Applications")
	target := makeApp(t, parent, "old", darwinBundleID)
	zip := zipBundle(t, makeApp(t, t.TempDir(), "new", darwinBundleID))

	failRename(t, 2, 3)
	err := installBundle(zip, target)
	if err == nil {
		t.Fatal("installBundle succeeded though both renames failed")
	}
	old := filepath.Join(parent, ".Manga Downloader.app.old-")
	if !strings.Contains(err.Error(), old) {
		t.Errorf("error %q doesn't name the moved-aside app (%s...)", err, old)
	}
}

func TestInstallBundleSweepsStaleCopies(t *testing.T) {
	root := tempRoot(t)
	parent := filepath.Join(root, "Applications")
	target := makeApp(t, parent, "old", darwinBundleID)
	stale := []string{".Manga Downloader.app.new-1a2b", ".Manga Downloader.app.old-3c4d"}
	for _, d := range stale {
		if err := os.MkdirAll(filepath.Join(parent, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// only directories of this app's names are swept
	keep := []string{".Manga Downloader.app.old-file", ".Other.app.new-5e6f"}
	if err := os.WriteFile(filepath.Join(parent, keep[0]), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(parent, keep[1]), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installBundle(zipBundle(t, makeApp(t, t.TempDir(), "new", darwinBundleID)), target); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(target, darwinExe)); string(data) != "new" {
		t.Errorf("the target holds %q, want the new app", data)
	}
	for _, d := range stale {
		if _, err := os.Stat(filepath.Join(parent, d)); !os.IsNotExist(err) {
			t.Errorf("stale %s not swept: %v", d, err)
		}
	}
	for _, k := range keep {
		if _, err := os.Stat(filepath.Join(parent, k)); err != nil {
			t.Errorf("%s was swept: %v", k, err)
		}
	}
}
