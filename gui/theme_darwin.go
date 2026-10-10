// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// themeTimeout bounds the settings lookup, which runs before the window opens
const themeTimeout = 500 * time.Millisecond

// systemPrefersDark reads the global appearance. "Dark" is the only value
// macOS writes; no value means light. A lookup that fails or times out counts
// as light.
func systemPrefersDark() bool {
	ctx, cancel := context.WithTimeout(context.Background(), themeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleInterfaceStyle")
	// a killed defaults whose children still hold stdout would otherwise keep
	// Output waiting past the deadline
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()

	return err == nil && strings.TrimSpace(string(out)) == "Dark"
}
