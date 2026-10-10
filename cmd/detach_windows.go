// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
)

// detachedProcess is DETACHED_PROCESS from the Win32 API: the child gets no
// console, and closing the one that started it leaves the child running
const detachedProcess = 0x00000008

// detach starts the GUI without a console of its own, so it outlives the
// console it was launched from
func detach(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
