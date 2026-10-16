// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/grabber"
)

// SplitLocked separates the chapters the listing marks as locked from the rest,
// keeping their order. Both the CLI and the app leave the locked ones out
// before downloading, as a logged-out reader can't open them.
func SplitLocked(chapters grabber.Filterables) (open, locked grabber.Filterables) {
	for _, c := range chapters {
		if grabber.LockedOf(c) {
			locked = append(locked, c)
		} else {
			open = append(open, c)
		}
	}
	return open, locked
}

// LockedMessage says which locked chapters are being left out, with runs of
// consecutive whole numbers compacted: "skipping 4 locked chapters: 134-136, 140"
func LockedMessage(locked grabber.Filterables) string {
	var parts []string
	for i := 0; i < len(locked); {
		j := i
		for j+1 < len(locked) && locked[j+1].GetNumber() == locked[j].GetNumber()+1 {
			j++
		}
		first := strconv.FormatFloat(locked[i].GetNumber(), 'f', -1, 64)
		if j == i {
			parts = append(parts, first)
		} else {
			parts = append(parts, first+"-"+strconv.FormatFloat(locked[j].GetNumber(), 'f', -1, 64))
		}
		i = j + 1
	}
	noun := "chapters"
	if len(locked) == 1 {
		noun = "chapter"
	}
	return fmt.Sprintf("skipping %d locked %s: %s", len(locked), noun, strings.Join(parts, ", "))
}
