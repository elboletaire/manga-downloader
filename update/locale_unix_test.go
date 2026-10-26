// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows

package update

import "testing"

func TestSystemLocaleReadsTheEnvironment(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "es_ES.UTF-8")
	if got := SystemLocale(); got != "es-ES" {
		t.Errorf("SystemLocale() = %q, want es-ES", got)
	}
	t.Setenv("LANG", "C.UTF-8")
	if got := SystemLocale(); got != "" {
		t.Errorf("SystemLocale() with C.UTF-8 = %q, want \"\"", got)
	}
}
