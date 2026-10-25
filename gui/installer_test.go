// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

func asset(name string, size int64) Asset {
	return Asset{Name: name, URL: installerURLPrefix + "v2.0.0/" + name, Size: size}
}

func TestPickAsset(t *testing.T) {
	assets := []Asset{
		asset("manga-downloader-gui-v2.0.0-windows-amd64.zip", 5),
		asset(windowsInstallerName("v2.0.0", "arm64"), 7),
		asset(windowsInstallerName("v2.0.0", "amd64"), 9),
		asset(linuxTarballName("v2.0.0", "amd64"), 11),
		asset(darwinZipName("v2.0.0"), 13),
	}
	if got := pickAsset(assets, windowsInstallerName("v2.0.0", "amd64")); got == nil || got.Size != 9 {
		t.Errorf("amd64 = %+v", got)
	}
	if got := pickAsset(assets, linuxTarballName("v2.0.0", "amd64")); got == nil || got.Size != 11 {
		t.Errorf("tarball = %+v", got)
	}
	if got := pickAsset(assets, darwinZipName("v2.0.0")); got == nil || got.Size != 13 {
		t.Errorf("macOS zip = %+v", got)
	}
	if got := pickAsset(assets, linuxTarballName("v2.0.0", "arm64")); got != nil {
		t.Errorf("missing arch = %+v, want none", got)
	}
	if got := pickAsset(assets, windowsInstallerName("v2.0.1", "amd64")); got != nil {
		t.Errorf("another tag = %+v, want none", got)
	}
	if got := pickAsset(assets, ""); got != nil {
		t.Errorf("no name = %+v, want none", got)
	}
	foreign := []Asset{{Name: windowsInstallerName("v2.0.0", "amd64"), URL: "https://evil.example/x.exe", Size: 1}}
	if got := pickAsset(foreign, windowsInstallerName("v2.0.0", "amd64")); got != nil {
		t.Errorf("foreign URL = %+v, want none", got)
	}
}

func TestInstalledAt(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator)+"Program Files", "Manga Downloader")
	exe := filepath.Join(dir, "manga-downloader-gui.exe")
	none := func(string) bool { return false }
	cases := []struct {
		name, exe, location string
		exists              func(string) bool
		want                bool
	}{
		{"registry match", exe, dir, none, true},
		{"registry match, case and slash", exe, strings.ToLower(dir) + string(filepath.Separator), none, true},
		{"registry elsewhere", exe, string(filepath.Separator) + "Other", func(string) bool { return true }, false},
		{"no key, uninstaller next to it", exe, "", func(p string) bool { return strings.HasSuffix(p, "uninstall.exe") }, true},
		{"no key, no uninstaller", exe, "", none, false},
	}
	for _, tc := range cases {
		if got := installedAt(tc.exe, tc.location, tc.exists); got != tc.want {
			t.Errorf("%s: installedAt = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDetectScope(t *testing.T) {
	sep := string(filepath.Separator)
	user := filepath.Join(sep+"Users", "me", "AppData", "Local", "Programs", "Manga Downloader")
	machine := filepath.Join(sep+"Program Files", "Manga Downloader")
	exeIn := func(d string) string { return filepath.Join(d, "manga-downloader-gui.exe") }
	none := func(string) bool { return false }
	keys := func(u, m string) func(installScope) string {
		return func(sc installScope) string {
			if sc == scopeUser {
				return u
			}

			return m
		}
	}
	cases := []struct {
		name   string
		exe    string
		read   func(installScope) string
		exists func(string) bool
		want   installScope
	}{
		{"per user", exeIn(user), keys(user, ""), none, scopeUser},
		{"per machine", exeIn(machine), keys("", machine), none, scopeMachine},
		{"both installs, running the machine one", exeIn(machine), keys(user, machine), none, scopeMachine},
		{"both installs, running the user one", exeIn(user), keys(user, machine), none, scopeUser},
		{"portable copy", exeIn(sep + "Tmp"), keys(user, machine), none, 0},
		{"stale keys, uninstaller next to it", exeIn(sep + "Tmp"), keys(user, machine), func(p string) bool { return strings.HasSuffix(p, "uninstall.exe") }, scopeMachine},
		{"no keys, uninstaller next to it", exeIn(sep + "Tmp"), keys("", ""), func(p string) bool { return strings.HasSuffix(p, "uninstall.exe") }, scopeMachine},
		{"no keys, no uninstaller", exeIn(sep + "Tmp"), keys("", ""), none, 0},
	}
	for _, tc := range cases {
		got, ok := detectScope(tc.exe, tc.read, tc.exists)
		if got != tc.want || ok != (tc.want != 0) {
			t.Errorf("%s: detectScope = %v, %v; want %v", tc.name, got, ok, tc.want)
		}
	}
}

func TestCheckUpdatePicksThePackageAndNeedsSums(t *testing.T) {
	pkg := packageName("v2.0.0", runtime.GOARCH)
	if pkg == "" {
		t.Skip("no one-click update on this system")
	}
	release := func(names ...string) string {
		var parts []string
		for _, n := range names {
			parts = append(parts, `{"name":"`+n+`","browser_download_url":"`+installerURLPrefix+`v2.0.0/`+n+`","size":9}`)
		}

		return `{"tag_name":"v2.0.0","assets":[` + strings.Join(parts, ",") + `]}`
	}

	srv, _ := releaseServer(t, http.StatusOK, release(pkg, sumsName("v2.0.0")))
	got, err := checkUpdate(context.Background(), "v1.12.0", srv.URL, srv.Client())
	if err != nil || got == nil {
		t.Fatalf("checkUpdate = %+v, %v", got, err)
	}
	if got.Installer == nil || got.Sums == nil {
		t.Errorf("Installer = %+v, Sums = %+v", got.Installer, got.Sums)
	}

	srv, _ = releaseServer(t, http.StatusOK, release(pkg))
	got, err = checkUpdate(context.Background(), "v1.12.0", srv.URL, srv.Client())
	if err != nil || got == nil {
		t.Fatalf("checkUpdate = %+v, %v", got, err)
	}
	if got.Installer != nil {
		t.Errorf("a release without checksums offers %+v", got.Installer)
	}
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))

	return hex.EncodeToString(h[:])
}

func TestParseSums(t *testing.T) {
	good := sha("a")
	data := good + "  one.zip\n" + strings.ToUpper(good) + " *two.zip\r\n" +
		"short  three.zip\n" + strings.Repeat("z", 64) + "  four.zip\n\n" + good + "\n"
	got := parseSums([]byte(data))
	if len(got) != 2 || got["one.zip"] != good || got["two.zip"] != good {
		t.Errorf("parseSums = %v", got)
	}
}

func TestVerifyChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyChecksum(path, "f", map[string]string{"f": sha("12345")}); err != nil {
		t.Errorf("matching sum: %v", err)
	}
	if err := verifyChecksum(path, "f", map[string]string{"f": sha("other")}); err == nil {
		t.Error("a mismatch passed")
	}
	if err := verifyChecksum(path, "f", map[string]string{"g": sha("12345")}); err == nil {
		t.Error("a missing entry passed")
	}
}

func TestDownloadInstaller(t *testing.T) {
	var sums string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "SHA256SUMS.txt") {
			_, _ = w.Write([]byte(sums))

			return
		}
		_, _ = w.Write([]byte("12345"))
	}))
	t.Cleanup(srv.Close)
	client := &http.Client{Transport: rewriteTo(srv)}
	sumsAsset := func() *Asset {
		a := asset("SHA256SUMS.txt", int64(len(sums)))

		return &a
	}

	a := asset("installer.exe", 5)
	sums = sha("12345") + "  installer.exe\n"
	path, err := downloadInstaller(context.Background(), client, &a, sumsAsset())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
	if b, _ := os.ReadFile(path); string(b) != "12345" {
		t.Errorf("saved %q", b)
	}

	for _, size := range []int64{4, 6} {
		bad := asset("installer.exe", size)
		if p, err := downloadInstaller(context.Background(), client, &bad, sumsAsset()); err == nil {
			_ = os.RemoveAll(filepath.Dir(p))
			t.Errorf("size %d: want a size mismatch error", size)
		}
	}

	sums = sha("tampered") + "  installer.exe\n"
	if p, err := downloadInstaller(context.Background(), client, &a, sumsAsset()); err == nil {
		_ = os.RemoveAll(filepath.Dir(p))
		t.Error("a checksum mismatch was accepted")
	}
	sums = sha("12345") + "  other.exe\n"
	if p, err := downloadInstaller(context.Background(), client, &a, sumsAsset()); err == nil {
		_ = os.RemoveAll(filepath.Dir(p))
		t.Error("a missing checksum entry was accepted")
	}
	if p, err := downloadInstaller(context.Background(), client, &a, nil); err == nil {
		_ = os.RemoveAll(filepath.Dir(p))
		t.Error("a download without checksums was accepted")
	}
}

type tarEntry struct {
	name, body, link string
	typ              byte
	mode             int64
}

func makeTarGz(t *testing.T, entries []tarEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "u.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: e.mode, Linkname: e.link}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			_, _ = tw.Write([]byte(e.body))
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestExtractTarGz(t *testing.T) {
	dest := t.TempDir()
	src := makeTarGz(t, []tarEntry{
		{name: "app/", typ: tar.TypeDir, mode: 0o755},
		{name: "app/install.sh", body: "#!/bin/sh\n", mode: 0o755},
		{name: "app/icon.png", body: "png", mode: 0o644},
	})
	if err := extractTarGz(src, dest); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dest, "app", "install.sh"))
	if err != nil || st.Mode().Perm()&0o100 == 0 {
		t.Errorf("install.sh = %v, %v", st, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "app", "icon.png")); string(b) != "png" {
		t.Errorf("icon = %q", b)
	}
}

func TestExtractTarGzRefusesUnsafeEntries(t *testing.T) {
	bad := map[string]tarEntry{
		"absolute":      {name: "/tmp/evil", body: "x", mode: 0o644},
		"parent":        {name: "../evil", body: "x", mode: 0o644},
		"nested parent": {name: "app/../../evil", body: "x", mode: 0o644},
		"symlink":       {name: "app/link", typ: tar.TypeSymlink, link: "/etc/passwd"},
		"relative link": {name: "app/link", typ: tar.TypeSymlink, link: "../.."},
		"hardlink":      {name: "app/hard", typ: tar.TypeLink, link: "/etc/passwd"},
		"device":        {name: "app/dev", typ: tar.TypeChar},
	}
	for name, e := range bad {
		root := t.TempDir()
		dest := filepath.Join(root, "dest")
		if err := os.Mkdir(dest, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := extractTarGz(makeTarGz(t, []tarEntry{e}), dest); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if left, _ := filepath.Glob(filepath.Join(root, "*")); len(left) != 1 {
			t.Errorf("%s: wrote outside dest: %v", name, left)
		}
	}
}

// rewriteTo sends every request to srv, whatever its URL says
func rewriteTo(srv *httptest.Server) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r2 := r.Clone(r.Context())
		r2.URL.Scheme = "http"
		r2.URL.Host = strings.TrimPrefix(srv.URL, "http://")

		return http.DefaultTransport.RoundTrip(r2)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAppUpdate(t *testing.T) {
	inst := asset("installer.exe", 5)
	sums := asset("SHA256SUMS.txt", 3)
	newTest := func(installed bool, installer, sums *Asset) (*App, *[]string) {
		c := NewController(&grabber.Settings{}, nil)
		c.update = &Update{Latest: "v2.0.0", Installer: installer, Sums: sums}
		a := newApp(c, &fakeSink{}, "", "v1.0.0", "v1.0.0")
		a.installed = installed
		var calls []string
		dir := filepath.Join(t.TempDir(), updateDirPrefix+"x")
		path := filepath.Join(dir, "i.exe")
		a.fetch = func(context.Context, *Asset, *Asset) (string, error) {
			calls = append(calls, "fetch")

			return path, os.MkdirAll(dir, 0o700)
		}
		a.install = func(string) error { calls = append(calls, "install"); return nil }
		a.quit = func() { calls = append(calls, "quit") }

		return a, &calls
	}

	a, calls := newTest(true, &inst, &sums)
	if st := a.State(); st.Update == nil || !st.Update.OneClick {
		t.Errorf("OneClick = %+v", st.Update)
	}
	if err := a.Update(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*calls, ","); got != "fetch,install,quit" {
		t.Errorf("calls = %s", got)
	}

	for name, a := range map[string]*App{
		"not installed": func() *App { a, _ := newTest(false, &inst, &sums); return a }(),
		"no installer":  func() *App { a, _ := newTest(true, nil, &sums); return a }(),
		"no checksums":  func() *App { a, _ := newTest(true, &inst, nil); return a }(),
	} {
		if st := a.State(); st.Update.OneClick {
			t.Errorf("%s: OneClick is set", name)
		}
		if err := a.Update(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}

	a, calls = newTest(true, &inst, &sums)
	a.install = func(string) error { return errNotRestarted }
	if err := a.Update(); err == nil || !strings.Contains(err.Error(), "open Manga Downloader again") || strings.Join(*calls, ",") != "fetch,quit" {
		t.Errorf("failed restart: %v, calls %v", err, *calls)
	}

	a, calls = newTest(true, &inst, &sums)
	a.install = func(string) error { return errors.New("denied") }
	if err := a.Update(); err == nil || len(*calls) != 1 {
		t.Errorf("failed install: %v, calls %v", err, *calls)
	}
	var last LogLine
	if l := a.ctrl.Log(); len(l) > 0 {
		last = l[len(l)-1]
	}
	if last.Level != LogError || !strings.Contains(last.Text, "denied") {
		t.Errorf("last log line = %+v, want the error", last)
	}
}

func runtimeArchIs(arch string) bool { return runtime.GOARCH == arch }

func TestRemoveUpdateDirRefusesOthers(t *testing.T) {
	other := t.TempDir()
	if err := removeUpdateDir(other); err == nil {
		t.Error("removed a directory that isn't an update directory")
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("the directory is gone: %v", err)
	}
	if err := removeUpdateDir(filepath.Dir(other)); err == nil {
		t.Error("removed a parent directory")
	}
}

func TestSweepUpdateDirs(t *testing.T) {
	tmp := t.TempDir()
	stale := filepath.Join(tmp, updateDirPrefix+"1")
	keep := filepath.Join(tmp, "other")
	for _, d := range []string{stale, keep} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	sweepUpdateDirs(tmp)
	if _, err := os.Stat(stale); err == nil {
		t.Error("the stale directory is still there")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("an unrelated directory was removed")
	}
}
