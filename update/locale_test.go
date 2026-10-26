// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package update

import "testing"

func TestNormalizeLocale(t *testing.T) {
	cases := map[string]string{
		"es_ES.UTF-8":          "es-ES",
		"ca_ES@valencia":       "ca-ES",
		"ca_ES.UTF-8@valencia": "ca-ES",
		"fr-FR":                "fr-FR",
		"  de_DE  ":            "de-DE",
		"en_us":                "en-US",
		"zh-Hant-TW":           "zh-Hant-TW",
		"zh_Hant_TW.UTF-8":     "zh-Hant-TW",
		"es-419":               "es-419",
		"en-US-u-ca-gregory":   "",
		"sr_RS@latin":          "sr-RS",
		"C":                    "",
		"C.UTF-8":              "",
		"POSIX":                "",
		"":                     "",
		"hello world":          "",
		"12345":                "",
	}
	for in, want := range cases {
		if got := NormalizeLocale(in); got != want {
			t.Errorf("NormalizeLocale(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnvLocaleTakesTheFirstSetVariable(t *testing.T) {
	cases := []struct {
		name                          string
		lcAll, lcMessages, lang, want string
	}{
		{"LC_ALL wins", "fr_FR.UTF-8", "de_DE", "es_ES", "fr_FR.UTF-8"},
		{"then LC_MESSAGES", "", "de_DE", "es_ES", "de_DE"},
		{"then LANG", "", "", "es_ES.UTF-8", "es_ES.UTF-8"},
		{"nothing set", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LC_ALL", tc.lcAll)
			t.Setenv("LC_MESSAGES", tc.lcMessages)
			t.Setenv("LANG", tc.lang)
			t.Setenv("LANGUAGE", "ja")
			if got := envLocale(); got != tc.want {
				t.Errorf("envLocale() = %q, want %q", got, tc.want)
			}
		})
	}
}
