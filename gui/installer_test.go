// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
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

func TestPickInstaller(t *testing.T) {
	assets := []Asset{
		asset("manga-downloader-gui-v2.0.0-windows-amd64.zip", 5),
		asset("manga-downloader-gui-v2.0.0-windows-arm64-installer.exe", 7),
		asset("manga-downloader-gui-v2.0.0-windows-amd64-installer.exe", 9),
	}
	if got := pickInstaller(assets, "v2.0.0", "amd64"); got == nil || got.Size != 9 {
		t.Errorf("amd64 = %+v", got)
	}
	if got := pickInstaller(assets, "v2.0.0", "arm64"); got == nil || got.Size != 7 {
		t.Errorf("arm64 = %+v", got)
	}
	if got := pickInstaller(assets, "v2.0.0", "386"); got != nil {
		t.Errorf("386 = %+v, want none", got)
	}
	if got := pickInstaller(assets, "v2.0.1", "amd64"); got != nil {
		t.Errorf("another tag = %+v, want none", got)
	}
	foreign := []Asset{{Name: installerName("v2.0.0", "amd64"), URL: "https://evil.example/x.exe", Size: 1}}
	if got := pickInstaller(foreign, "v2.0.0", "amd64"); got != nil {
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

func TestCheckUpdatePicksTheInstaller(t *testing.T) {
	body := `{"tag_name":"v2.0.0","assets":[{"name":"` + installerName("v2.0.0", "amd64") +
		`","browser_download_url":"` + installerURLPrefix + `v2.0.0/` + installerName("v2.0.0", "amd64") + `","size":9}]}`
	srv, _ := releaseServer(t, http.StatusOK, body)
	got, err := checkUpdate(context.Background(), "v1.12.0", srv.URL, srv.Client())
	if err != nil || got == nil {
		t.Fatalf("checkUpdate = %+v, %v", got, err)
	}
	if (got.Installer != nil) != (runtimeArchIs("amd64")) {
		t.Errorf("Installer = %+v on this arch", got.Installer)
	}
}

func TestDownloadInstaller(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	t.Cleanup(srv.Close)
	client := &http.Client{Transport: rewriteTo(srv)}

	a := asset("installer.exe", 5)
	path, err := downloadInstaller(context.Background(), client, &a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
	if b, _ := os.ReadFile(path); string(b) != "12345" {
		t.Errorf("saved %q", b)
	}

	for _, size := range []int64{4, 6} {
		bad := asset("installer.exe", size)
		if p, err := downloadInstaller(context.Background(), client, &bad); err == nil {
			_ = os.RemoveAll(filepath.Dir(p))
			t.Errorf("size %d: want a size mismatch error", size)
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
	newTest := func(installed bool, installer *Asset) (*App, *[]string) {
		c := NewController(&grabber.Settings{}, nil)
		c.update = &Update{Latest: "v2.0.0", Installer: installer}
		a := newApp(c, &fakeSink{}, "", "v1.0.0", "v1.0.0")
		a.installed = installed
		var calls []string
		a.fetch = func(context.Context, *Asset) (string, error) {
			calls = append(calls, "fetch")
			return "/tmp/i.exe", nil
		}
		a.install = func(p string) error { calls = append(calls, "install "+p); return nil }
		a.quit = func() { calls = append(calls, "quit") }

		return a, &calls
	}

	a, calls := newTest(true, &inst)
	if st := a.State(); st.Update == nil || !st.Update.OneClick {
		t.Errorf("OneClick = %+v", st.Update)
	}
	if err := a.Update(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(*calls, ","); got != "fetch,install /tmp/i.exe,quit" {
		t.Errorf("calls = %s", got)
	}

	for name, a := range map[string]*App{
		"not installed": func() *App { a, _ := newTest(false, &inst); return a }(),
		"no installer":  func() *App { a, _ := newTest(true, nil); return a }(),
	} {
		if st := a.State(); st.Update.OneClick {
			t.Errorf("%s: OneClick is set", name)
		}
		if err := a.Update(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}

	a, calls = newTest(true, &inst)
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
