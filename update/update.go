// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package update asks for the latest release, through the update Worker and
// GitHub's API when the Worker can't answer, for the CLI and the desktop app.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// BaseURL is the update Worker, which answers GitHub's latest-release JSON
	BaseURL = "https://releases.mangadownloader.cat"
	// GitHubLatestURL is the fallback, the newest release as the GitHub API reports it
	GitHubLatestURL = "https://api.github.com/repos/elboletaire/manga-downloader/releases/latest"
)

// timeout bounds each request; a Worker that hangs falls back to GitHub
var timeout = 5 * time.Second

// maxBody caps what a release answer is read to
const maxBody = 1 << 20

// Source is where the latest release comes from: the Worker, then GitHub
type Source struct {
	// Worker is the update Worker's base URL
	Worker string
	// GitHub is the latest-release API, asked when the Worker fails
	GitHub string
	// HTTP makes the requests, http.DefaultClient when nil
	HTTP *http.Client
}

// Default is the production source
var Default = Source{Worker: BaseURL, GitHub: GitHubLatestURL}

// Check is what the latest-release request tells the Worker about the caller
type Check struct {
	// Client is "cli" or "gui"
	Client string
	// Version is the running build's version, with or without the "v" tag prefix
	Version string
	// Lang is the desktop app's UI language, "" when the caller sends none
	Lang string
	// SystemLang is the system's locale, normalized (see NormalizeLocale), "" when the caller doesn't send one
	SystemLang string
}

// Latest returns the latest release's JSON, as GitHub's API answers it. Any
// failure of the Worker (an error, a timeout, a non-200 answer, a body without
// a tag) asks GitHub instead.
func (s Source) Latest(ctx context.Context, c Check) ([]byte, error) {
	body, err := s.get(ctx, s.Worker+"/v1/latest", c.headers())
	if err == nil && hasTag(body) {
		return body, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return s.get(ctx, s.GitHub, nil)
}

// get returns the body of a 200 answer to url, within timeout. extra headers go
// only with this request, so the GitHub fallback never carries the Worker's.
func (s Source) get(ctx context.Context, address string, extra http.Header) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "manga-downloader")
	for k, v := range extra {
		req.Header[k] = v
	}
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}

	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

// hasTag reports whether body is a release JSON with a tag_name
func hasTag(body []byte) bool {
	var release struct {
		TagName string `json:"tag_name"`
	}

	return json.Unmarshal(body, &release) == nil && release.TagName != ""
}

// headers are the Worker's X-MD-* headers for c: the client, its version
// without the "v" tag prefix, the platform, and the languages when there are any
func (c Check) headers() http.Header {
	h := http.Header{}
	h.Set("X-MD-Client", c.Client)
	h.Set("X-MD-Version", strings.TrimPrefix(c.Version, "v"))
	h.Set("X-MD-OS", runtime.GOOS)
	h.Set("X-MD-Arch", runtime.GOARCH)
	if c.Lang != "" {
		h.Set("X-MD-Lang", c.Lang)
	}
	if c.SystemLang != "" {
		h.Set("X-MD-Syslang", c.SystemLang)
	}

	return h
}

// IsRelease reports whether version is a release's MAJOR.MINOR.PATCH tag, not a
// dev build's name such as "develop"
func IsRelease(version string) bool {
	_, err := parseSemver(version)

	return err == nil
}

// Newer reports whether latest is a later version than current. It errors
// when either isn't a MAJOR.MINOR.PATCH version.
func Newer(current, latest string) (bool, error) {
	cur, err := parseSemver(current)
	if err != nil {
		return false, err
	}
	lat, err := parseSemver(latest)
	if err != nil {
		return false, err
	}

	return lat.after(cur), nil
}

// semver is a MAJOR.MINOR.PATCH version
type semver [3]int

// parseSemver reads "v1.2.3" (the v is optional). A pre-release or build suffix
// is dropped, so "v1.2.3-rc.1" counts as 1.2.3 and isn't newer than it. So does
// a git describe name past a tag, "v1.2.3-3-gabc": it's 1.2.3 plus commits.
func parseSemver(tag string) (semver, error) {
	s := strings.TrimPrefix(tag, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("%q isn't MAJOR.MINOR.PATCH", tag)
	}

	var v semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, fmt.Errorf("%q isn't MAJOR.MINOR.PATCH", tag)
		}
		v[i] = n
	}

	return v, nil
}

// after reports whether v is a later version than w
func (v semver) after(w semver) bool {
	for i := range v {
		if v[i] != w[i] {
			return v[i] > w[i]
		}
	}

	return false
}
