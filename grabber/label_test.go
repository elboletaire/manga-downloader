// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestChapterLabelGolden pins every grabber that composes labels to the titles
// it wrote before #326 (but for mangadex and inmanga, which drop the padding),
// and checks that the title it keeps is the site's own
func TestChapterLabelGolden(t *testing.T) {
	type row struct {
		name      string
		chapter   Chapter
		want      string // the old title, now the label
		siteTitle string
	}
	var rows []row
	format := func(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }

	// the site's title, or the chapter word and number when it gives none
	for _, g := range []struct{ name, lang, word string }{
		{"atsumaru", "", "Chapter"}, {"flamecomics", "", "Chapter"}, {"guya", "", "Chapter"},
		{"mangafire", "", "Chapter"}, {"manganelo", "", "Chapter"}, {"qimanga", "", "Chapter"},
		{"rsccomic", "", "Chapter"}, {"sacachispa", "", "Chapter"}, {"stonescape", "", "Chapter"},
		{"tanstackcomic", "", "Chapter"}, {"plainhtml", "", "Chapter"},
		{"fmteam", "fr", "Chapitre"}, {"mangadenizi", "tr", "Bölüm"},
	} {
		for _, c := range []struct {
			title  string
			number float64
		}{
			{"", 12}, {"", 10.5}, {"", 0}, {"Dead End", 36}, {"Chapter 186 - Foo", 186}, {"Capítulo 0012", 12},
		} {
			want := c.title
			if want == "" {
				want = g.word + " " + format(c.number)
			}
			rows = append(rows, row{fmt.Sprintf("%s %q %v", g.name, c.title, c.number),
				Chapter{Title: c.title, Number: c.number, Label: Label{Style: LabelNumbered, Lang: g.lang}}, want, c.title})
		}
	}
	// never a site title
	for _, g := range []string{"comix", "heavenmanga", "lelscans", "mangitto", "mgeko", "weebcentral", "blogger", "wordpresscategory"} {
		for _, n := range []float64{1, 10.5, 1196} {
			rows = append(rows, row{fmt.Sprintf("%s %v", g, n),
				Chapter{Number: n, Label: Label{Style: LabelNumbered}}, "Chapter " + format(n), ""})
		}
	}
	// a chapter split over several posts
	for _, g := range []string{"blogger", "wordpresscategory"} {
		rows = append(rows,
			row{g + " part", Chapter{Number: 364, Label: Label{Style: LabelNumbered, Part: "2"}}, "Chapter 364 Part 2", ""},
			row{g + " decimal part", Chapter{Number: 10.5, Label: Label{Style: LabelNumbered, Part: "1"}}, "Chapter 10.5 Part 1", ""})
	}
	// the site's number text, as written
	for _, g := range []struct{ name, lang, word string }{
		{"chainsawmanonline", "", "Chapter"}, {"heavenlydelusion", "", "Chapter"}, {"mkissa", "", "Chapter"},
		{"jestful", "", "Chapter"}, {"mangataro", "", "Chapter"}, {"roliascan", "", "Chapter"},
		{"toivolite", "", "Chapter"}, {"bigsolo", "fr", "Chapitre"},
	} {
		for _, raw := range []string{"231", "13.5", "05", "10.50"} {
			n, _ := strconv.ParseFloat(raw, 64)
			rows = append(rows, row{g.name + " " + raw,
				Chapter{Number: n, Label: Label{Style: LabelNumbered, Number: raw, Lang: g.lang}}, g.word + " " + raw, ""})
		}
		rows = append(rows, row{g.name + " titled",
			Chapter{Title: "Night", Number: 26, Label: Label{Style: LabelNumbered, Number: "26", Lang: g.lang}}, "Night", "Night"})
	}
	// "Chapter N", then " - title" when the site gives one
	for _, c := range []struct {
		title, number string
		want          string
	}{
		{"", "5", "Chapter 5"}, {"   ", "5", "Chapter 5"}, {"Foo", "5", "Chapter 5 - Foo"},
		{"  Foo  ", "10.5", "Chapter 10.5 -   Foo"}, {"Foo", "05", "Chapter 05 - Foo"}, {"", "05", "Chapter 05"},
	} {
		n, _ := strconv.ParseFloat(c.number, 64)
		rows = append(rows, row{fmt.Sprintf("teamshadowi %q %s", c.title, c.number),
			Chapter{Title: c.title, Number: n, Label: Label{Style: LabelPrefixed, Number: c.number}}, c.want, strings.TrimSpace(c.title)})
	}
	// a one-shot without a title
	rows = append(rows,
		row{"mangapark one-shot", Chapter{Number: 4, Label: Label{Style: LabelUnnumbered}}, "Chapter", ""},
		row{"mangapark titled one-shot", Chapter{Title: "Extra", Number: 4, Label: Label{Style: LabelUnnumbered}}, "Extra", "Extra"})
	// the number, then the title; mangadex's listing keeps the site's title
	for _, c := range []struct {
		title  string
		number float64
		want   string
	}{
		{"Market", 1, "Chapter 1 Market"}, {"", 1, "Chapter 1"}, {"", 10.5, "Chapter 10.5"},
		{"Foo", 10.5, "Chapter 10.5 Foo"}, {"Foo", 1186, "Chapter 1186 Foo"}, {"Chapter 5", 5, "Chapter 5 Chapter 5"},
	} {
		rows = append(rows,
			row{fmt.Sprintf("mangadex fetched %q %v", c.title, c.number),
				Chapter{Title: c.title, Number: c.number, Label: Label{Style: LabelNumberFirst}}, c.want, c.title},
			row{fmt.Sprintf("mangadex listed %q %v", c.title, c.number),
				Chapter{Title: c.title, Number: c.number}, c.title, c.title})
	}
	for _, n := range []float64{5, 912.5, 1186} {
		rows = append(rows, row{fmt.Sprintf("inmanga %v", n),
			Chapter{Number: n, Label: Label{Style: LabelNumbered, Lang: "es"}}, "Capítulo " + format(n), ""})
	}

	for _, r := range rows {
		if got := ChapterLabel(r.chapter); got != r.want {
			t.Errorf("%s: ChapterLabel = %q, want %q", r.name, got, r.want)
		}
		if got := r.chapter.GetTitle(); got != r.siteTitle {
			t.Errorf("%s: GetTitle = %q, want the site's %q", r.name, got, r.siteTitle)
		}
		// messages name the chapter from its label, as they did from the title
		old := Chapter{Title: r.want, Number: r.chapter.Number}
		if got, want := ChapterName(r.chapter), ChapterName(old); got != want {
			t.Errorf("%s: ChapterName = %q, want %q", r.name, got, want)
		}
	}
}

func TestChapterLabelHelpers(t *testing.T) {
	// a listing type embedding Chapter composes the same, and passes its Label on
	listed := &MangadexChapter{Chapter: Chapter{Number: 2, Label: Label{Style: LabelNumbered, Number: "02"}}}
	if got := ChapterLabel(listed); got != "Chapter 02" {
		t.Errorf("ChapterLabel(listed) = %q, want %q", got, "Chapter 02")
	}
	fetched := &Chapter{Title: listed.GetTitle(), Number: listed.Number, Label: labelOf(listed)}
	if got := ChapterLabel(fetched); got != "Chapter 02" {
		t.Errorf("fetched label = %q, want %q", got, "Chapter 02")
	}
	if got := ChapterLabel(&Chapter{Title: " Foo\nBar ", Number: 3}); got != "Foo Bar" {
		t.Errorf("LabelTitle = %q, want %q", got, "Foo Bar")
	}
}

// TestChapterLabelPart tells the halves of one episode apart: a numeric part
// reads "Part 2", a word part ("前編") reads as is, and either goes with the title
func TestChapterLabelPart(t *testing.T) {
	for _, tt := range []struct {
		name    string
		chapter Chapter
		want    string
	}{
		{"numeric part", Chapter{Number: 364, Label: Label{Style: LabelNumbered, Part: "2"}}, "Chapter 364 Part 2"},
		{"numeric part with title", Chapter{Title: "Foo", Number: 364, Label: Label{Style: LabelNumbered, Part: "2"}}, "Part 2 Foo"},
		{"word part", Chapter{Number: 110, Label: Label{Style: LabelNumbered, Part: "前編"}}, "Chapter 110 前編"},
		{"word part with title", Chapter{Title: "ヤる時", Number: 110, Label: Label{Style: LabelNumbered, Part: "前編"}}, "前編 ヤる時"},
	} {
		if got := ChapterLabel(tt.chapter); got != tt.want {
			t.Errorf("%s: ChapterLabel = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// chapterWordRe matches a chapter or part word as titles write it
var chapterWordRe = regexp.MustCompile(`\b(Chapter|Chapitre|Capítulo|Bölüm|Episode|Ch\.|Part)( |["` + "`" + `]$)`)

// TestNoChapterWordOutsideLabels keeps grabbers from writing a chapter word of
// their own into titles: outside label.go, a string literal holding one must
// be a pattern or a message, listed here with its reason
func TestNoChapterWordOutsideLabels(t *testing.T) {
	allowed := map[string]string{
		`", Chapter "`: "chainsawmanonline parses the site's aria-label with it",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "label.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok && isMessageOrPattern(call) {
				// errors, warnings and regexps may name chapters
				return false
			}
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || !chapterWordRe.MatchString(lit.Value) {
				return true
			}
			if _, ok := allowed[lit.Value]; !ok {
				t.Errorf("%s: %s holds a chapter word; titles leave it to ChapterLabel", fset.Position(lit.Pos()), lit.Value)
			}
			return true
		})
	}
}

// isMessageOrPattern tells calls whose string arguments are errors, warnings,
// selectors or patterns rather than titles
func isMessageOrPattern(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "Find", "Filter", "Is", "Not", "Has", "Contains", "Closest", "ChildrenFiltered", "ParentsFiltered", "NextFiltered", "PrevFiltered", "SiblingsFiltered":
		// goquery selectors, whatever the receiver
		return true
	}
	pkg, _ := sel.X.(*ast.Ident)
	if pkg == nil {
		return false
	}
	switch pkg.Name {
	case "fmt":
		return sel.Sel.Name == "Errorf"
	case "errors", "regexp", "notice":
		return true
	}
	return false
}
