// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// installerURLPrefix is where a release's assets live; what is downloaded gets
// run (the Windows installer elevated), so nothing from another place is fetched
const installerURLPrefix = repoURL + "/releases/download/"

// updateDirPrefix names the temp directories downloadInstaller creates; the
// cleanup helpers refuse any other directory
const updateDirPrefix = "manga-downloader-update-"

// updateTimeout bounds a whole installer download
const updateTimeout = 10 * time.Minute

// errNotRestarted marks an installer that finished but couldn't start the new
// version again
var errNotRestarted = errors.New("couldn't restart")

// maxSumsSize bounds the checksums file, which is read into memory
const maxSumsSize = 1 << 20

// maxExtractSize bounds what a tarball may unpack to
const maxExtractSize = 1 << 30

// newDownloadClient is the client for the update download: a stalled server
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

// sweepUpdateDirs removes the update directories earlier runs left in tmp.
// An update quits the app once its installer starts, so the next start
// cleans up after it, on every system the same way.
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

// windowsInstallerName is the Windows installer's asset name for a release
// tag and GOARCH, as the release workflow names it
func windowsInstallerName(tag, goarch string) string {
	return fmt.Sprintf("manga-downloader-gui-%s-windows-%s-installer.exe", tag, goarch)
}

// linuxTarballName is the Linux tarball's asset name for a release tag and GOARCH
func linuxTarballName(tag, goarch string) string {
	return fmt.Sprintf("manga-downloader-gui-%s-linux-%s.tar.gz", tag, goarch)
}

// darwinZipName is the macOS zip's asset name for a release tag; one universal
// build serves every GOARCH
func darwinZipName(tag string) string {
	return fmt.Sprintf("manga-downloader-gui-%s-darwin-universal.zip", tag)
}

// sumsName is the checksums asset the release workflow publishes for the GUI
func sumsName(tag string) string {
	return fmt.Sprintf("manga-downloader-gui-%s-SHA256SUMS.txt", tag)
}

// pickAsset returns the asset called name, nil when the release has none (or
// its URL isn't a release download of this repository)
func pickAsset(assets []Asset, name string) *Asset {
	if name == "" {
		return nil
	}
	for _, a := range assets {
		if a.Name == name && a.Size > 0 && strings.HasPrefix(a.URL, installerURLPrefix) {
			asset := a

			return &asset
		}
	}

	return nil
}

// parseSums reads sha256sum output ("<hex>  <name>", or "<hex> *<name>")
// into name -> lowercase hex digest
func parseSums(data []byte) map[string]string {
	sums := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		sum, name, ok := strings.Cut(line, " ")
		if !ok || len(sum) != sha256.Size*2 {
			continue
		}
		if _, err := hex.DecodeString(sum); err != nil {
			continue
		}
		name = strings.TrimPrefix(strings.TrimPrefix(name, " "), "*")
		if name != "" {
			sums[name] = strings.ToLower(sum)
		}
	}

	return sums
}

// verifyChecksum hashes the file at path and compares it with name's entry in
// the sums; a missing entry is as much a failure as a mismatch
func verifyChecksum(path, name string, sums map[string]string) error {
	want, ok := sums[name]
	if !ok {
		return fmt.Errorf("the release's checksums don't list %s", name)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("the checksum of %s doesn't match the release's (got %s, want %s)", name, got, want)
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

// installScope is where the Windows installer put the app: per user (HKCU,
// %LOCALAPPDATA%) or per machine (HKLM, Program Files)
type installScope int

const (
	scopeUser installScope = iota + 1
	scopeMachine
)

// detectScope finds the scope whose uninstall key points at exe's folder, the
// user's first. Failing that, an uninstall.exe beside exe counts as per machine,
// whose elevated run works for both.
func detectScope(exe string, read func(installScope) string, exists func(string) bool) (installScope, bool) {
	for _, sc := range []installScope{scopeUser, scopeMachine} {
		loc := read(sc)
		if loc == "" {
			continue
		}
		if installedAt(exe, loc, exists) {
			return sc, true
		}
	}
	if installedAt(exe, "", exists) {
		return scopeMachine, true
	}

	return 0, false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// get opens a GET on url, which must be a release download
func get(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	if !strings.HasPrefix(url, installerURLPrefix) {
		return nil, errors.New("the release has no update for this system")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "manga-downloader-gui")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()

		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}

	return resp, nil
}

// fetchSums downloads the release's checksums file
func fetchSums(ctx context.Context, client *http.Client, a *Asset) (map[string]string, error) {
	if a == nil || a.Size <= 0 || a.Size > maxSumsSize {
		return nil, errors.New("the release has no checksums for this system")
	}
	resp, err := get(ctx, client, a.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, a.Size+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != a.Size {
		return nil, fmt.Errorf("the checksums are %d bytes, expected %d", len(data), a.Size)
	}

	return parseSums(data), nil
}

// downloadInstaller saves the asset into a new temp directory and returns its
// path. The size must match the API's and the SHA-256 the release's checksums
// (sums), so a cut-off or swapped download never runs.
func downloadInstaller(ctx context.Context, client *http.Client, a, sums *Asset) (string, error) {
	if a == nil || filepath.Base(a.Name) != a.Name || a.Name == "." {
		return "", errors.New("the release has no update for this system")
	}
	list, err := fetchSums(ctx, client, sums)
	if err != nil {
		return "", err
	}
	resp, err := get(ctx, client, a.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	dir, err := os.MkdirTemp("", updateDirPrefix)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, a.Name)
	if err := saveExact(path, resp.Body, a.Size); err != nil {
		_ = removeUpdateDir(dir)

		return "", err
	}
	if err := verifyChecksum(path, a.Name, list); err != nil {
		_ = removeUpdateDir(dir)

		return "", err
	}

	return path, nil
}

// extractTarGz unpacks a .tar.gz into dest (an existing directory). Only
// directories and regular files are accepted, and every name must stay inside
// dest, so a crafted archive can't write elsewhere.
func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !filepath.IsLocal(h.Name) {
			return fmt.Errorf("the archive has an entry outside its folder: %q", h.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(h.Name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			total += h.Size
			if h.Size < 0 || total > maxExtractSize {
				return errors.New("the archive is too large")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			if err := writeEntry(target, tr, h.Size, os.FileMode(h.Mode).Perm()&0o755|0o600); err != nil {
				return err
			}
		default:
			return fmt.Errorf("the archive has an entry that isn't a file or folder: %q", h.Name)
		}
	}
}

func writeEntry(path string, r io.Reader, size int64, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(r, size))
	if cerr := f.Close(); err == nil {
		err = cerr
	}

	return err
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
		return fmt.Errorf("the download is %d bytes, expected %d", n, size)
	}

	return nil
}

// lastLine is the last non-empty line of a command's output
func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}

	return "no output"
}
