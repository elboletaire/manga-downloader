// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// linuxApp is the binary's name, which install.sh installs under ~/.local/bin
const linuxApp = "manga-downloader-gui"

// packageName is the release asset that updates this platform
func packageName(tag, goarch string) string { return linuxTarballName(tag, goarch) }

// installedCopy reports whether the running exe is the one install.sh put in
// ~/.local/bin
func installedCopy() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	return installedLinux(exe, home)
}

// installedLinux reports whether exe, symlinks resolved, is home's
// ~/.local/bin/manga-downloader-gui. The bin directory is resolved too, since
// install.sh replaces the file there, never a link.
func installedLinux(exe, home string) bool {
	exe, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return false
	}
	binDir, err := filepath.EvalSymlinks(filepath.Join(home, ".local", "bin"))
	if err != nil {
		return false
	}

	return exe == filepath.Join(binDir, linuxApp)
}

// runInstaller unpacks the tarball beside itself, runs its install.sh (which
// swaps the binary by rename, so the running one is untouched) and starts the
// installed binary again, detached so it outlives this process.
func runInstaller(tarball string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}

	dest := filepath.Join(filepath.Dir(tarball), "extracted")
	if err := os.Mkdir(dest, 0o700); err != nil {
		return err
	}
	if err := extractTarGz(tarball, dest); err != nil {
		return fmt.Errorf("couldn't unpack the update: %w", err)
	}
	script := filepath.Join(dest, strings.TrimSuffix(filepath.Base(tarball), ".tar.gz"), "install.sh")
	if out, err := exec.Command("/bin/sh", script).CombinedOutput(); err != nil {
		return fmt.Errorf("the installer failed: %w: %s", err, lastLine(string(out)))
	}

	if err := relaunch(exe); err != nil {
		return fmt.Errorf("%w: %v", errNotRestarted, err)
	}

	return nil
}

// relaunch starts bin in a new session after a second, once this process has
// quit and released the window
func relaunch(bin string) error {
	cmd := exec.Command("/bin/sh", "-c", `sleep 1; exec "$0"`, bin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Process.Release()
}
