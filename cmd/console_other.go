// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows

package cmd

// launchedFromExplorer is only meaningful on Windows, where double-clicking a
// console program opens a console of its own
func launchedFromExplorer() bool {
	return false
}
