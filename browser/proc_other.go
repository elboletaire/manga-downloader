// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows

package browser

import "os"

// procGuard keeps a browser's processes from outliving the app; only Windows
// has one. On Linux chromedp has the browser SIGKILLed when the app dies
// (Pdeathsig), and Chrome's helpers exit along with it. macOS has no such
// thing: Close and the CLI's interrupt handler cover normal exits, but an app
// that crashes or is killed can leave the browser running.
type procGuard struct{}

func guardProcess(*os.Process) procGuard { return procGuard{} }

// release is called once the browser process has been stopped
func (procGuard) release() {}
