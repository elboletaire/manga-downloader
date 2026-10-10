// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package notice carries the messages library code (grabbers, packer, browser)
// raises while it works. The CLI prints them in colour, as it always has; a
// GUI redirects them with SetHandler so they reach its own log instead of the
// terminal.
package notice

import (
	"fmt"
	"strings"
	"sync"

	"github.com/fatih/color"
)

// Level tells informational messages apart from warnings
type Level int

const (
	// LevelInfo is a heads-up about what's going on (printed in blue)
	LevelInfo Level = iota
	// LevelWarn is something that went wrong without stopping the download
	// (printed in yellow)
	LevelWarn
)

var (
	mu      sync.RWMutex
	handler func(level Level, msg string)
)

// SetHandler redirects Info and Warn to fn, which receives the formatted
// message without a trailing newline. Passing nil restores the default, which
// prints the message in colour on stdout. Safe for concurrent use.
func SetHandler(fn func(level Level, msg string)) {
	mu.Lock()
	defer mu.Unlock()
	handler = fn
}

// Info reports a heads-up, formatted like fmt.Printf (or printed verbatim
// when there are no arguments, the same as color.Blue).
func Info(format string, a ...any) {
	emit(LevelInfo, color.Blue, format, a...)
}

// Warn reports a warning, formatted like fmt.Printf (or printed verbatim when
// there are no arguments, the same as color.Yellow).
func Warn(format string, a ...any) {
	emit(LevelWarn, color.Yellow, format, a...)
}

func emit(level Level, print func(string, ...any), format string, a ...any) {
	mu.RLock()
	h := handler
	mu.RUnlock()

	if h == nil {
		// the default path is the color function itself, so the terminal
		// output is byte-for-byte what the library code printed before
		print(format, a...)
		return
	}

	msg := format
	if len(a) > 0 {
		msg = fmt.Sprintf(format, a...)
	}
	h(level, strings.TrimSuffix(msg, "\n"))
}
