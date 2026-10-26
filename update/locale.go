// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package update

import (
	"os"
	"strings"
)

// NormalizeLocale trims a locale such as "es_ES.UTF-8", "ca_ES@valencia" or
// "de-DE_phoneb" (Windows' sort suffix) to a BCP 47 tag such as "es-ES": a
// language, then an optional script and region, and nothing after them. It
// returns "" for C, POSIX and anything without a language.
func NormalizeLocale(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(s, "_", "-")), "-")
	if !isAlpha(parts[0], 2, 3) {
		return ""
	}
	out := []string{parts[0]}
	rest := parts[1:]
	if len(rest) > 0 && isAlpha(rest[0], 4, 4) {
		out = append(out, strings.ToUpper(rest[0][:1])+rest[0][1:])
		rest = rest[1:]
	}
	if len(rest) > 0 && (isAlpha(rest[0], 2, 2) || isDigits(rest[0], 3)) {
		out = append(out, strings.ToUpper(rest[0]))
	}

	return strings.Join(out, "-")
}

// isAlpha reports whether s is between lo and hi ASCII letters long
func isAlpha(s string, lo, hi int) bool {
	if len(s) < lo || len(s) > hi {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}

	return true
}

// isDigits reports whether s is exactly n ASCII digits long
func isDigits(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

// envLocale is the first set of LC_ALL, LC_MESSAGES and LANG, unnormalized.
// LANGUAGE is a priority list rather than a locale, so it's left out.
func envLocale() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}

	return ""
}
