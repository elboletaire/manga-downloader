// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "regexp"

// Azorafly is a grabber for azorafly.com (Arabic), a tanstackComic site (see
// tanstackcomic.go). Like vortexscans, the series page only server-renders
// the newest chapters, while the full list is embedded in its seroval data.
type Azorafly struct {
	tanstackComic
}

func NewAzorafly(g *Grabber) *Azorafly {
	return &Azorafly{tanstackComic{Grabber: g}}
}

// Test returns true if the URL is an azorafly.com URL
func (a *Azorafly) Test() (bool, error) {
	re := regexp.MustCompile(`azorafly\.com`)
	return re.MatchString(a.URL), nil
}
