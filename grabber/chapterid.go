// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "net/url"

// chapterURLID returns the ID of a chapter URL for the grabbers whose chapters
// are only addressed by their URL: its path, plus its query with the
// parameters sorted, so the same chapter keeps its ID when the site moves to
// another host or reorders its links. The host and fragment are dropped.
// Returns "" when rawURL is empty or doesn't parse. Relative hrefs work too.
func chapterURLID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if u.RawQuery == "" {
		return u.Path
	}

	return u.Path + "?" + u.Query().Encode()
}
