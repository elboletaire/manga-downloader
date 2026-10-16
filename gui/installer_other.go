// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows

package main

import "errors"

// installedCopy is always false: only Windows has an installer
func installedCopy() bool { return false }

func runInstaller(string) error {
	return errors.New("one-click update is only available on Windows")
}
