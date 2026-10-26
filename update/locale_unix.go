// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows

package update

// SystemLocale is the user's locale, from the environment, normalized (see NormalizeLocale)
func SystemLocale() string {
	return NormalizeLocale(envLocale())
}
