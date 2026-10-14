// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import "net/url"

// chapterURLID returns the path of a chapter URL, the chapter ID of the
// grabbers whose chapters are only addressed by their URL. The host, query
// and fragment are dropped, so the same release keeps its ID when the site
// moves to another host or adds tracking parameters to its links. Returns ""
// when rawURL is empty or doesn't parse.
func chapterURLID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	return u.Path
}
