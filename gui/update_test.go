// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

func TestNewerRelease(t *testing.T) {
	cases := []struct {
		current, latest string
		want            *Update
		wantErr         bool
	}{
		{current: "v1.12.0", latest: "v1.13.0", want: &Update{Latest: "v1.13.0"}},
		{current: "v1.12.0", latest: "v1.12.1", want: &Update{Latest: "v1.12.1"}},
		{current: "v1.12.9", latest: "v1.13.0", want: &Update{Latest: "v1.13.0"}},
		{current: "v1.12.0", latest: "v2.0.0", want: &Update{Latest: "v2.0.0"}},
		{current: "1.12.0", latest: "v1.12.0"},
		{current: "v1.13.0", latest: "v1.12.5"},
		{current: "v1.13.0-rc.1", latest: "v1.13.0"},
		{current: "v1.12.0", latest: "v1.13.0-rc.1", want: &Update{Latest: "v1.13.0-rc.1"}},
		{current: "v1.12.0", latest: "v1.12.0+build.7"},
		{current: "v1.12.0-3-gabc", latest: "v1.13.0", want: &Update{Latest: "v1.13.0"}},
		{current: "v1.12.0-3-gabc", latest: "v1.12.0"},
		{current: "v1.12.0", latest: "v1.12.0-3-gabc"},
		{current: "dev", latest: "v1.13.0", wantErr: true},
		{current: "v1.12.0", latest: "v1.13", wantErr: true},
		{current: "v1.12.0", latest: "vX.1.0", wantErr: true},
	}
	for _, tc := range cases {
		got, err := newerRelease(tc.current, tc.latest)
		if tc.wantErr {
			if err == nil {
				t.Errorf("newerRelease(%q, %q): want an error", tc.current, tc.latest)
			}
			continue
		}
		if err != nil {
			t.Errorf("newerRelease(%q, %q): %v", tc.current, tc.latest, err)
			continue
		}
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("newerRelease(%q, %q) = %+v, want %+v", tc.current, tc.latest, got, tc.want)
		}
	}
}

// releaseServer answers the latest-release API with status and body, and
// remembers the request's headers
func releaseServer(t *testing.T, status int, body string) (*httptest.Server, *http.Header) {
	t.Helper()
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv, &seen
}

func TestCheckUpdateFetchesTheLatestTag(t *testing.T) {
	srv, seen := releaseServer(t, http.StatusOK, `{"tag_name":"v2.0.0","name":"x"}`)

	got, err := checkUpdate(context.Background(), "v1.12.0", srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Latest != "v2.0.0" {
		t.Fatalf("checkUpdate = %+v, want an update to v2.0.0", got)
	}
	if accept := (*seen).Get("Accept"); accept != "application/vnd.github+json" {
		t.Errorf("Accept = %q", accept)
	}
	if (*seen).Get("User-Agent") == "" {
		t.Error("the request has no User-Agent")
	}
}

func TestCheckUpdateErrors(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"http error":   {http.StatusForbidden, `{"message":"rate limited"}`},
		"bad JSON":     {http.StatusOK, `<html>`},
		"no tag":       {http.StatusOK, `{"name":"untagged"}`},
		"not a semver": {http.StatusOK, `{"tag_name":"nightly"}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv, _ := releaseServer(t, tc.status, tc.body)
			if got, err := checkUpdate(context.Background(), "v1.12.0", srv.URL, srv.Client()); err == nil || got != nil {
				t.Errorf("checkUpdate = %+v, %v; want no update and an error", got, err)
			}
		})
	}
}

func TestCheckUpdateRecordsTheUpdateAndStaysSilentOnFailure(t *testing.T) {
	notified := 0
	c := NewController(&grabber.Settings{}, func() { notified++ })

	srv, _ := releaseServer(t, http.StatusOK, `{"tag_name":"v1.13.0"}`)
	c.CheckUpdate(context.Background(), "v1.12.0", srv.URL, srv.Client())
	if u := c.Update(); u == nil || u.Latest != "v1.13.0" {
		t.Fatalf("Update() = %+v, want a minor update to v1.13.0", u)
	}
	if notified == 0 {
		t.Error("finding an update doesn't notify the window")
	}

	failing, _ := releaseServer(t, http.StatusInternalServerError, "")
	c = NewController(&grabber.Settings{}, func() { notified++ })
	notified = 0
	c.CheckUpdate(context.Background(), "v1.12.0", failing.URL, failing.Client())
	if u := c.Update(); u != nil {
		t.Errorf("Update() = %+v after a failed check, want nil", u)
	}
	if notified != 0 {
		t.Error("a failed check notifies the window")
	}
	if log := c.Log(); len(log) != 0 {
		t.Errorf("log = %+v after a failed check, want nothing in the window", log)
	}
}

func TestCheckUpdateStopsWithItsContext(t *testing.T) {
	srv, _ := releaseServer(t, http.StatusOK, `{"tag_name":"v2.0.0"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := checkUpdate(ctx, "v1.12.0", srv.URL, srv.Client()); err == nil || got != nil {
		t.Errorf("checkUpdate with a cancelled context = %+v, %v; want no update and an error", got, err)
	}
}

func TestShouldCheckUpdates(t *testing.T) {
	on, off := PrefsDTO{CheckUpdates: true}, PrefsDTO{}
	cases := []struct {
		tag   string
		prefs PrefsDTO
		want  bool
	}{
		{"", on, false},
		{"", off, false},
		{"v1.12.0", off, false},
		{"v1.12.0", on, true},
	}
	for _, c := range cases {
		if got := shouldCheckUpdates(c.tag, c.prefs); got != c.want {
			t.Errorf("shouldCheckUpdates(%q, %+v) = %v, want %v", c.tag, c.prefs, got, c.want)
		}
	}
}

func TestUpdateCheckStartsOncePerSession(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	if !c.claimUpdateCheck() {
		t.Fatal("the first check was refused")
	}
	if c.claimUpdateCheck() {
		t.Error("a second check in the same session was allowed")
	}
}

func TestReleaseURL(t *testing.T) {
	if got := releaseURL(""); got != "" {
		t.Errorf("releaseURL(\"\") = %q, want none for a dev build", got)
	}
	if got := releaseURL("v1.12.0"); got != repoURL+"/releases/tag/v1.12.0" {
		t.Errorf("releaseURL = %q", got)
	}
}

func TestStateCarriesTheReleaseAndTheUpdate(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	a := newApp(c, &fakeSink{}, "", "v1.12.0", "v1.12.0")
	if st := a.State(); st.ReleaseURL != releaseURL("v1.12.0") || st.Update != nil {
		t.Fatalf("State() before a check = %q, %+v", st.ReleaseURL, st.Update)
	}

	srv, _ := releaseServer(t, http.StatusOK, `{"tag_name":"v2.0.0"}`)
	c.CheckUpdate(context.Background(), "v1.12.0", srv.URL, srv.Client())
	want := UpdateDTO{Latest: "v2.0.0", URL: latestReleasePage}
	if st := a.State(); st.Update == nil || *st.Update != want {
		t.Errorf("State().Update = %+v, want %+v", st.Update, want)
	}

	dev := newApp(NewController(&grabber.Settings{}, nil), &fakeSink{}, "", "", "abc123")
	if st := dev.State(); st.ReleaseURL != "" {
		t.Errorf("a dev build's ReleaseURL = %q, want none", st.ReleaseURL)
	}
}
