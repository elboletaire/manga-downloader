// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "regexp"

// Vortexscans is a grabber for vortexscans.org, an astroPlatform site (see
// astroisland.go): the series page only renders the ~20 most recent chapters
// as plain <a> links, but the full chapter list is embedded as devalue-encoded
// hydration data on an <astro-island> element's "props" attribute. Reader
// pages are plain server rendered HTML (<img data-reader-page-image> tags), no
// browser/JS needed. Recently released chapters can be paywalled (coins);
// those render zero reader images.
type Vortexscans struct {
	astroPlatform
}

func NewVortexscans(g *Grabber) *Vortexscans {
	return &Vortexscans{astroPlatform{Grabber: g}}
}

// Test returns true if the URL is a vortexscans.org URL
func (v *Vortexscans) Test() (bool, error) {
	re := regexp.MustCompile(`vortexscans\.org`)
	return re.MatchString(v.URL), nil
}
