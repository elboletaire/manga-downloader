// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"bytes"
	"fmt"
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

// stubExit records the calls onInterrupt and exit make instead of exiting
func stubExit(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	prevExit, prevClose := osExit, closeBrowser
	osExit = func(code int) {
		calls = append(calls, fmt.Sprintf("exit %d", code))
	}
	closeBrowser = func() { calls = append(calls, "close") }
	t.Cleanup(func() {
		osExit, closeBrowser = prevExit, prevClose
		interrupted.Store(0)
	})
	return &calls
}

func TestInterruptStopsListeningFirst(t *testing.T) {
	calls := stubExit(t)

	onInterrupt(syscall.SIGTERM, func() { *calls = append(*calls, "stop") })

	want := []string{"stop", "close", "exit 143"}
	if len(*calls) != len(want) {
		t.Fatalf("calls = %v, want %v", *calls, want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Fatalf("calls = %v, want %v", *calls, want)
		}
	}
}

func TestInterruptedRunExitsWithTheSignalCode(t *testing.T) {
	calls := stubExit(t)

	interrupted.Store(130)
	// what an error path does after an aborted load
	exit(1)

	if got := (*calls)[len(*calls)-1]; got != "exit 130" {
		t.Errorf("exited with %q, want 130", got)
	}
}

func TestOutputMutedOnceInterrupted(t *testing.T) {
	stubExit(t)
	var buf bytes.Buffer
	w := muted{&buf}

	w.Write([]byte("before\n"))
	interrupted.Store(130)
	if n, err := w.Write([]byte("error: browser closed\n")); n != 22 || err != nil {
		t.Errorf("muted write = %d, %v", n, err)
	}
	if buf.String() != "before\n" {
		t.Errorf("output = %q, want only what came before the interrupt", buf.String())
	}
}
