// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// a saved options file that doesn't parse must be reported in the window's
// log: the handler is the controller's own, so nothing here masks the order
// main() relies on
func TestStartupWarningsReachControllerLog(t *testing.T) {
	// XDG_CONFIG_HOME only steers os.UserConfigDir on Linux; elsewhere this
	// would read the developer's real gui.json
	if runtime.GOOS != "linux" {
		t.Skip("config dir override is Linux-only")
	}
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	dir := filepath.Join(config, "manga-downloader")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gui.json"), []byte(`{"format":`), 0600); err != nil {
		t.Fatal(err)
	}

	ctrl, _ := newSession(func() {})

	for _, line := range ctrl.Log() {
		if line.Level == LogWarn && strings.Contains(line.Text, "ignoring the saved options") {
			return
		}
	}
	t.Errorf("the startup warning is missing from the controller's log: %+v", ctrl.Log())
}
