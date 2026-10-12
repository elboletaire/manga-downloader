// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/notice"
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
// parent). The returned function stops listening.
func closeBrowserOnInterrupt() (stop func()) {
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

// red and yellow print like color.Red and color.Yellow, but nothing once the
// CLI was interrupted: the loads the interrupt aborts would print errors. The
// progress bars aren't muted, as mpb needs the terminal itself to size them.
func red(format string, a ...any) {
	if interrupted.Load() == 0 {
		color.Red(format, a...)
	}
}

func yellow(format string, a ...any) {
	if interrupted.Load() == 0 {
		color.Yellow(format, a...)
	}
}

// printNotice is the CLI's notice handler: the default colours, muted once
// interrupted like red and yellow
func printNotice(level notice.Level, msg string) {
	if level == notice.LevelWarn {
		yellow(msg)
	} else if interrupted.Load() == 0 {
		color.Blue(msg)
	}
}
