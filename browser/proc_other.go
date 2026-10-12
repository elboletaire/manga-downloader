// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows

package browser

import "os"

// procGuard keeps a browser's processes from outliving the app. Elsewhere
// than on Windows there's nothing to add: chromedp kills the browser itself,
// with SIGKILL on the parent's death on Linux, and Chrome's helper processes
// exit along with it.
type procGuard struct{}

func guardProcess(*os.Process) procGuard { return procGuard{} }

// release is called once the browser process has been stopped
func (procGuard) release() {}
