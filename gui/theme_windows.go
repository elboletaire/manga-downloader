// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build windows

package main

import "golang.org/x/sys/windows/registry"

// systemPrefersDark reads the "apps use light theme" switch Windows 10 and 11
// keep per user. A missing key (older Windows) means light.
func systemPrefersDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")

	return err == nil && v == 0
}
