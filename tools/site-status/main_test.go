// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"strings"
	"testing"
)

const testMakefile = `
grabber: \
	grabber/a

grabber/alpha:
	go run . https://www.alpha.com/manga/x 1

# comment
grabber/beta:
	go run . https://w2.beta-site.online/y 3

grabber/gamma:
	go run . --browser-visible https://old-gamma.org/z 2

grabber/gamma2:
	go run . https://gamma.net/z 2

grabber/orphan:
	go run . https://nowhere.example/q 2

grabber/browser: \
	grabber/gamma
`

const testReadme = `# Title

| Site | Notes |
| --- | --- |
| [not.a.site](https://x.y) | outside the section |

## Supported sites

<details>

| Site | Notes | Last verified |
| --- | --- | --- |
| [alpha.com](https://alpha.com) |  | — |
| [beta-site.online](https://beta-site.online) | Beta, Arabic | ❌ failing since 2026-09-01 |
| [gamma.net](https://gamma.net) \* | Gamma, former old-gamma.org | 2026-01-01 |
| [lonely.org](https://lonely.org) | | — |

Legend | with a pipe.

</details>

## Next
`

func hosts() map[string]string { return targetHosts(testMakefile) }

func TestTargetHosts(t *testing.T) {
	got := hosts()
	want := map[string]string{"alpha": "alpha.com", "beta": "w2.beta-site.online", "gamma": "old-gamma.org", "gamma2": "gamma.net", "orphan": "nowhere.example"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
}

func TestUpdate(t *testing.T) {
	res := map[string]bool{"alpha": true, "beta": false, "gamma": true, "gamma2": false, "orphan": true, "ghost": true}
	out, unmapped, err := update(testReadme, hosts(), res, "2026-10-15")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| [alpha.com](https://alpha.com) |  | 2026-10-15 |",
		// beta keeps its since date and gets this run's check.
		"| [beta-site.online](https://beta-site.online) | Beta, Arabic | ❌ failing since 2026-09-01 (checked 2026-10-15) |",
		// gamma and gamma2 share the row; one failing target fails it.
		"| [gamma.net](https://gamma.net) \\* | Gamma, former old-gamma.org | ❌ failing since 2026-10-15 (checked 2026-10-15) |",
		"| [lonely.org](https://lonely.org) | | — |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	got := strings.Join(unmapped, "\n")
	for _, want := range []string{"target orphan", "target ghost", "row lonely.org"} {
		if !strings.Contains(got, want) {
			t.Errorf("unmapped lacks %q: %s", want, got)
		}
	}
	// Everything but the three updated cells stays byte for byte.
	if n := diffLines(testReadme, out); n != 3 {
		t.Errorf("changed %d lines, want 3", n)
	}
}

func TestFailingSinceKeptThenCleared(t *testing.T) {
	r := map[string]bool{"beta": false}
	out, _, _ := update(testReadme, hosts(), r, "2026-10-15")
	if !strings.Contains(out, "❌ failing since 2026-09-01 (checked 2026-10-15) |") {
		t.Errorf("failing date not kept:\n%s", out)
	}
	out, _, _ = update(testReadme, hosts(), map[string]bool{"beta": true}, "2026-10-15")
	if !strings.Contains(out, "| Beta, Arabic | 2026-10-15 |") {
		t.Errorf("pass did not replace the failure:\n%s", out)
	}
}

func TestIdempotent(t *testing.T) {
	res := map[string]bool{"alpha": true, "beta": false}
	once, _, _ := update(testReadme, hosts(), res, "2026-10-15")
	twice, _, _ := update(once, hosts(), res, "2026-10-15")
	if once != twice {
		t.Errorf("second run changed the README")
	}
}

func TestRealFilesMapEveryPassingTarget(t *testing.T) {
	mk, err := os.ReadFile("../../makefile")
	if err != nil {
		t.Fatal(err)
	}
	rd, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	res := map[string]bool{}
	for target := range targetHosts(string(mk)) {
		res[target] = true
	}
	_, unmapped, err := update(string(rd), targetHosts(string(mk)), res, "2026-10-15")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range unmapped {
		if strings.HasPrefix(u, "target ") {
			t.Errorf("makefile target without a README row: %s", u)
		}
	}
}

func TestParseResults(t *testing.T) {
	got, err := parseResults("a pass\ngrabber/b fail\n\n")
	if err != nil || !got["a"] || got["b"] || len(got) != 2 {
		t.Errorf("got %v, %v", got, err)
	}
	if _, err := parseResults("a maybe\n"); err == nil {
		t.Error("want an error for a bad line")
	}
}

func diffLines(a, b string) int {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	if len(al) != len(bl) {
		return -1
	}
	n := 0
	for i := range al {
		if al[i] != bl[i] {
			n++
		}
	}
	return n
}

func TestOldest(t *testing.T) {
	// alpha and orphan were never checked (a dash, or no row at all), gamma and
	// gamma2 share a row verified 2026-01-01, beta is failing since 2026-09-01.
	cands := []string{"beta", "gamma2", "orphan", "gamma", "alpha"}
	got, err := oldest(testReadme, hosts(), cands, len(cands), 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := "alpha orphan gamma gamma2 beta"; strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
	got, _ = oldest(testReadme, hosts(), cands, 3, 0)
	if want := "alpha orphan gamma"; strings.Join(got, " ") != want {
		t.Errorf("first 3: got %v, want %s", got, want)
	}
}

func TestListOldest(t *testing.T) {
	dir := t.TempDir()
	mk := dir + "/makefile"
	rd := dir + "/README.md"
	if err := os.WriteFile(mk, []byte(testMakefile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rd, []byte(testReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := listOldest(strings.NewReader("beta\n\n alpha \n"), &out, 5, 0, rd, mk); err != nil {
		t.Fatal(err)
	}
	if want := "alpha\nbeta\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestCellStates(t *testing.T) {
	row := func(cell string) string { return "| [x.com](https://x.com) |  | " + cell + " |" }
	cases := []struct {
		name, cell string
		pass       bool
		want       string
	}{
		{"first failure", "—", false, "❌ failing since 2026-10-18 (checked 2026-10-18)"},
		{"first failure over a date", "2026-10-15", false, "❌ failing since 2026-10-18 (checked 2026-10-18)"},
		{"still failing", "❌ failing since 2026-09-01 (checked 2026-10-04)", false, "❌ failing since 2026-09-01 (checked 2026-10-18)"},
		{"old form still failing", "❌ failing since 2026-09-01", false, "❌ failing since 2026-09-01 (checked 2026-10-18)"},
		{"recovery", "❌ failing since 2026-09-01 (checked 2026-10-04)", true, "2026-10-18"},
	}
	for _, c := range cases {
		readme := "## Supported sites\n\n| Site | Notes | Last verified |\n| --- | --- | --- |\n" + row(c.cell) + "\n\n## Next\n"
		out, _, err := update(readme, map[string]string{"x": "x.com"}, map[string]bool{"x": c.pass}, "2026-10-18")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, row(c.want)+"\n") {
			t.Errorf("%s: want %q in\n%s", c.name, row(c.want), out)
		}
	}
}

func TestOldestByCheckedDate(t *testing.T) {
	const mk = `
grabber/a:
	go run . https://a.com/x 1
grabber/b:
	go run . https://b.com/x 1
grabber/c:
	go run . https://c.com/x 1
grabber/d:
	go run . https://d.com/x 1
grabber/e:
	go run . https://e.com/x 1
`
	const readme = `## Supported sites

| Site | Notes | Last verified |
| --- | --- | --- |
| [a.com](https://a.com) |  | — |
| [b.com](https://b.com) |  | 2026-05-01 |
| [c.com](https://c.com) |  | ❌ failing since 2026-01-01 (checked 2026-10-01) |
| [d.com](https://d.com) |  | ❌ failing since 2026-03-01 (checked 2026-06-01) |
| [e.com](https://e.com) |  | ❌ failing since 2026-04-01 |

## Next
`
	// a: never checked; e: old form, queued by its since date; d: by its check.
	got, err := oldest(readme, targetHosts(mk), []string{"c", "d", "e", "b", "a"}, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := "a e b d c"; strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

func TestOldestOffset(t *testing.T) {
	// a and h were never checked, b-e share one date and f-g another.
	const readme = `## Supported sites

| Site | Notes | Last verified |
| --- | --- | --- |
| [a.com](https://a.com) |  | — |
| [h.com](https://h.com) |  | — |
| [b.com](https://b.com) |  | 2026-05-01 |
| [c.com](https://c.com) |  | 2026-05-01 |
| [d.com](https://d.com) |  | 2026-05-01 |
| [e.com](https://e.com) |  | 2026-05-01 |
| [f.com](https://f.com) |  | 2026-06-01 |
| [g.com](https://g.com) |  | 2026-06-01 |

## Next
`
	hosts := map[string]string{}
	var cands []string
	for _, l := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		hosts[l] = l + ".com"
		cands = append([]string{l}, cands...)
	}
	cases := []struct {
		n, offset int
		want      string
	}{
		{8, 0, "a h b c d e f g"},
		// each group rotates by its own size: the first of b-e moves to the back.
		{8, 1, "h a c d e b g f"},
		{3, 1, "h a c"},
		// an offset past the group wraps: 5 is 1 in a group of two and of four.
		{8, 5, "h a c d e b g f"},
		{8, 2, "a h d e b c f g"},
		// the never-checked group stays first at any offset.
		{2, 3, "h a"},
		{8, -1, "h a e b c d g f"},
	}
	for _, c := range cases {
		got, err := oldest(readme, hosts, cands, c.n, c.offset)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("n=%d offset=%d: got %v, want %s", c.n, c.offset, got, c.want)
		}
	}
}
