// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"os"
	"syscall"
	"testing"
)

func TestExitCode(t *testing.T) {
	if got := exitCode(os.Interrupt); got != 130 {
		t.Errorf("exitCode(SIGINT) = %d, want 130", got)
	}
	if got := exitCode(syscall.SIGTERM); got != 143 {
		t.Errorf("exitCode(SIGTERM) = %d, want 143", got)
	}
}
