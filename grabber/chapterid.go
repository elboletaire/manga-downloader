// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "net/url"

// chapterURLID returns the ID of a chapter URL: its path and sorted query, so the
// ID survives a host change or reordered links. "" when rawURL doesn't parse.
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
