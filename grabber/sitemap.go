// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bufio"
	"encoding/xml"
	"io"
	"strings"

	"github.com/elboletaire/manga-downloader/http"
)

// sitemapMaxDepth caps how deeply nested sitemap indexes are followed to
// prevent a self-referential index from looping forever.
const sitemapMaxDepth = 2

// sitemapDoc covers both <sitemapindex> (child sitemaps) and <urlset> (page URLs).
type sitemapDoc struct {
	Sitemaps []sitemapLoc `xml:"sitemap"`
	URLs     []sitemapLoc `xml:"url"`
}

type sitemapLoc struct {
	Loc string `xml:"loc"`
}

// robotsSitemaps returns the sitemap URLs advertised in robots.txt.
// A missing or unreadable robots.txt is not an error; the caller falls back
// to well-known paths.
func robotsSitemaps(base string) []string {
	rbody, err := http.Get(http.RequestParams{URL: base + "/robots.txt"})
	if err != nil {
		return nil
	}
	defer rbody.Close()

	var out []string
	sc := bufio.NewScanner(rbody)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		const k = "sitemap:"
		if len(line) <= len(k) || !strings.EqualFold(line[:len(k)], k) {
			continue
		}
		if u := strings.TrimSpace(line[len(k):]); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// collectSitemapURLs returns every page URL reachable from the given sitemap,
// following a sitemap index down to its child sitemaps when needed.
func collectSitemapURLs(url string, depth int) ([]string, error) {
	doc, err := fetchSitemapDoc(url)
	if err != nil {
		return nil, err
	}

	if len(doc.URLs) > 0 {
		locs := make([]string, 0, len(doc.URLs))
		for _, u := range doc.URLs {
			locs = append(locs, u.Loc)
		}
		return locs, nil
	}

	if depth >= sitemapMaxDepth {
		return nil, nil
	}

	var locs []string
	for _, sm := range doc.Sitemaps {
		// A child sitemap that fails is skipped so one bad section doesn't
		// cost every chapter in its siblings.
		child, err := collectSitemapURLs(sm.Loc, depth+1)
		if err != nil {
			continue
		}
		locs = append(locs, child...)
	}
	return locs, nil
}

// fetchSitemapDoc fetches and decodes an XML sitemap document.
func fetchSitemapDoc(url string) (*sitemapDoc, error) {
	rbody, err := http.Get(http.RequestParams{URL: url})
	if err != nil {
		return nil, err
	}
	defer rbody.Close()

	body, err := io.ReadAll(rbody)
	if err != nil {
		return nil, err
	}

	var out sitemapDoc
	if err := xml.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
