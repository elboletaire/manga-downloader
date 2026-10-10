// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package notice

import (
	"testing"
)

func TestHandlerReceivesFormattedMessage(t *testing.T) {
	type entry struct {
		level Level
		msg   string
	}
	var got []entry
	SetHandler(func(level Level, msg string) { got = append(got, entry{level, msg}) })
	t.Cleanup(func() { SetHandler(nil) })

	Warn("page %03d: %s", 4, "kept")
	Warn("no args")
	Info("launching %s...\n", "Chrome")

	want := []entry{
		{LevelWarn, "page 004: kept"},
		{LevelWarn, "no args"},
		{LevelInfo, "launching Chrome..."},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestHandlerNilRestoresDefault(t *testing.T) {
	called := false
	SetHandler(func(Level, string) { called = true })
	SetHandler(nil)

	// the default prints to stdout; the handler must not be consulted
	Warn("default path")
	Info("default path")
	if called {
		t.Error("handler still called after reset to nil")
	}
}

func TestHandlerConcurrentUse(t *testing.T) {
	SetHandler(func(Level, string) {})
	t.Cleanup(func() { SetHandler(nil) })

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				Warn("warn %d", j)
			}
		}()
	}
	for i := 0; i < 100; i++ {
		SetHandler(func(Level, string) {})
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
