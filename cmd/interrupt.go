// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/elboletaire/manga-downloader/browser"
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
		case <-sig:
			browser.Close()
			os.Exit(130)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(sig)
		close(done)
	}
}
