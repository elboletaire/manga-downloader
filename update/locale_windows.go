// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package update

import "golang.org/x/sys/windows"

// SystemLocale is the user's locale, normalized (see NormalizeLocale): the environment's
// when it names one (Git Bash, MSYS), else the display language Windows prefers
func SystemLocale() string {
	if l := NormalizeLocale(envLocale()); l != "" {
		return l
	}

	return NormalizeLocale(windowsLocale())
}

// windowsLocale is the first UI language Windows prefers for the user, "" when it can't say
func windowsLocale() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(langs) == 0 {
		return ""
	}

	return langs[0]
}
