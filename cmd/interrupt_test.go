// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/fatih/color"
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

func TestErrorsMutedOnceInterrupted(t *testing.T) {
	stubExit(t)
	var buf bytes.Buffer
	prev := color.Output
	color.Output = &buf
	t.Cleanup(func() { color.Output = prev })

	red("before")
	interrupted.Store(130)
	red("error: %s", "browser closed")
	yellow("site not recognised")
	printNotice(notice.LevelInfo, "closing the browser")
	printNotice(notice.LevelWarn, "a warning")
	if got := buf.String(); !strings.Contains(got, "before") || strings.Count(got, "\n") != 1 {
		t.Errorf("output = %q, want only what came before the interrupt", got)
	}
}

func TestProgressBarsWriteToTheTerminal(t *testing.T) {
	prev := color.Output
	stop := closeBrowserOnInterrupt()
	// mpb only sizes its bars to the terminal when given the *os.File
	if color.Output != prev {
		t.Errorf("color.Output was replaced: %T", color.Output)
	}
	if _, ok := color.Output.(*os.File); !ok && runtime.GOOS != "windows" {
		t.Errorf("color.Output is a %T, not the terminal's *os.File", color.Output)
	}
	stop()
}

func TestAttentionPrintsOneHighlightedLine(t *testing.T) {
	stubExit(t)
	var buf bytes.Buffer
	prev, prevNoColor := color.Output, color.NoColor
	color.Output, color.NoColor = &buf, false
	t.Cleanup(func() { color.Output, color.NoColor = prev, prevNoColor })

	a := browser.Attention{Active: true, Host: "example.com", Browser: "Brave"}
	printAttention(a)
	printAttention(browser.Attention{Host: "example.com", Browser: "Brave"})

	got := buf.String()
	if strings.Count(got, "\n") != 1 || !strings.Contains(got, a.Message()) {
		t.Fatalf("output = %q, want the message on one line", got)
	}
	if !strings.Contains(got, "\x1b[93;1m") {
		t.Errorf("output = %q, want it bold bright yellow", got)
	}

	interrupted.Store(130)
	printAttention(a)
	if strings.Count(buf.String(), "\n") != 1 {
		t.Errorf("printed after the interrupt: %q", buf.String())
	}
}
