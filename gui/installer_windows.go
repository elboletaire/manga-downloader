// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build windows

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Manga Downloader`

// packageName is the release asset that updates this platform
func packageName(tag, goarch string) string { return windowsInstallerName(tag, goarch) }

// installedCopy reports whether the running exe is the installer's copy
func installedCopy() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	return installedAt(exe, installLocation(), fileExists)
}

// installLocation reads the uninstall key from the 64-bit view, where the
// installer writes it; "" when it isn't there
func installLocation() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallKey, registry.QUERY_VALUE|registry.WOW64_64KEY)
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

// runInstaller starts the installer with /relaunch. It asks for elevation
// itself, which CreateProcess (os/exec) can't do (error 740), so it goes
// through ShellExecute's "runas", which shows the UAC prompt.
func runInstaller(path string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
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
