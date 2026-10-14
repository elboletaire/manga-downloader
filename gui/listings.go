// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// listingCacheSize bounds the listings a session keeps for filter changes
const listingCacheSize = 8

// cachedListing is the unfiltered listing of a URL in one language. A
// ScanlatorLister's listing depends on the group too, as the site filters it;
// any other listing holds every group's releases, and the group is applied to
// it (core.Listing.Series), so its entry serves every group.
type cachedListing struct {
	url, language string
	// scanlator is part of the key only for a ScanlatorLister (lister)
	scanlator string
	lister    bool
	listing   *core.Listing
}

// matches reports whether the entry is the listing of url for the settings
func (e cachedListing) matches(url string, s *grabber.Settings) bool {
	return e.url == url && e.language == s.Language && (!e.lister || e.scanlator == s.Scanlator)
}

// listingCache keeps the listings of the session's loads, most recently used
// first, and forgets the least recently used past listingCacheSize
type listingCache []cachedListing

// get returns the cached listing of url for the settings, nil when there's
// none, and makes it the most recently used
func (l *listingCache) get(url string, s *grabber.Settings) *core.Listing {
	for i, e := range *l {
		if !e.matches(url, s) {
			continue
		}
		copy((*l)[1:i+1], (*l)[:i])
		(*l)[0] = e

		return e.listing
	}

	return nil
}

// put caches the listing of url for the settings, in place of any entry it
// replaces
func (l *listingCache) put(url string, s *grabber.Settings, listing *core.Listing) {
	_, lister := listing.Site.(grabber.ScanlatorLister)
	e := cachedListing{url: url, language: s.Language, lister: lister, listing: listing}
	if lister {
		e.scanlator = s.Scanlator
	}

	l.drop(url, s)
	*l = append([]cachedListing{e}, *l...)
	if len(*l) > listingCacheSize {
		*l = (*l)[:listingCacheSize]
	}
}

// drop forgets the cached listing of url for the settings, if there's one
func (l *listingCache) drop(url string, s *grabber.Settings) {
	kept := (*l)[:0]
	for _, e := range *l {
		if !e.matches(url, s) {
			kept = append(kept, e)
		}
	}
	*l = kept
}
