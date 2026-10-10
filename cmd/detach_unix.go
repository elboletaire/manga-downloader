// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build unix

package cmd

import (
	"os/exec"
	"syscall"
)

// detach gives the GUI its own session, so closing the terminal that ran
// `manga-downloader ui` (which sends SIGHUP to the session) doesn't kill it
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
