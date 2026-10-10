// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

// TestBackgroundsMatchTheStylesheet reads the page's --bg from styles.css (the
// light one from :root, the dark one from the prefers-color-scheme block), so
// the window's startup colours can't drift from the page's.
func TestBackgroundsMatchTheStylesheet(t *testing.T) {
	raw, err := os.ReadFile("frontend/src/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	dark := strings.Index(css, "@media (prefers-color-scheme: dark)")
	if dark < 0 {
		t.Fatal("styles.css has no dark scheme block")
	}
	bg := regexp.MustCompile(`--bg:\s*#([0-9a-fA-F]{6});`)
	light := bg.FindStringSubmatch(css[:dark])
	darkBG := bg.FindStringSubmatch(css[dark:])
	if light == nil || darkBG == nil {
		t.Fatal("styles.css has no --bg in one of the schemes")
	}

	check := func(name string, c options.RGBA, m []string) {
		t.Helper()
		if got := fmt.Sprintf("%02x%02x%02x", c.R, c.G, c.B); got != strings.ToLower(m[1]) {
			t.Errorf("%s background is #%s, styles.css --bg is #%s", name, got, m[1])
		}
	}
	check("light", lightBackground, light)
	check("dark", darkBackground, darkBG)
}
