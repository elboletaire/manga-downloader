// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/url"
	"regexp"
	"strings"
)

// Kaynscan is a grabber for kaynscans.com (Kayn Scans, formerly kaynscan.org,
// which was an Astro/astro-island site — see astroisland.go — until the move,
// around Aug 2026). The new domain runs the same server-rendered Next.js RSC
// comic platform as witchtoons.net, implemented in rsccomic.go: series live
// under /series/comic/{slug} and both the (paginated) chapter list and the
// reader page images ship inside the RSC flight payload, all over plain HTTP.
// kaynscan.org still 301s here remapping /series/{slug} to
// /series/comic/{slug}; Test() applies the same rewrite in code so every URL
// the grabber builds targets the live domain directly.
type Kaynscan struct {
	rscComic
}

func NewKaynscan(g *Grabber) *Kaynscan {
	return &Kaynscan{rscComic{Grabber: g}}
}

// Test returns true if the URL is a kaynscans.com URL, rewriting old
// kaynscan.org URLs to the new domain and path shape first
func (k *Kaynscan) Test() (bool, error) {
	if regexp.MustCompile(`kaynscans\.com`).MatchString(k.URL) {
		return true, nil
	}

	if !regexp.MustCompile(`kaynscan\.org`).MatchString(k.URL) {
		return false, nil
	}

	uri, err := url.Parse(k.URL)
	if err != nil {
		return false, err
	}
	uri.Scheme = "https"
	uri.Host = "kaynscans.com"
	if strings.HasPrefix(uri.Path, "/series/") && !strings.HasPrefix(uri.Path, "/series/comic/") {
		uri.Path = "/series/comic/" + strings.TrimPrefix(uri.Path, "/series/")
	}
	k.URL = uri.String()

	return true, nil
}
