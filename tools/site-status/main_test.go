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
		"| [beta-site.online](https://beta-site.online) | Beta, Arabic | ❌ failing since 2026-09-01 |", // kept
		// gamma and gamma2 share the row; one failing target fails it.
		"| [gamma.net](https://gamma.net) \\* | Gamma, former old-gamma.org | ❌ failing since 2026-10-15 |",
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
	if n := diffLines(testReadme, out); n != 2 {
		t.Errorf("changed %d lines, want 2", n)
	}
}

func TestFailingSinceKeptThenCleared(t *testing.T) {
	r := map[string]bool{"beta": false}
	out, _, _ := update(testReadme, hosts(), r, "2026-10-15")
	if !strings.Contains(out, "❌ failing since 2026-09-01 |") {
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
