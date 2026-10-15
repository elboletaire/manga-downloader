// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// site-status rewrites the "Last verified" column of the README's Supported
// sites table from smoke-test results, and nothing else in the file.
//
// The results file has one "target pass|fail" per line, target being a
// makefile smoke target without its "grabber/" prefix. Each target is mapped
// to a README row by the host of its first URL in the makefile; targets and
// rows that don't map are reported, never guessed.
//
// Usage: go run ./tools/site-status -results FILE -date YYYY-MM-DD [-readme README.md] [-makefile makefile]
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const failPrefix = "❌ failing since "

var (
	// | [domain](url) \* | notes | verified |
	rowRe     = regexp.MustCompile(`^\| \[([^\]]+)\]\((\S+?)\)( \\\*)? \|(.*)\| ?([^|]*?) ?\|$`)
	targetRe  = regexp.MustCompile(`^grabber/([A-Za-z0-9_-]+):\s*$`)
	urlRe     = regexp.MustCompile(`https?://[^/\s"']+`)
	formerRe  = regexp.MustCompile(`former ([^,]+)`)
	domainRe  = regexp.MustCompile(`[a-z0-9-]+(?:\.[a-z0-9-]+)+`)
	dateRe    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	sectionRe = regexp.MustCompile(`(?i)^##\s+Supported sites\s*$`)
)

func main() {
	results := flag.String("results", "", "file with one \"target pass|fail\" per line")
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "date of the run (YYYY-MM-DD)")
	readme := flag.String("readme", "README.md", "README to update in place")
	makefile := flag.String("makefile", "makefile", "makefile holding the grabber/<target> recipes")
	flag.Parse()

	if *results == "" || !dateRe.MatchString(*date) {
		fmt.Fprintln(os.Stderr, "usage: site-status -results FILE -date YYYY-MM-DD [-readme FILE] [-makefile FILE]")
		os.Exit(2)
	}
	if err := run(*results, *date, *readme, *makefile); err != nil {
		fmt.Fprintln(os.Stderr, "site-status:", err)
		os.Exit(1)
	}
}

func run(resultsPath, date, readmePath, makefilePath string) error {
	rb, err := os.ReadFile(resultsPath)
	if err != nil {
		return err
	}
	results, err := parseResults(string(rb))
	if err != nil {
		return err
	}
	mb, err := os.ReadFile(makefilePath)
	if err != nil {
		return err
	}
	rd, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	out, unmapped, err := update(string(rd), targetHosts(string(mb)), results, date)
	if err != nil {
		return err
	}
	prefix := "unmapped: "
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		prefix = "::warning::unmapped: "
	}
	for _, m := range unmapped {
		fmt.Fprintln(os.Stderr, prefix+m)
	}
	if out == string(rd) {
		return nil
	}
	return os.WriteFile(readmePath, []byte(out), 0o644)
}

// parseResults reads "target pass|fail" lines into target -> passed.
func parseResults(s string) (map[string]bool, error) {
	res := map[string]bool{}
	for i, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) != 2 || (f[1] != "pass" && f[1] != "fail") {
			return nil, fmt.Errorf("results line %d: want \"target pass|fail\", got %q", i+1, line)
		}
		res[strings.TrimPrefix(f[0], "grabber/")] = f[1] == "pass"
	}
	return res, nil
}

// targetHosts maps each makefile smoke target to the host of its first URL.
func targetHosts(makefile string) map[string]string {
	hosts := map[string]string{}
	target := ""
	for _, line := range strings.Split(makefile, "\n") {
		if m := targetRe.FindStringSubmatch(line); m != nil {
			target = m[1]
			continue
		}
		if !strings.HasPrefix(line, "\t") {
			if !strings.HasPrefix(line, "#") {
				target = ""
			}
			continue
		}
		if target == "" || !strings.Contains(line, "go run .") {
			continue
		}
		if _, done := hosts[target]; done {
			continue
		}
		if u := urlRe.FindString(line); u != "" {
			hosts[target] = hostOf(u)
		}
	}
	return hosts
}

func hostOf(u string) string {
	return strings.TrimPrefix(strings.ToLower(u[strings.Index(u, "//")+2:]), "www.")
}

// rowDomains lists the domains a row answers to: its own, the host of its
// link and any "former x.y" in its notes.
func rowDomains(domain, url, notes string) []string {
	ds := []string{strings.TrimPrefix(strings.ToLower(domain), "www."), hostOf(url)}
	for _, f := range formerRe.FindAllStringSubmatch(notes, -1) {
		ds = append(ds, domainRe.FindAllString(strings.ToLower(f[1]), -1)...)
	}
	return ds
}

// matches reports how well host fits domain: 2 for equal, 1 for a subdomain, 0 for no.
func matches(host, domain string) int {
	switch {
	case host == domain:
		return 2
	case strings.HasSuffix(host, "."+domain):
		return 1
	}
	return 0
}

// update returns the README with the verified column rewritten from results,
// plus the targets and rows that couldn't be mapped.
func update(readme string, hosts map[string]string, results map[string]bool, date string) (string, []string, error) {
	lines := strings.Split(readme, "\n")
	type row struct {
		line    int
		domains []string
	}
	var rows []row
	inSection := false
	for i, l := range lines {
		if sectionRe.MatchString(l) {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(l, "## ") {
			break
		}
		if !inSection {
			continue
		}
		if m := rowRe.FindStringSubmatch(l); m != nil {
			rows = append(rows, row{i, rowDomains(m[1], m[2], m[4])})
		}
	}
	if len(rows) == 0 {
		return "", nil, fmt.Errorf("README has no Supported sites table")
	}

	// Per row: any failing target fails it; otherwise any passing one verifies it.
	verdict := map[int]bool{}
	var unmapped []string
	targets := make([]string, 0, len(results))
	for t := range results {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	for _, t := range targets {
		host, ok := hosts[t]
		if !ok {
			unmapped = append(unmapped, "target "+t+" has no URL in the makefile")
			continue
		}
		best, bestScore := -1, 0
		for ri, r := range rows {
			for _, d := range r.domains {
				if s := matches(host, d); s > bestScore {
					best, bestScore = ri, s
				}
			}
		}
		if best < 0 {
			unmapped = append(unmapped, "target "+t+" ("+host+") matches no README row")
			continue
		}
		if prev, seen := verdict[best]; seen {
			verdict[best] = prev && results[t]
		} else {
			verdict[best] = results[t]
		}
	}

	for ri, r := range rows {
		pass, ok := verdict[ri]
		if !ok {
			continue
		}
		lines[r.line] = withVerified(lines[r.line], pass, date)
	}
	for ri, r := range rows {
		if _, ok := verdict[ri]; !ok && hasTarget(r.domains, hosts) == "" {
			unmapped = append(unmapped, "row "+r.domains[0]+" has no smoke target")
		}
	}
	return strings.Join(lines, "\n"), unmapped, nil
}

// hasTarget returns a makefile target whose host fits one of the domains.
func hasTarget(domains []string, hosts map[string]string) string {
	for t, h := range hosts {
		for _, d := range domains {
			if matches(h, d) > 0 {
				return t
			}
		}
	}
	return ""
}

// withVerified replaces the last cell of a table row, keeping an existing
// "failing since" date while the site keeps failing.
func withVerified(line string, pass bool, date string) string {
	m := rowRe.FindStringSubmatchIndex(line)
	cell := line[m[10]:m[11]]
	next := date
	if !pass {
		next = failPrefix + date
		if strings.HasPrefix(cell, failPrefix) {
			next = cell
		}
	}
	return line[:m[10]] + next + line[m[11]:]
}
