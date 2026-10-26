// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package update

import (
	"os"
	"regexp"
	"strings"
)

// localeRe is a language, then up to two more subtags (script, region), as BCP 47 has them
var localeRe = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{1,8}){0,2}$`)

// NormalizeLocale trims a locale such as "es_ES.UTF-8" or "ca_ES@valencia" to a
// BCP 47 tag such as "es-ES", at most three subtags. It returns "" for C, POSIX
// and anything that isn't a language tag.
func NormalizeLocale(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	s = strings.ToLower(strings.ReplaceAll(s, "_", "-"))
	if !localeRe.MatchString(s) {
		return ""
	}
	parts := strings.Split(s, "-")
	for i, p := range parts[1:] {
		switch len(p) {
		case 2:
			parts[i+1] = strings.ToUpper(p)
		case 4:
			parts[i+1] = strings.ToUpper(p[:1]) + p[1:]
		}
	}

	return strings.Join(parts, "-")
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
