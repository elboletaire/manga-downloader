// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "regexp"

// Witchtoons is a grabber for witchtoons.net (the witchscans.com rebrand,
// which 301s to this domain but drops the path: the old themesia wordpress
// theme is gone and series now live under /series/comic/{slug}). It runs the
// server-rendered Next.js RSC comic platform implemented in rsccomic.go
// (shared with kaynscans.com), so plain HTTP gets both the chapter list and
// the reader pages out of the flight payload.
type Witchtoons struct {
	rscComic
}

func NewWitchtoons(g *Grabber) *Witchtoons {
	return &Witchtoons{rscComic{Grabber: g}}
}

// Test returns true if the URL is a witchtoons.net URL
func (w *Witchtoons) Test() (bool, error) {
	re := regexp.MustCompile(`witchtoons\.net`)
	return re.MatchString(w.URL), nil
}
