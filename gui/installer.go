// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// installerURLPrefix is where a release's assets live; the installer is run
// elevated, so nothing from another place is downloaded
const installerURLPrefix = repoURL + "/releases/download/"

// updateDirPrefix names the temp directories downloadInstaller creates; the
// cleanup helpers refuse any other directory
const updateDirPrefix = "manga-downloader-update-"

// updateTimeout bounds a whole installer download
const updateTimeout = 10 * time.Minute

// newDownloadClient is the client for the installer download: a stalled server
// fails on the headers, and updateTimeout bounds the body through the context.
func newDownloadClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 30 * time.Second

	return &http.Client{Transport: t}
}

// removeUpdateDir deletes a download directory, and refuses anything that
// isn't named like one
func removeUpdateDir(dir string) error {
	if !strings.HasPrefix(filepath.Base(dir), updateDirPrefix) {
		return fmt.Errorf("refusing to remove %q: not an update directory", dir)
	}

	return os.RemoveAll(dir)
}

// sweepUpdateDirs removes the update directories earlier runs left in tmp,
// since the installer runs from its directory and can't delete it itself
func sweepUpdateDirs(tmp string) {
	dirs, _ := filepath.Glob(filepath.Join(tmp, updateDirPrefix+"*"))
	for _, d := range dirs {
		_ = removeUpdateDir(d)
	}
}

// Asset is a file attached to a GitHub release
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// installerName is the Windows installer's asset name for a release tag and
// GOARCH, as the release workflow names it
func installerName(tag, goarch string) string {
	return fmt.Sprintf("manga-downloader-gui-%s-windows-%s-installer.exe", tag, goarch)
}

// pickInstaller returns the installer asset for tag and goarch, nil when the
// release has none (or its URL isn't a release download of this repository)
func pickInstaller(assets []Asset, tag, goarch string) *Asset {
	want := installerName(tag, goarch)
	for _, a := range assets {
		if a.Name == want && a.Size > 0 && strings.HasPrefix(a.URL, installerURLPrefix) {
			asset := a

			return &asset
		}
	}

	return nil
}

// installedAt reports whether exe sits in the directory the installer put the
// app in. The uninstall key's InstallLocation (location, "" when the key or
// value is missing) is authoritative; without it, an uninstall.exe next to the
// exe says the same. A portable copy fails both, so it keeps the release link.
func installedAt(exe, location string, exists func(string) bool) bool {
	dir := filepath.Dir(exe)
	if location != "" {
		return strings.EqualFold(filepath.Clean(location), filepath.Clean(dir))
	}

	return exists(filepath.Join(dir, "uninstall.exe"))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// downloadInstaller saves the asset into a new temp directory and returns its
// path. The size must match the API's, so a cut-off download never runs.
func downloadInstaller(ctx context.Context, client *http.Client, a *Asset) (string, error) {
	if a == nil || !strings.HasPrefix(a.URL, installerURLPrefix) || filepath.Base(a.Name) != a.Name {
		return "", errors.New("the release has no installer for this system")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "manga-downloader-gui")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub answered %s", resp.Status)
	}

	dir, err := os.MkdirTemp("", updateDirPrefix)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, a.Name)
	if err := saveExact(path, resp.Body, a.Size); err != nil {
		_ = removeUpdateDir(dir)

		return "", err
	}

	return path, nil
}

// saveExact writes r to path and fails unless it holds exactly size bytes
func saveExact(path string, r io.Reader, size int64) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("the installer is %d bytes, expected %d", n, size)
	}

	return nil
}
