// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !unix && !windows

package cmd

import "os/exec"

// detach is a no-op on platforms the CLI doesn't ship for
func detach(*exec.Cmd) {}
