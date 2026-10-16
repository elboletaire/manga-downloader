// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	repoURL = "https://github.com/elboletaire/manga-downloader"
	// latestReleaseAPI is the newest release, as the GitHub API reports it
	latestReleaseAPI = "https://api.github.com/repos/elboletaire/manga-downloader/releases/latest"
	// latestReleasePage is where the window links an available update
	latestReleasePage = repoURL + "/releases/latest"
)

// updateClient bounds the update check, which runs in the background
var updateClient = &http.Client{Timeout: 10 * time.Second}

// Update is a release newer than the running build
type Update struct {
	// Latest is the newest release's tag, as GitHub names it
	Latest string
	// Installer is the Windows installer of this build's architecture on the
	// release, nil when it has none
	Installer *Asset
}

// releaseURL is the GitHub page of the release a build was made from, "" when
// the build has no release tag
func releaseURL(tag string) string {
	if tag == "" {
		return ""
	}

	return repoURL + "/releases/tag/" + tag
}

// CheckUpdate looks for a release newer than current, the build's release tag,
// and records it for the window. It's meant to run in the background, and
// stops when ctx is cancelled. A failure goes to stderr and leaves no update:
// it's never logged in the window, where it would hide the status bar's line
// on every offline start.
func (c *Controller) CheckUpdate(ctx context.Context, current, apiURL string, client *http.Client) {
	update, err := checkUpdate(ctx, current, apiURL, client)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("couldn't check for updates: %v", err)
		}
		return
	}
	if update == nil {
		return
	}
	c.mu.Lock()
	c.update = update
	c.mu.Unlock()
	c.emit()
}

// Update is the newer release the update check found, nil when there's none or
// no check has found one yet. It's a copy, like the other getters.
func (c *Controller) Update() *Update {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.update == nil {
		return nil
	}
	u := *c.update

	return &u
}

// checkUpdate returns the release newer than current, nil when current is the
// newest one
func checkUpdate(ctx context.Context, current, apiURL string, client *http.Client) (*Update, error) {
	latest, assets, err := fetchLatestRelease(ctx, apiURL, client)
	if err != nil {
		return nil, err
	}
	update, err := newerRelease(current, latest)
	if update != nil {
		update.Installer = pickInstaller(assets, latest, runtime.GOARCH)
	}

	return update, err
}

// fetchLatestRelease reads the tag and the assets of the release the API
// reports as the latest
func fetchLatestRelease(ctx context.Context, apiURL string, client *http.Client) (string, []Asset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "manga-downloader-gui")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}

	var release struct {
		TagName string  `json:"tag_name"`
		Assets  []Asset `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return "", nil, err
	}
	if release.TagName == "" {
		return "", nil, errors.New("the latest release has no tag")
	}

	return release.TagName, release.Assets, nil
}

// newerRelease returns the update to latest when it's newer than current, nil
// otherwise
func newerRelease(current, latest string) (*Update, error) {
	cur, err := parseSemver(current)
	if err != nil {
		return nil, err
	}
	lat, err := parseSemver(latest)
	if err != nil {
		return nil, err
	}
	if !lat.after(cur) {
		return nil, nil
	}

	return &Update{Latest: latest}, nil
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
