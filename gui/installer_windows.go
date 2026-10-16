// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build windows

package main

import (
	"errors"
	"os"
	"sync/atomic"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Manga Downloader`

// packageName is the release asset that updates this platform
func packageName(tag, goarch string) string { return windowsInstallerName(tag, goarch) }

// scope is where installedCopy found the app; set once at startup
var scope atomic.Int32

// installedCopy reports whether the running exe is the installer's copy, per
// user or per machine, and remembers which
func installedCopy() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	sc, ok := detectScope(exe, installLocation, fileExists)
	scope.Store(int32(sc))

	return ok
}

// installLocation reads the scope's uninstall key; a per-machine one is in the
// 64-bit view, where the installer writes it. "" when it isn't there.
func installLocation(sc installScope) string {
	root, access := registry.CURRENT_USER, uint32(registry.QUERY_VALUE)
	if sc == scopeMachine {
		root, access = registry.LOCAL_MACHINE, registry.QUERY_VALUE|registry.WOW64_64KEY
	}
	k, err := registry.OpenKey(root, uninstallKey, access)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("InstallLocation")
	if err != nil {
		return ""
	}

	return v
}

// runInstaller starts the installer with /relaunch: "runas" for a per-machine
// install (elevation, which os/exec can't do), "open" for a per-user one.
func runInstaller(path string) error {
	name := "open"
	if installScope(scope.Load()) == scopeMachine {
		name = "runas"
	}
	verb, _ := windows.UTF16PtrFromString(name)
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	args, _ := windows.UTF16PtrFromString("/relaunch")
	err = windows.ShellExecute(0, verb, file, args, nil, windows.SW_NORMAL)
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return errors.New("the update was cancelled at the permission prompt")
	}

	return err
}
