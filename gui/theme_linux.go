// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// themeTimeout bounds the settings lookups, which run before the window opens
const themeTimeout = 500 * time.Millisecond

// systemPrefersDark asks gsettings, which GNOME-based desktops keep the colour
// scheme and the GTK theme in. Other desktops have no such keys, and they and
// any lookup that fails or times out count as light. The lookups share one
// deadline, so opening the window waits at most themeTimeout for them.
func systemPrefersDark() bool {
	ctx, cancel := context.WithTimeout(context.Background(), themeTimeout)
	defer cancel()
	if gsettingsValue(ctx, "color-scheme") == "'prefer-dark'" {
		return true
	}

	return strings.Contains(strings.ToLower(gsettingsValue(ctx, "gtk-theme")), "dark")
}

// gsettingsValue is the value of a key of the desktop interface schema, "" when
// gsettings isn't there, the key doesn't exist or ctx is done
func gsettingsValue(ctx context.Context, key string) string {
	cmd := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.interface", key)
	// a killed gsettings whose children still hold stdout would otherwise
	// keep Output waiting past the deadline
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}
