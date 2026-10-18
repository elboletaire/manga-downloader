// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"io"

	"github.com/elboletaire/manga-downloader/http"
)

// pageCache holds the pages one IdentifySite call has fetched, keyed by the URL
// as requested, so the grabbers that identify a page by its markup share one GET
type pageCache struct {
	entries map[string]cachedPage
}

// cachedPage is a fetched page, or the error its fetch failed with
type cachedPage struct {
	body string
	err  error
}

// newPageCache returns an empty cache for one IdentifySite call
func newPageCache() *pageCache {
	return &pageCache{entries: map[string]cachedPage{}}
}

// identifyPage returns the body of rawURL, fetched without retries since a
// failure means "not this site". Within IdentifySite each URL is fetched once,
// its error included; outside it, every call fetches.
func (g *Grabber) identifyPage(rawURL string) (string, error) {
	if g.pages == nil {
		return fetchPage(rawURL)
	}
	if page, ok := g.pages.entries[rawURL]; ok {
		return page.body, page.err
	}
	body, err := fetchPage(rawURL)
	g.pages.entries[rawURL] = cachedPage{body: body, err: err}
	return body, err
}

// fetchPage reads the whole of rawURL. A body cut short by the connection is
// an error, not a page, so it can't be identified or cached as one.
func fetchPage(rawURL string) (string, error) {
	body, err := http.Get(http.RequestParams{URL: rawURL, DisableRetry: true})
	if err != nil {
		return "", err
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
