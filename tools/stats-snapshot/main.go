// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// stats-snapshot stores a daily raw snapshot of downloads, pulls and stars, since
// GitHub and Docker Hub only expose current totals, and rebuilds daily.csv from all of them.
//
// Usage: go run ./tools/stats-snapshot -dir DIR [-repo owner/name] [-date YYYY-MM-DD]
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const snapshotVersion = 1

var (
	githubAPI = "https://api.github.com"
	dockerAPI = "https://hub.docker.com/v2/repositories/elboletaire/manga-downloader/"
	dateRe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// Snapshot is the raw data of one day.
type Snapshot struct {
	Version  int       `json:"version"`
	TakenAt  time.Time `json:"taken_at"`
	Repo     RepoStats `json:"repo"`
	Releases []Release `json:"releases"`
	Docker   *Docker   `json:"docker"`
}

type RepoStats struct {
	Stars       int `json:"stars"`
	Forks       int `json:"forks"`
	Subscribers int `json:"subscribers"`
	OpenIssues  int `json:"open_issues"`
}

type Release struct {
	Tag         string    `json:"tag"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

type Asset struct {
	Name          string `json:"name"`
	DownloadCount int    `json:"download_count"`
}

type Docker struct {
	PullCount int `json:"pull_count"`
	StarCount int `json:"star_count"`
}

func main() {
	dir := flag.String("dir", "", "data directory (required)")
	repo := flag.String("repo", "elboletaire/manga-downloader", "GitHub repository")
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "date of the snapshot (YYYY-MM-DD)")
	flag.Parse()

	if *dir == "" || !dateRe.MatchString(*date) {
		fmt.Fprintln(os.Stderr, "usage: stats-snapshot -dir DIR [-repo owner/name] [-date YYYY-MM-DD]")
		os.Exit(2)
	}
	if err := run(*dir, *repo, *date, os.Getenv("GITHUB_TOKEN")); err != nil {
		fmt.Fprintln(os.Stderr, "stats-snapshot:", err)
		os.Exit(1)
	}
}

func run(dir, repo, date, token string) error {
	snap, err := fetch(repo, token)
	if err != nil {
		return err
	}
	snapDir := filepath.Join(dir, "snapshots")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(snapDir, date+".json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	return writeCSV(dir)
}

func fetch(repo, token string) (*Snapshot, error) {
	var info struct {
		Stars       int `json:"stargazers_count"`
		Forks       int `json:"forks_count"`
		Subscribers int `json:"subscribers_count"`
		OpenIssues  int `json:"open_issues_count"`
	}
	if err := getJSON(fmt.Sprintf("%s/repos/%s", githubAPI, repo), token, &info); err != nil {
		return nil, err
	}
	snap := &Snapshot{
		Version:  snapshotVersion,
		TakenAt:  time.Now().UTC().Truncate(time.Second),
		Repo:     RepoStats{info.Stars, info.Forks, info.Subscribers, info.OpenIssues},
		Releases: []Release{},
	}

	for page := 1; ; page++ {
		var rels []struct {
			Tag         string    `json:"tag_name"`
			Draft       bool      `json:"draft"`
			PublishedAt time.Time `json:"published_at"`
			Assets      []struct {
				Name          string `json:"name"`
				DownloadCount int    `json:"download_count"`
			} `json:"assets"`
		}
		url := fmt.Sprintf("%s/repos/%s/releases?per_page=100&page=%d", githubAPI, repo, page)
		if err := getJSON(url, token, &rels); err != nil {
			return nil, err
		}
		for _, r := range rels {
			if r.Draft {
				continue
			}
			rel := Release{Tag: r.Tag, PublishedAt: r.PublishedAt, Assets: []Asset{}}
			for _, a := range r.Assets {
				rel.Assets = append(rel.Assets, Asset{a.Name, a.DownloadCount})
			}
			snap.Releases = append(snap.Releases, rel)
		}
		if len(rels) < 100 {
			break
		}
	}

	// Docker Hub is best effort: a failure leaves it null instead of failing the run.
	var d struct {
		PullCount int `json:"pull_count"`
		StarCount int `json:"star_count"`
	}
	if err := getJSON(dockerAPI, "", &d); err != nil {
		fmt.Fprintln(os.Stderr, "stats-snapshot: docker hub:", err)
	} else {
		snap.Docker = &Docker{d.PullCount, d.StarCount}
	}
	return snap, nil
}

func getJSON(url, token string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "manga-downloader-stats-snapshot")
	if strings.HasPrefix(url, githubAPI) {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// classify sums the download counts per kind, skipping checksum files.
func classify(releases []Release) (cli, app int) {
	for _, r := range releases {
		for _, a := range r.Assets {
			switch {
			case strings.HasSuffix(a.Name, ".md5"), strings.HasSuffix(a.Name, "-SHA256SUMS.txt"):
			case strings.HasPrefix(a.Name, "manga-downloader-gui-"):
				app += a.DownloadCount
			case strings.HasPrefix(a.Name, "manga-downloader-"):
				cli += a.DownloadCount
			}
		}
	}
	return cli, app
}

// row is one daily.csv line; docker is nil when Docker Hub failed that day.
type row struct {
	date            string
	cli, app, total int
	docker          *int
	stars, forks    int
}

func writeCSV(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "snapshots", "*.json"))
	if err != nil {
		return err
	}
	sort.Strings(files)

	var rows []row
	for _, f := range files {
		date := strings.TrimSuffix(filepath.Base(f), ".json")
		if !dateRe.MatchString(date) {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var s Snapshot
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		cli, app := classify(s.Releases)
		r := row{date: date, cli: cli, app: app, total: cli + app, stars: s.Repo.Stars, forks: s.Repo.Forks}
		if s.Docker != nil {
			r.docker = &s.Docker.PullCount
		}
		rows = append(rows, r)
	}

	out, err := os.Create(filepath.Join(dir, "daily.csv"))
	if err != nil {
		return err
	}
	defer out.Close()
	w := csv.NewWriter(out)
	w.Write([]string{"date", "cli_downloads", "app_downloads", "total_downloads", "docker_pulls", "stars", "forks",
		"cli_delta", "app_delta", "total_delta", "docker_delta", "stars_delta"})
	for i, r := range rows {
		rec := []string{r.date, itoa(r.cli), itoa(r.app), itoa(r.total), optInt(r.docker), itoa(r.stars), itoa(r.forks)}
		if i == 0 {
			rec = append(rec, "", "", "", "", "")
		} else {
			p := rows[i-1]
			rec = append(rec, itoa(r.cli-p.cli), itoa(r.app-p.app), itoa(r.total-p.total), dockerDelta(r.docker, p.docker), itoa(r.stars-p.stars))
		}
		w.Write(rec)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return out.Close()
}

func itoa(n int) string { return strconv.Itoa(n) }

func optInt(n *int) string {
	if n == nil {
		return ""
	}
	return itoa(*n)
}

func dockerDelta(cur, prev *int) string {
	if cur == nil || prev == nil {
		return ""
	}
	return itoa(*cur - *prev)
}
