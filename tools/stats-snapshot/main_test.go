// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releasesJSON = `[
 {"tag_name":"v2","draft":false,"published_at":"2026-01-02T00:00:00Z","assets":[
  {"name":"manga-downloader-linux-amd64","download_count":%d},
  {"name":"manga-downloader-linux-amd64.md5","download_count":99},
  {"name":"manga-downloader-v2-SHA256SUMS.txt","download_count":98},
  {"name":"manga-downloader-gui-v2-SHA256SUMS.txt","download_count":97},
  {"name":"manga-downloader-gui-linux-amd64.zip","download_count":%d}]},
 {"tag_name":"v3","draft":true,"published_at":"2026-01-03T00:00:00Z","assets":[
  {"name":"manga-downloader-draft","download_count":1000}]}
]`

// serve fakes GitHub and Docker Hub; dockerOK false makes Docker Hub answer 500.
func serve(t *testing.T, cli, app, pulls, stars int, dockerOK bool) {
	t.Helper()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases") {
			fmt.Fprintf(w, releasesJSON, cli, app)
			return
		}
		fmt.Fprintf(w, `{"stargazers_count":%d,"forks_count":3,"subscribers_count":2,"open_issues_count":1}`, stars)
	}))
	dk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !dockerOK {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, `{"pull_count":%d,"star_count":4}`, pulls)
	}))
	oldGH, oldDK := githubAPI, dockerAPI
	githubAPI, dockerAPI = gh.URL, dk.URL
	t.Cleanup(func() { githubAPI, dockerAPI = oldGH, oldDK; gh.Close(); dk.Close() })
}

func csvLines(t *testing.T, dir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "daily.csv"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestClassify(t *testing.T) {
	serve(t, 10, 5, 100, 7, true)
	s, err := fetch("o/r", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Releases) != 1 {
		t.Fatalf("draft not skipped: %d releases", len(s.Releases))
	}
	if len(s.Releases[0].Assets) != 5 {
		t.Fatalf("raw assets must keep checksum files: %v", s.Releases[0].Assets)
	}
	if cli, app := classify(s.Releases); cli != 10 || app != 5 {
		t.Fatalf("cli=%d app=%d", cli, app)
	}
}

func TestCSVDeltasWithGap(t *testing.T) {
	dir := t.TempDir()
	serve(t, 10, 5, 100, 7, true)
	if err := run(dir, "o/r", "2026-01-01", ""); err != nil {
		t.Fatal(err)
	}
	serve(t, 14, 6, 130, 9, true)
	// two days later: the gap is just a bigger delta
	if err := run(dir, "o/r", "2026-01-03", ""); err != nil {
		t.Fatal(err)
	}
	got := csvLines(t, dir)
	want := []string{
		"date,cli_downloads,app_downloads,total_downloads,docker_pulls,stars,forks,cli_delta,app_delta,total_delta,docker_delta,stars_delta",
		"2026-01-01,10,5,15,100,7,3,,,,,",
		"2026-01-03,14,6,20,130,9,3,4,1,5,30,2",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDockerFailureIsNull(t *testing.T) {
	dir := t.TempDir()
	serve(t, 10, 5, 0, 7, false)
	if err := run(dir, "o/r", "2026-01-01", ""); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "snapshots", "2026-01-01.json"))
	if !strings.Contains(string(b), `"docker": null`) {
		t.Fatalf("docker not null:\n%s", b)
	}
	if got := csvLines(t, dir)[1]; got != "2026-01-01,10,5,15,,7,3,,,,," {
		t.Fatalf("row = %q", got)
	}
}
