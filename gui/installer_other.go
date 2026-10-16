// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows && !linux

package main

import "errors"

// packageName is "": only Windows and Linux have a one-click update. A macOS
// implementation goes in its own installer_darwin.go.
func packageName(string, string) string { return "" }

func installedCopy() bool { return false }

func runInstaller(string) error {
	return errors.New("one-click update isn't available on this system")
}
