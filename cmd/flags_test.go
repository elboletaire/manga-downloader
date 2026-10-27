// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import "testing"

// --skip-pages takes a negative position as its value, with or without "=",
// since pflag reads a long flag's argument verbatim (see the flag's comment)
func TestSkipPagesTakesNegativePositions(t *testing.T) {
	flags := rootCmd.Flags()
	t.Cleanup(func() {
		settings.SkipPages = ""
		flags.Lookup("skip-pages").Changed = false
	})

	for _, args := range [][]string{{"--skip-pages", "-1"}, {"--skip-pages=-1"}} {
		settings.SkipPages = ""
		if err := flags.Parse(args); err != nil {
			t.Fatalf("parsing %v: %v", args, err)
		}
		if settings.SkipPages != "-1" {
			t.Errorf("parsing %v: SkipPages = %q, want -1", args, settings.SkipPages)
		}
	}
}

// --skip-existing is off unless the flag is given
func TestSkipExistingFlag(t *testing.T) {
	flags := rootCmd.Flags()
	t.Cleanup(func() {
		settings.SkipExisting = false
		flags.Lookup("skip-existing").Changed = false
	})

	if settings.SkipExisting {
		t.Fatal("SkipExisting is on before the flag")
	}
	if err := flags.Parse([]string{"--skip-existing"}); err != nil {
		t.Fatal(err)
	}
	if !settings.SkipExisting {
		t.Error("--skip-existing didn't set SkipExisting")
	}
}
