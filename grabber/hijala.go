// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "regexp"

// Hijala is a grabber for en-hijala.com, a tanstackComic site (see
// tanstackcomic.go). The series page embeds the full chapter list in its
// seroval data, so no API or series id is needed.
type Hijala struct {
	tanstackComic
}

func NewHijala(g *Grabber) *Hijala {
	return &Hijala{tanstackComic{Grabber: g}}
}

// hijalaHostRe matches en-hijala.com case-insensitively.
var hijalaHostRe = regexp.MustCompile(`(?i)en-hijala\.com`)

// Test returns true if the URL is an en-hijala.com URL
func (h *Hijala) Test() (bool, error) {
	return hijalaHostRe.MatchString(h.URL), nil
}
