// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build windows

package cmd

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// launchedFromExplorer reports whether this process is the only one attached to
// its console, which is what Explorer leaves behind when it starts a console
// program by double-click. A shell (cmd, PowerShell) is always attached too.
func launchedFromExplorer() bool {
	// two slots are enough: any count above one is a shell and the call only
	// reports how many there are when the buffer is too small
	var pids [2]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))

	return n == 1
}
