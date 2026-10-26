// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"runtime"
	"strings"

	"github.com/elboletaire/manga-downloader/update"
)

const (
	repoURL = "https://github.com/elboletaire/manga-downloader"
	// latestReleasePage is where the window links an available update
	latestReleasePage = repoURL + "/releases/latest"
)

// Update is a release newer than the running build
type Update struct {
	// Latest is the newest release's tag, as GitHub names it
	Latest string
	// Installer is the package that updates this build in place (the Windows
	// installer, the Linux tarball, the macOS zip), nil when the release has
	// none or has no checksums for it
	Installer *Asset
	// Sums is the release's checksums asset, which Installer is verified against
	Sums *Asset
}

// systemLocaleLen caps the system's locale the window reports; a BCP 47 tag is far shorter
const systemLocaleLen = 35

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
func (c *Controller) CheckUpdate(ctx context.Context, current string, src update.Source) {
	c.mu.Lock()
	check := updateCheck(current, c.prefs, c.systemLocale)
	c.mu.Unlock()
	found, err := checkUpdate(ctx, check, src)
	if err != nil && ctx.Err() == nil {
		log.Printf("couldn't check for updates: %v", err)
	}
	c.mu.Lock()
	c.updateChecking = false
	// turned off while it ran: the answer is dropped
	recorded := c.prefs.CheckUpdates && found != nil
	if recorded {
		c.update = found
	}
	c.mu.Unlock()
	if recorded {
		c.emit()
	}
}

// SetSystemLocale stores the system's raw locale, as the window reports it, for
// the update check. It's trimmed and capped, and doesn't notify: it isn't in State.
func (c *Controller) SetSystemLocale(locale string) {
	locale = strings.TrimSpace(locale)
	if r := []rune(locale); len(r) > systemLocaleLen {
		locale = string(r[:systemLocaleLen])
	}
	c.mu.Lock()
	c.systemLocale = locale
	c.mu.Unlock()
}

// updateCheck is what the update check tells the Worker: the app's language, or
// "system" with the system's locale when the app follows the system
func updateCheck(current string, prefs PrefsDTO, systemLocale string) update.Check {
	check := update.Check{Client: "gui", Version: current, Lang: prefs.UILanguage}
	if prefs.UILanguage == "" {
		check.Lang = "system"
		check.SystemLang = systemLocale
	}

	return check
}

// ClearUpdate forgets the update the check found, so the window stops offering it,
// and lets the next turn-on check again even while an old check is still running
func (c *Controller) ClearUpdate() {
	c.mu.Lock()
	c.update = nil
	c.updateChecking = false
	c.mu.Unlock()
	c.emit()
}

// claimUpdateCheck reports whether no update check is running, and marks one as
// running. CheckUpdate clears the mark when it ends.
func (c *Controller) claimUpdateCheck() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.updateChecking {
		return false
	}
	c.updateChecking = true

	return true
}

// shouldCheckUpdates is whether the update check runs: never for a dev build
// (no tag), nor when the user turned it off
func shouldCheckUpdates(tag string, prefs PrefsDTO) bool {
	return tag != "" && prefs.CheckUpdates
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

// checkUpdate returns the release newer than the check's version, nil when that
// is the newest one
func checkUpdate(ctx context.Context, check update.Check, src update.Source) (*Update, error) {
	latest, assets, err := fetchLatestRelease(ctx, check, src)
	if err != nil {
		return nil, err
	}
	found, err := newerRelease(check.Version, latest)
	if found != nil {
		found.Installer = pickAsset(assets, packageName(latest, runtime.GOARCH))
		found.Sums = pickAsset(assets, sumsName(latest))
		if found.Sums == nil {
			found.Installer = nil
		}
	}

	return found, err
}

// fetchLatestRelease reads the tag and the assets of the release the update
// source reports as the latest
func fetchLatestRelease(ctx context.Context, check update.Check, src update.Source) (string, []Asset, error) {
	body, err := src.Latest(ctx, check)
	if err != nil {
		return "", nil, err
	}

	var release struct {
		TagName string  `json:"tag_name"`
		Assets  []Asset `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
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
	newer, err := update.Newer(current, latest)
	if err != nil || !newer {
		return nil, err
	}

	return &Update{Latest: latest}, nil
}
