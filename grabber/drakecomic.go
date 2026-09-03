// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"strings"
)

// Drakecomic is a grabber for drakecomic.net (Drake Scans). It used to be a
// themesia wordpress theme on drakecomic.org, behind a cloudflare challenge
// (shared BrowserSiteSelector entry with sushiscan.net in
// plainhtmlbrowser.go). The site has since moved wholesale to drakecomic.net,
// running the same server-rendered Next.js RSC comic platform implemented in
// rsccomic.go (shared with witchtoons.net and kaynscans.com) — plain HTTP
// gets both the chapter list and the reader pages out of the flight payload,
// no browser needed.
//
// drakecomic.org still 301s /manga/{slug}/ to
// https://drakecomic.net/series/comic/{slug}/; Test() applies the same
// rewrite in code so every URL the grabber builds targets the live domain
// directly. Unlike witchtoons.net's old domain, this redirect only covers
// the exact /manga/{slug}/ shape — any other .org path (e.g. a reconstructed
// chapter URL) 301s to the bare drakecomic.net homepage instead, which is
// why chapter URLs must always be built from the rewritten domain rather
// than resolved against the original .org URL.
type Drakecomic struct {
	rscComic
}

func NewDrakecomic(g *Grabber) *Drakecomic {
	return &Drakecomic{rscComic{Grabber: g}}
}

// Test returns true if the URL is a drakecomic.net URL, rewriting old
// drakecomic.org URLs to the new domain and path shape first.
func (d *Drakecomic) Test() (bool, error) {
	if regexp.MustCompile(`drakecomic\.net`).MatchString(d.URL) {
		return true, nil
	}

	if !regexp.MustCompile(`drakecomic\.org`).MatchString(d.URL) {
		return false, nil
	}

	uri, err := url.Parse(d.URL)
	if err != nil {
		return false, err
	}
	uri.Scheme = "https"
	uri.Host = "drakecomic.net"
	if strings.HasPrefix(uri.Path, "/manga/") {
		uri.Path = "/series/comic/" + strings.TrimPrefix(uri.Path, "/manga/")
	}
	d.URL = uri.String()

	return true, nil
}
