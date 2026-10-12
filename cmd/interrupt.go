// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"io"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/fatih/color"
)

// interrupted holds the exit code once the CLI was interrupted, 0 until then
var interrupted atomic.Int32

// swapped in tests
var (
	osExit       = os.Exit
	closeBrowser = browser.Close
)

// closeBrowserOnInterrupt closes the shared browser when the CLI is
// interrupted (Ctrl+C, or the console closing), then exits. Without it, an
// interrupt kills the process before its deferred browser.Close runs, which
// leaves the browser running on Windows and macOS (Linux kills it with the
// parent). Output is muted from the interrupt on, so the loads it aborts
// don't print errors. The returned function stops listening.
func closeBrowserOnInterrupt() (stop func()) {
	out, errOut := color.Output, color.Error
	color.Output, color.Error = muted{out}, muted{errOut}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case s := <-sig:
			onInterrupt(s, func() { signal.Stop(sig) })
		case <-done:
		}
	}()
	return func() {
		signal.Stop(sig)
		close(done)
		if interrupted.Load() != 0 {
			exit(0)
		}
		color.Output, color.Error = out, errOut
	}
}

// onInterrupt handles the first interrupt. stopSignals goes first, so a
// second Ctrl+C kills the process at once if closing the browser hangs.
func onInterrupt(s os.Signal, stopSignals func()) {
	stopSignals()
	interrupted.Store(int32(exitCode(s)))
	closeBrowser()
	osExit(exitCode(s))
}

// exitCode is the shell's code for a process ended by s: 128 + the signal
func exitCode(s os.Signal) int {
	if s == syscall.SIGTERM {
		return 143
	}
	return 130
}

// muted drops what's written once the CLI was interrupted
type muted struct{ w io.Writer }

func (m muted) Write(p []byte) (int, error) {
	if interrupted.Load() != 0 {
		return len(p), nil
	}
	return m.w.Write(p)
}
