// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

// Linker is a site whose pasted volume link names the release the user wants, so
// the front ends can land on it; a chapter link is treated as its series
type Linker interface {
	// LinkedID is the Chapter.ID of the release the URL names, "" when it
	// names none (a series URL)
	LinkedID() string
}

// LinkedIndex returns the position in chapters of the release site's URL names,
// -1 when none; named tells a release the listing lacks from a series URL
func LinkedIndex(site Site, chapters Filterables) (index int, named bool) {
	l, ok := site.(Linker)
	if !ok {
		return -1, false
	}
	id := l.LinkedID()
	if id == "" {
		return -1, false
	}
	for i, c := range chapters {
		if r, ok := c.(interface{ GetID() string }); ok && r.GetID() == id {
			return i, true
		}
	}
	return -1, true
}
