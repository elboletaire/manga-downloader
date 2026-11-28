// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestAnimeSamaTest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://anime-sama.to/catalogue/chainsaw-man/", true},
		{"https://anime-sama.to/catalogue/chainsaw-man/scan_special/vf/", true},
		{"https://www.anime-sama.tv/catalogue/x/scan/va", true},
		{"https://anime-sama.to/", false},
		{"https://example.com/catalogue/x/", false},
		{"https://bigsolo.org/manga/x", false},
	}
	for _, tt := range tests {
		a := NewAnimeSama(&Grabber{URL: tt.url})
		if got, _ := a.Test(); got != tt.want {
			t.Errorf("Test(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestAnimeSamaPath(t *testing.T) {
	tests := []struct {
		url                 string
		slug, section, lang string
		ok                  bool
	}{
		{"https://anime-sama.to/catalogue/chainsaw-man/", "chainsaw-man", "", "", true},
		{"https://anime-sama.to/catalogue/chainsaw-man/scan/VF", "chainsaw-man", "scan", "vf", true},
		{"https://anime-sama.to/catalogue/chainsaw-man/scan_special/vf", "chainsaw-man", "scan_special", "vf", true},
		{"https://anime-sama.to/catalogue/chainsaw-man/scan", "chainsaw-man", "scan", "", true},
		{"https://anime-sama.to/catalogue/chainsaw-man/saison1/vostfr/", "chainsaw-man", "saison1", "vostfr", true},
		{"https://anime-sama.to/catalogue/", "", "", "", false},
	}
	for _, tt := range tests {
		slug, section, lang, ok := animeSamaPath(tt.url)
		if slug != tt.slug || section != tt.section || lang != tt.lang || ok != tt.ok {
			t.Errorf("animeSamaPath(%q) = %q, %q, %q, %v; want %q, %q, %q, %v",
				tt.url, slug, section, lang, ok, tt.slug, tt.section, tt.lang, tt.ok)
		}
	}
}

func TestStripJSComments(t *testing.T) {
	src := "/*\nresetListe(); toujours\n*/\n" +
		"// newSP(9);\n" +
		"<!-- creerListe(1,9); -->\n" +
		"a = \"http://a\"; // tail newSP(8)\n" +
		"b = 'it\\'s /* kept */';\n" +
		"finirListe(3);"
	got := stripJSComments(src)

	for _, gone := range []string{"resetListe", "newSP(9)", "creerListe", "tail", "newSP(8)"} {
		if strings.Contains(got, gone) {
			t.Errorf("stripJSComments() kept %q in %q", gone, got)
		}
	}
	for _, kept := range []string{`"http://a"`, `'it\'s /* kept */'`, "finirListe(3);"} {
		if !strings.Contains(got, kept) {
			t.Errorf("stripJSComments() dropped %q from %q", kept, got)
		}
	}
}

func TestAnimeSamaLabels(t *testing.T) {
	tests := []struct {
		name   string
		script string
		taille int
		want   map[int]string // index -> label
		count  int
	}{
		{
			name:   "specials",
			script: "$(document).ready(function(){ resetListe(); creerListe(1, 25);newSP(25.5); finirListe(26); });",
			taille: 98,
			want:   map[int]string{0: "Chapitre 1", 25: "Chapitre 25.5", 26: "Chapitre 26", 97: "Chapitre 97"},
			count:  98,
		},
		{
			name:   "one shot",
			script: `resetListe(); creerListe(1, 1045);newSP("One Shot"); finirListe(1046);`,
			taille: 1047,
			want:   map[int]string{1045: "Chapitre One Shot", 1046: "Chapitre 1046"},
			count:  1047,
		},
		{
			name:   "decimals and an epilogue",
			script: `resetListe(); newSP(0); creerListe(1, 262);newSP(262.5); creerListe(263, 271);newSP("Épilogue"); finirListe(272);`,
			taille: 276,
			want:   map[int]string{0: "Chapitre 0", 263: "Chapitre 262.5", 273: "Chapitre Épilogue", 275: "Chapitre 273"},
			count:  276,
		},
		{
			name:   "template only",
			script: "/*\nresetListe(); toujours au debut en premier\ncreerListe(debut, fin);newSP(special);\nfinirListe(debut de la fin);\n*/",
			taille: 3,
			want:   map[int]string{0: "Chapitre 1", 2: "Chapitre 3"},
			count:  3,
		},
		{
			name:   "commented calls and bad bounds",
			script: "/* creerListe(1, 3); */ creerListe(debut, fin); creerListe(1, 2);",
			taille: 5,
			want:   map[int]string{0: "Chapitre 1", 1: "Chapitre 2"},
			count:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := animeSamaLabels(stripJSComments(tt.script), tt.taille)
			if len(got) != tt.count {
				t.Fatalf("animeSamaLabels() got %d labels, want %d: %q", len(got), tt.count, got)
			}
			for i, label := range tt.want {
				if got[i] != label {
					t.Errorf("label %d = %q, want %q", i, got[i], label)
				}
			}
		})
	}
}

// animeSamaTestServer serves an edition's page and the page-count endpoint
// from maps keyed by path and by oeuvre
func animeSamaTestServer(t *testing.T, pages map[string]string, counts map[string]string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/s2/scans/get_nb_chap_et_img.php" {
			body, ok := counts[r.URL.Query().Get("oeuvre")]
			if !ok {
				body = `{"error":"not found"}`
			}
			w.Write([]byte(body))
			return
		}
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
}

// the page of a One Piece edition whose name has a trailing space
const animeSamaOnePieceTestPage = `<html><body>
<h1 id="titreOeuvre" class="ct-titre">One Piece </h1>
<a href="../vf" id="switchVF" class="ct-langue ct-langue-active">VF</a>
<a href="../va" id="switchVA" class="ct-langue">VA</a>
<script>$(document).ready(function(){
/*
resetListe(); toujours au debut en premier
*/
resetListe(); creerListe(1, 2);newSP("One Shot"); finirListe(3);
});</script>
</body></html>`

func TestAnimeSamaFetch(t *testing.T) {
	srv := animeSamaTestServer(t,
		map[string]string{"/catalogue/one-piece/scan_noir-et-blanc/vf/": animeSamaOnePieceTestPage},
		map[string]string{"One Piece ": `{"1":3,"2":2,"3":4,"5":1}`},
	)
	defer srv.Close()

	a := NewAnimeSama(&Grabber{URL: srv.URL + "/catalogue/one-piece/scan_noir-et-blanc/vf/", Settings: &Settings{}})

	title, err := a.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "One Piece" {
		t.Errorf("FetchTitle() = %q, want %q", title, "One Piece")
	}

	chapters, errs := a.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	// folder 4 has no pages on the site, so "Chapitre 3" is left out
	want := []struct {
		label      string
		number     float64
		unnumbered bool
		pages      int
	}{
		{"Chapitre 1", 1, false, 3},
		{"Chapitre 2", 2, false, 2},
		{"One Shot", 0, true, 4},
		{"Chapitre 4", 4, false, 1},
	}
	if len(chapters) != len(want) {
		t.Fatalf("FetchChapters() got %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i]
		if got := ChapterLabel(c); got != w.label {
			t.Errorf("chapter %d label = %q, want %q", i, got, w.label)
		}
		if c.GetNumber() != w.number {
			t.Errorf("chapter %d number = %v, want %v", i, c.GetNumber(), w.number)
		}
		if Unnumbered(c) != w.unnumbered {
			t.Errorf("chapter %d unnumbered = %v, want %v", i, Unnumbered(c), w.unnumbered)
		}
		if c.(*AnimeSamaChapter).Language != "vf" {
			t.Errorf("chapter %d language = %q, want vf", i, c.(*AnimeSamaChapter).Language)
		}
		if got := c.(*AnimeSamaChapter).Count; got != w.pages {
			t.Errorf("chapter %d pages = %d, want %d", i, got, w.pages)
		}
	}

	chapter, err := a.FetchChapter(chapters[3])
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if len(chapter.Pages) != 1 || chapter.Pages[0].URL != srv.URL+"/s2/scans/One%20Piece%20/5/1.jpg" {
		t.Errorf("FetchChapter() pages = %+v, want one page from folder 5", chapter.Pages)
	}

	if opts := a.LanguageOptions(); opts.Current != "vf" || strings.Join(opts.Values, ",") != "vf,va" {
		t.Errorf("LanguageOptions() = %+v, want current vf of vf, va", opts)
	}
}

// a series page resolves to its first scan edition, and names the others
func TestAnimeSamaSeriesPage(t *testing.T) {
	series := `<html><body><script>
/* the doc: panneauScan("nom", "scan/vf"); */
//panneauScan("Hidden", "scan_hidden/vf");
panneauScan("Scans", "scan/vf");
panneauScan("Digital Colored Comics", "scan_special/vf");
</script></body></html>`
	edition := `<html><body><h1 id="titreOeuvre">Chainsaw Man</h1>
<a href="../vf" id="switchVF" class="ct-langue ct-langue-active">VF</a>
<script>resetListe(); creerListe(1, 1);</script></body></html>`
	srv := animeSamaTestServer(t,
		map[string]string{
			"/catalogue/chainsaw-man/":         series,
			"/catalogue/chainsaw-man/scan/vf/": edition,
		},
		map[string]string{"Chainsaw Man": `{"1":1}`},
	)
	defer srv.Close()

	a := NewAnimeSama(&Grabber{URL: srv.URL + "/catalogue/chainsaw-man/", Settings: &Settings{}})
	if _, err := a.FetchChapters(); len(err) > 0 {
		t.Fatalf("FetchChapters() errors = %v", err)
	}
	if !strings.HasSuffix(a.edition.URL, "/catalogue/chainsaw-man/scan/vf/") {
		t.Errorf("edition URL = %q, want the scan edition", a.edition.URL)
	}
}

func TestAnimeSamaSeriesPageWithoutScans(t *testing.T) {
	srv := animeSamaTestServer(t, map[string]string{
		"/catalogue/chainsaw-man/": `<html><body><script>/* panneauScan("a", "scan/vf"); */</script></body></html>`,
	}, nil)
	defer srv.Close()

	a := NewAnimeSama(&Grabber{URL: srv.URL + "/catalogue/chainsaw-man/", Settings: &Settings{}})
	if _, errs := a.FetchChapters(); len(errs) != 1 {
		t.Fatalf("FetchChapters() errors = %v, want one error", errs)
	}
}

// --language picks the other language's edition, and fails for one the site
// doesn't have, naming the ones it does
func TestAnimeSamaLanguage(t *testing.T) {
	vfPage := `<html><body><h1 id="titreOeuvre">Chainsaw Man</h1>
<a href="../vf" id="switchVF" class="ct-langue ct-langue-active">VF</a>
<a href="../va" id="switchVA" class="ct-langue">VA</a>
<script>resetListe(); creerListe(1, 1);</script></body></html>`
	vaPage := `<html><body><h1 id="titreOeuvre">Chainsaw Man Anglais</h1>
<a href="../vf" id="switchVF" class="ct-langue">VF</a>
<a href="../va" id="switchVA" class="ct-langue ct-langue-active">VA</a>
<script>resetListe(); creerListe(1, 1);</script></body></html>`
	srv := animeSamaTestServer(t,
		map[string]string{
			"/catalogue/chainsaw-man/scan/vf/": vfPage,
			"/catalogue/chainsaw-man/scan/va/": vaPage,
		},
		map[string]string{
			"Chainsaw Man":         `{"1":1}`,
			"Chainsaw Man Anglais": `{"1":1}`,
		},
	)
	defer srv.Close()
	url := srv.URL + "/catalogue/chainsaw-man/scan/vf/"

	a := NewAnimeSama(&Grabber{URL: url, Settings: &Settings{Language: "en"}})
	title, err := a.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "Chainsaw Man Anglais" || a.edition.Language != "va" {
		t.Errorf("FetchTitle() = %q in %q, want the English edition", title, a.edition.Language)
	}

	a = NewAnimeSama(&Grabber{URL: url, Settings: &Settings{Language: "de"}})
	_, errs := a.FetchChapters()
	var unavailable *UnavailableFilterError
	if len(errs) != 1 || !errors.As(errs[0], &unavailable) {
		t.Fatalf("FetchChapters() errors = %v, want an UnavailableFilterError", errs)
	}
	if strings.Join(unavailable.Available, ",") != "vf,va" {
		t.Errorf("Available = %v, want vf, va", unavailable.Available)
	}
}

func TestAnimeSamaLanguageAliases(t *testing.T) {
	tests := map[string]string{"": "", "all": "", "FR": "vf", "en": "va", "vf": "vf", "es": "es"}
	for in, want := range tests {
		if got := animeSamaLanguage(in); got != want {
			t.Errorf("animeSamaLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

// a quote one script leaves open must not hide the comments of the next one:
// the commented resetListe() would wipe the list if it were read as a call
func TestAnimeSamaInlineScriptsStripPerScript(t *testing.T) {
	page := `<html><body><script>var re = /"/;</script>` +
		`<script>creerListe(1, 3); /* resetListe(); */ creerListe(1, 2);</script></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}

	if labels := animeSamaLabels(inlineScripts(doc), 5); len(labels) != 5 {
		t.Errorf("animeSamaLabels() got %d labels, want 5: %q", len(labels), labels)
	}
}
