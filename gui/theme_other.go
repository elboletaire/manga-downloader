// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows && !darwin && !linux

package main

// systemPrefersDark has no settings to read on this platform, so the window
// uses the light background
func systemPrefersDark() bool { return false }
