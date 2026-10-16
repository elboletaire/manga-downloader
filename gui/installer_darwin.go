// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build darwin

package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	// darwinAppName is the bundle the release zip holds; an installed copy keeps its own name
	darwinAppName = "Manga Downloader.app"
	// darwinBundleID must be in the new bundle's Info.plist, so a zip of another app is refused
	darwinBundleID = "com.github.elboletaire.mangadownloader"
	// darwinExe is the executable's path inside the bundle
	darwinExe = "Contents/MacOS/manga-downloader-gui"
)

// caskroomDirs are the Caskroom folders of the Homebrew casks that install
// this app. While one exists a cask is installed, and brew's upgrade owns the
// copy, so the release link stays.
var caskroomDirs = []string{
	"/opt/homebrew/Caskroom/manga-downloader-app",
	"/opt/homebrew/Caskroom/manga-downloader",
	"/usr/local/Caskroom/manga-downloader-app",
	"/usr/local/Caskroom/manga-downloader",
}

// packageName is the release zip that updates this platform
func packageName(tag, _ string) string { return darwinZipName(tag) }

// installedCopy reports whether the running exe is an app bundle this user can
// replace in place
func installedCopy() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	return installedDarwin(exe, caskroomDirs, userCanWrite)
}

// userCanWrite reports whether this user can change path. It's false on a
// read-only volume, such as a mounted disk image.
func userCanWrite(path string) bool {
	return unix.Access(path, unix.W_OK) == nil
}

// installedDarwin reports whether exe, symlinks resolved, runs from an app
// bundle that can be replaced in place: not translocated, not one of casks,
// and writable by this user (its folder and itself, as the swap renames both).
func installedDarwin(exe string, casks []string, writable func(string) bool) bool {
	exe, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return false
	}
	bundle, ok := appBundle(exe)
	if !ok || strings.Contains(exe, "/AppTranslocation/") {
		return false
	}
	for _, c := range casks {
		if fileExists(c) {
			return false
		}
	}

	return writable(filepath.Dir(bundle)) && writable(bundle)
}

// appBundle returns the bundle that exe runs from, when exe is its
// Contents/MacOS executable
func appBundle(exe string) (string, bool) {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(bundle, ".app") || filepath.Join(bundle, darwinExe) != exe {
		return "", false
	}

	return bundle, true
}

// runInstaller replaces the running bundle with the one in the release zip,
// then opens it again, detached, so it outlives this process
func runInstaller(zip string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	target, ok := appBundle(exe)
	if !ok {
		return errors.New("the app isn't running from an app bundle")
	}
	if err := installBundle(zip, target); err != nil {
		return err
	}
	if err := reopen(target); err != nil {
		return fmt.Errorf("%w: %v", errNotRestarted, err)
	}

	return nil
}

// renamePath is os.Rename, a seam for the swap's failure paths in the tests
var renamePath = os.Rename

// installBundle unpacks the zip beside itself, checks the bundle and swaps it
// in for target. The copy is made in target's folder, since a rename can't
// cross volumes and the zip's folder may be on another one.
func installBundle(zip, target string) error {
	parent, name := filepath.Dir(target), filepath.Base(target)
	// target exists, since this runs from it, so the copies left by an
	// interrupted swap are never the only app
	sweepSiblings(parent, name)

	dest := filepath.Join(filepath.Dir(zip), "extracted")
	if err := os.Mkdir(dest, 0o700); err != nil {
		return err
	}
	if err := runTool("/usr/bin/ditto", "-x", "-k", zip, dest); err != nil {
		return fmt.Errorf("couldn't unpack the update: %w", err)
	}
	app := filepath.Join(dest, darwinAppName)
	if err := validateBundle(app); err != nil {
		return fmt.Errorf("the update isn't this app: %w", err)
	}

	fresh, err := siblingPath(parent, name, "new")
	if err != nil {
		return err
	}
	if err := runTool("/usr/bin/ditto", app, fresh); err != nil {
		_ = os.RemoveAll(fresh)

		return fmt.Errorf("couldn't copy the update: %w", err)
	}
	old, err := siblingPath(parent, name, "old")
	if err != nil {
		_ = os.RemoveAll(fresh)

		return err
	}
	if err := renamePath(target, old); err != nil {
		_ = os.RemoveAll(fresh)

		return err
	}
	if err := renamePath(fresh, target); err != nil {
		_ = os.RemoveAll(fresh)
		if rerr := renamePath(old, target); rerr != nil {
			return fmt.Errorf("%w; couldn't move the old app back, it's at %s: %v", err, old, rerr)
		}

		return err
	}
	// a downloaded bundle is quarantined, which stops Gatekeeper from running it
	_ = runTool("/usr/bin/xattr", "-dr", "com.apple.quarantine", target)
	// macOS keeps the files the running process has mapped, so the old bundle can go now
	_ = os.RemoveAll(old)

	return nil
}

// sweepSiblings removes the hidden copies and moved-aside bundles of name that
// an interrupted swap left in dir
func sweepSiblings(dir, name string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() || !(strings.HasPrefix(n, "."+name+".new-") || strings.HasPrefix(n, "."+name+".old-")) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, n))
	}
}

// validateBundle refuses a bundle that isn't this app: no executable, another
// bundle id, or any symlink, since a chain of relative ones can climb out of
// the app and the copy keeps links as they are
func validateBundle(app string) error {
	info, err := os.Lstat(filepath.Join(app, darwinExe))
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("it has no executable")
	}
	plist, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil || !strings.Contains(string(plist), darwinBundleID) {
		return errors.New("its bundle id isn't " + darwinBundleID)
	}

	return filepath.WalkDir(app, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(app, path)

			return fmt.Errorf("it has a symlink at %s", rel)
		}

		return nil
	})
}

// siblingPath is an unused hidden name in dir for a copy or a bundle moved
// aside during the swap
func siblingPath(dir, name, kind string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return filepath.Join(dir, fmt.Sprintf(".%s.%s-%x", name, kind, b)), nil
}

// runTool runs a macOS tool; the last line of its output is the error's detail
func runTool(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, lastLine(string(out)))
	}

	return nil
}

// reopen opens the bundle in a new instance after a second, once this process
// has quit, detached so it outlives this process
func reopen(app string) error {
	cmd := exec.Command("/bin/sh", "-c", `sleep 1; exec /usr/bin/open -n "$0"`, app)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Process.Release()
}
