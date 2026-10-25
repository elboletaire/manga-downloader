// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

// answer serves status and body, and fails the test when it's asked for
// unexpectedly
func answer(t *testing.T, status int, body string, want bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !want {
			t.Errorf("unexpected request to %s", r.URL)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestLatestUsesTheWorkerWhenItAnswers(t *testing.T) {
	gh := answer(t, http.StatusOK, `{"tag_name":"v2.0.0"}`, false)
	worker := answer(t, http.StatusOK, `{"tag_name":"v1.13.0"}`, true)

	got, err := Source{Worker: worker.URL, GitHub: gh.URL, HTTP: worker.Client()}.Latest(context.Background(), "cli", "v1.12.0")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"tag_name":"v1.13.0"}` {
		t.Errorf("Latest = %s, want the Worker's answer", got)
	}
}

func TestLatestFallsBackOnAFailedWorker(t *testing.T) {
	worker := answer(t, http.StatusInternalServerError, "", true)
	gh := answer(t, http.StatusOK, `{"tag_name":"v2.0.0"}`, true)
	got, err := Source{Worker: worker.URL, GitHub: gh.URL, HTTP: worker.Client()}.Latest(context.Background(), "gui", "v1.12.0")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"tag_name":"v2.0.0"}` {
		t.Errorf("Latest = %s, want GitHub's answer after a Worker 500", got)
	}
}

func TestLatestFallsBackOnABadWorkerBody(t *testing.T) {
	for name, body := range map[string]string{"garbage": "<html>", "empty": "", "no tag": `{"name":"x"}`} {
		t.Run(name, func(t *testing.T) {
			worker := answer(t, http.StatusOK, body, true)
			gh := answer(t, http.StatusOK, `{"tag_name":"v2.0.0"}`, true)
			got, err := Source{Worker: worker.URL, GitHub: gh.URL, HTTP: worker.Client()}.Latest(context.Background(), "cli", "v1.12.0")
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != `{"tag_name":"v2.0.0"}` {
				t.Errorf("Latest = %s, want GitHub's answer after a bad Worker body", got)
			}
		})
	}
}

func TestLatestFallsBackWhenTheWorkerIsUnreachable(t *testing.T) {
	// A closed server refuses the connection, as an unresolvable or down host does
	down := answer(t, http.StatusOK, "", false)
	down.Close()
	gh := answer(t, http.StatusOK, `{"tag_name":"v2.0.0"}`, true)

	got, err := Source{Worker: down.URL, GitHub: gh.URL, HTTP: gh.Client()}.Latest(context.Background(), "cli", "v1.12.0")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"tag_name":"v2.0.0"}` {
		t.Errorf("Latest = %s, want GitHub's answer after the Worker refused the connection", got)
	}
}

func TestLatestFallsBackOnATimeout(t *testing.T) {
	prev := timeout
	timeout = 20 * time.Millisecond
	t.Cleanup(func() { timeout = prev })

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(slow.Close)
	gh := answer(t, http.StatusOK, `{"tag_name":"v2.0.0"}`, true)

	got, err := Source{Worker: slow.URL, GitHub: gh.URL, HTTP: slow.Client()}.Latest(context.Background(), "cli", "v1.12.0")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"tag_name":"v2.0.0"}` {
		t.Errorf("Latest = %s, want GitHub's answer after a timeout", got)
	}
}

func TestLatestStopsWithItsContext(t *testing.T) {
	gh := answer(t, http.StatusOK, `{}`, false)
	worker := answer(t, http.StatusInternalServerError, "", true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Source{Worker: worker.URL, GitHub: gh.URL, HTTP: worker.Client()}).Latest(ctx, "cli", "v1.12.0"); err == nil {
		t.Error("Latest with a cancelled context returned no error")
	}
}

func TestWorkerQueryParams(t *testing.T) {
	var q map[string][]string
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/latest" {
			t.Errorf("path = %s, want /v1/latest", r.URL.Path)
		}
		q = r.URL.Query()
		_, _ = w.Write([]byte(`{"tag_name":"v23.0.0"}`))
	}))
	t.Cleanup(worker.Close)

	if _, err := (Source{Worker: worker.URL, GitHub: worker.URL, HTTP: worker.Client()}).Latest(context.Background(), "gui", "v23.0.0"); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"client": "gui", "version": "23.0.0", "os": runtime.GOOS, "arch": runtime.GOARCH}
	for k, v := range want {
		if len(q[k]) != 1 || q[k][0] != v {
			t.Errorf("query %s = %v, want %q", k, q[k], v)
		}
	}
	if len(q) != len(want) {
		t.Errorf("query = %v, want only %v", q, want)
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
		wantErr         bool
	}{
		{current: "v1.22.0", latest: "v23.0.0", want: true},
		{current: "v23.0.0", latest: "v1.22.0"},
		{current: "v23.0.0", latest: "v23.0.0"},
		{current: "v1.12.0", latest: "v1.12.1", want: true},
		{current: "v1.13.0-rc.1", latest: "v1.13.0"},
		{current: "v1.12.0-3-gabc", latest: "v1.13.0", want: true},
		{current: "develop", latest: "v1.13.0", wantErr: true},
		{current: "v1.12.0", latest: "nightly", wantErr: true},
	}
	for _, tc := range cases {
		got, err := Newer(tc.current, tc.latest)
		if tc.wantErr != (err != nil) {
			t.Errorf("Newer(%q, %q) error = %v, want error %v", tc.current, tc.latest, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestIsRelease(t *testing.T) {
	if !IsRelease("v1.22.0") || !IsRelease("v1.22.0-3-gabc") {
		t.Error("a release tag isn't a release")
	}
	if IsRelease("develop") || IsRelease("abc123") {
		t.Error("a dev build's name is a release")
	}
}
