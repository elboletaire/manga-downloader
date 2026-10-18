// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// site-status rewrites the "Last verified" column of the README's Supported
// sites table from smoke-test results, and nothing else in the file.
//
// The results file has one "target pass|fail" per line, target being a
// makefile smoke target without its "grabber/" prefix. Each target is mapped
// to a README row by the host of its first URL in the makefile; targets and
// rows that don't map are reported, never guessed.
//
// With -oldest N it instead prints the N targets read from stdin whose rows
// were verified longest ago, so the smoke rotation can pick them.
//
// Usage: go run ./tools/site-status -results FILE -date YYYY-MM-DD [-readme README.md] [-makefile makefile]
//
//	go run ./tools/site-status -oldest N [-readme README.md] [-makefile makefile] < targets
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
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
	// "❌ failing since D", or with "(checked E)" once a run has seen it fail
	failRe = regexp.MustCompile(`^❌ failing since (\d{4}-\d{2}-\d{2})(?: \(checked (\d{4}-\d{2}-\d{2})\))?$`)
)

func main() {
	results := flag.String("results", "", "file with one \"target pass|fail\" per line")
	date := flag.String("date", time.Now().UTC().Format("2006-01-02"), "date of the run (YYYY-MM-DD)")
	readme := flag.String("readme", "README.md", "README to update in place")
	makefile := flag.String("makefile", "makefile", "makefile holding the grabber/<target> recipes")
	count := flag.Int("oldest", 0, "print the N stdin targets whose rows were verified longest ago")
	flag.Parse()

	if *count > 0 {
		if err := listOldest(os.Stdin, os.Stdout, *count, *readme, *makefile); err != nil {
			fmt.Fprintln(os.Stderr, "site-status:", err)
			os.Exit(1)
		}
		return
	}
	if *results == "" || !dateRe.MatchString(*date) {
		fmt.Fprintln(os.Stderr, "usage: site-status -results FILE -date YYYY-MM-DD [-readme FILE] [-makefile FILE]")
		fmt.Fprintln(os.Stderr, "       site-status -oldest N [-readme FILE] [-makefile FILE] < targets")
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

// listOldest writes the n candidate targets read from in (one per line) whose
// rows were verified longest ago, one per line.
func listOldest(in io.Reader, out io.Writer, n int, readmePath, makefilePath string) error {
	var candidates []string
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			candidates = append(candidates, t)
		}
	}
	if err := sc.Err(); err != nil {
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
	targets, err := oldest(string(rd), targetHosts(string(mb)), candidates, n)
	if err != nil {
		return err
	}
	for _, t := range targets {
		fmt.Fprintln(out, t)
	}
	return nil
}

// oldest returns the n candidates whose rows were verified longest ago: never
// checked first (a candidate with no row counts as never checked), then by
// date, ties by name. A failing row is ordered by when it was last checked.
func oldest(readme string, hosts map[string]string, candidates []string, n int) ([]string, error) {
	rows, err := readRows(strings.Split(readme, "\n"))
	if err != nil {
		return nil, err
	}
	type due struct{ target, date string }
	dues := make([]due, 0, len(candidates))
	for _, t := range candidates {
		d := due{target: t}
		if host, ok := hosts[t]; ok {
			if ri := bestRow(host, rows); ri >= 0 {
				d.date = queueDate(rows[ri].verified)
			}
		}
		dues = append(dues, d)
	}
	sort.Slice(dues, func(i, j int) bool {
		if dues[i].date != dues[j].date {
			return dues[i].date < dues[j].date
		}
		return dues[i].target < dues[j].target
	})
	if n > len(dues) {
		n = len(dues)
	}
	out := make([]string, 0, n)
	for _, d := range dues[:n] {
		out = append(out, d.target)
	}
	return out, nil
}

// queueDate returns the day that queues a "Last verified" cell for the rotation:
// its date, a failing row's last check (its since date for the old form), or "".
func queueDate(cell string) string {
	c := strings.TrimSpace(cell)
	if m := failRe.FindStringSubmatch(c); m != nil {
		if m[2] != "" {
			return m[2]
		}
		return m[1]
	}
	if dateRe.MatchString(c) {
		return c
	}
	return ""
}

// failingSince returns the "failing since" date of a failing cell, or "".
func failingSince(cell string) string {
	if m := failRe.FindStringSubmatch(strings.TrimSpace(cell)); m != nil {
		return m[1]
	}
	return ""
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

// row is a Supported sites table row: its line, the domains it answers to and
// its "Last verified" cell.
type row struct {
	line     int
	domains  []string
	verified string
}

// readRows returns the rows of the Supported sites table.
func readRows(lines []string) ([]row, error) {
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
			rows = append(rows, row{i, rowDomains(m[1], m[2], m[4]), m[5]})
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("README has no Supported sites table")
	}
	return rows, nil
}

// bestRow returns the index of the row whose domains fit host best, or -1.
func bestRow(host string, rows []row) int {
	best, bestScore := -1, 0
	for ri, r := range rows {
		for _, d := range r.domains {
			if s := matches(host, d); s > bestScore {
				best, bestScore = ri, s
			}
		}
	}
	return best
}

// update returns the README with the verified column rewritten from results,
// plus the targets and rows that couldn't be mapped.
func update(readme string, hosts map[string]string, results map[string]bool, date string) (string, []string, error) {
	lines := strings.Split(readme, "\n")
	rows, err := readRows(lines)
	if err != nil {
		return "", nil, err
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
		best := bestRow(host, rows)
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

// withVerified replaces the last cell of a table row. A failure keeps the
// "failing since" date of an earlier failure and refreshes the check date.
func withVerified(line string, pass bool, date string) string {
	m := rowRe.FindStringSubmatchIndex(line)
	cell := line[m[10]:m[11]]
	next := date
	if !pass {
		since := failingSince(cell)
		if since == "" {
			since = date
		}
		next = fmt.Sprintf("%s%s (checked %s)", failPrefix, since, date)
	}
	return line[:m[10]] + next + line[m[11]:]
}
