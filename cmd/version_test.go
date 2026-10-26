// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/update"
)

// versionSource answers the update check with body, and checks it's asked as the CLI
func versionSource(t *testing.T, body string) update.Source {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/latest" && r.URL.Query().Get("client") != "cli" {
			t.Errorf("client = %q, want cli", r.URL.Query().Get("client"))
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return update.Source{Worker: srv.URL, GitHub: srv.URL, HTTP: srv.Client()}
}

func TestUpdateStatus(t *testing.T) {
	cases := []struct {
		name, current, body, want string
	}{
		{"up to date", "v23.0.0", `{"tag_name":"v23.0.0"}`, "App is up to date.\n"},
		{"outdated", "v1.22.0", `{"tag_name":"v23.0.0"}`,
			"App is outdated. Download latest (23.0.0) from:\nhttps://github.com/elboletaire/manga-downloader/releases/tag/v23.0.0\n"},
		{"no tag", "v1.22.0", `{}`, "Error checking for updates: the latest release has no tag\n"},
		{"not a release", "v1.22.0", `{"tag_name":"nightly"}`, "Error checking for updates: \"nightly\" isn't MAJOR.MINOR.PATCH\n"},
		{"dev build", "develop", `{"tag_name":"v1.22.0"}`, "Development build, no release to compare with.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := updateStatus(context.Background(), tc.current, versionSource(t, tc.body)); got != tc.want {
				t.Errorf("updateStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUpdateStatusSendsTheSystemLocale(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "fr_FR.UTF-8")
	var q url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		_, _ = w.Write([]byte(`{"tag_name":"v23.0.0"}`))
	}))
	t.Cleanup(srv.Close)

	updateStatus(context.Background(), "v1.22.0", update.Source{Worker: srv.URL, GitHub: srv.URL, HTTP: srv.Client()})
	if got := q.Get("syslang"); got != "fr-FR" {
		t.Errorf("syslang = %q, want fr-FR", got)
	}
	if _, ok := q["lang"]; ok {
		t.Errorf("lang sent = %q, want none, the CLI has no UI language", q.Get("lang"))
	}
}

func TestShowVersionHonoursTheEnvironmentSwitch(t *testing.T) {
	prevTag, prevSource := Tag, updateSource
	Tag, updateSource = "v1.22.0", update.Source{}
	t.Cleanup(func() { Tag, updateSource = prevTag, prevSource })

	t.Setenv(noUpdateCheckEnv, "1")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prevOut := os.Stdout
	os.Stdout = w
	showVersion()
	w.Close()
	os.Stdout = prevOut
	out, _ := io.ReadAll(r)
	s := string(out)
	if !strings.Contains(s, "Version: ") || !strings.Contains(s, "coffee") {
		t.Errorf("output with the check off = %q, want the version and the Ko-fi line", s)
	}
	if i := strings.Index(s, "Version: "); i < 0 || strings.Contains(s[i:], " - ") {
		t.Errorf("output with the check off = %q, want the version line without a status", s)
	}
	if strings.Contains(s, "update") || strings.Contains(s, "Error") {
		t.Errorf("output with the check off = %q, want nothing about updates", s)
	}
}
