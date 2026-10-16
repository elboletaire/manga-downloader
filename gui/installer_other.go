// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows && !linux && !darwin

package main

import "errors"

// packageName is "": only Windows, Linux and macOS have a one-click update,
// each in its own installer_*.go.
func packageName(string, string) string { return "" }

func installedCopy() bool { return false }

func runInstaller(string) error {
	return errors.New("one-click update isn't available on this system")
}
