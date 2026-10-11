// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"strings"
)

// Elftoon is a grabber for elftoon.net (Elf Toon, formerly elftoon.com, which
// was a mangastream/themesia theme — see plainhtml.go). The site has moved
// wholesale to elftoon.net, running the same server-rendered Next.js RSC comic
// platform as witchtoons.net, kaynscans.com and drakecomic.net, implemented in
// rsccomic.go: plain HTTP gets both the chapter list and the reader pages out
// of the flight payload, no browser needed.
//
// elftoon.com still 301s /manga/{slug}/ to elftoon.net, which then redirects
// on to /series/comic/{slug}; Test() applies the same rewrite in code so every
// URL the grabber builds targets the live domain directly.
type Elftoon struct {
	rscComic
}

func NewElftoon(g *Grabber) *Elftoon {
	return &Elftoon{rscComic{Grabber: g}}
}

// Test returns true if the URL is an elftoon.net URL, rewriting old
// elftoon.com URLs to the new domain and path shape first.
func (e *Elftoon) Test() (bool, error) {
	if regexp.MustCompile(`elftoon\.net`).MatchString(e.URL) {
		return true, nil
	}

	if !regexp.MustCompile(`elftoon\.com`).MatchString(e.URL) {
		return false, nil
	}

	uri, err := url.Parse(e.URL)
	if err != nil {
		return false, err
	}
	uri.Scheme = "https"
	uri.Host = "elftoon.net"
	if strings.HasPrefix(uri.Path, "/manga/") {
		uri.Path = "/series/comic/" + strings.TrimPrefix(uri.Path, "/manga/")
	}
	e.URL = uri.String()

	return true, nil
}
