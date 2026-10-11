// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "regexp"

// Hivetoons is a grabber for hivetoons.org (HiveToons, VoidScans), a
// tanstackComic site (see tanstackcomic.go). It used to be a PlainHTML
// selector entry matching the server-rendered a.p-3 chapter anchors, but the
// site only renders the ~20 most recent chapters that way (the rest loads
// through a client-side "Load more"), so the selector silently truncated every
// longer series — e.g. 20 of Eleceed's 422 chapters.
type Hivetoons struct {
	tanstackComic
}

func NewHivetoons(g *Grabber) *Hivetoons {
	return &Hivetoons{tanstackComic{Grabber: g}}
}

// Test returns true if the URL is a hivetoons.org URL
func (h *Hivetoons) Test() (bool, error) {
	re := regexp.MustCompile(`hivetoons\.org`)
	return re.MatchString(h.URL), nil
}
