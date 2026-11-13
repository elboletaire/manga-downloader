// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

// Linker is a site whose URL can name one release of the series, as a pasted
// chapter or volume link does: the series is still listed whole, and the
// release the link names is the one the CLI downloads and the app selects.
type Linker interface {
	// LinkedID is the Chapter.ID of the release the URL names, "" when it
	// names none (a series URL)
	LinkedID() string
}

// LinkedIndex returns the position in chapters of the release site's URL names,
// -1 when there's none. named reports whether the URL names one at all, so a
// release the listing doesn't hold (another language, say) can be told apart
// from a series URL.
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
