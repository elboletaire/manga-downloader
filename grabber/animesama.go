// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
)

// AnimeSama is a grabber for anime-sama.to. Each scan edition and language is its
// own page, whose chapter list scans.js builds from a page-count endpoint and an
// inline script; it's mirrored here without a browser.
type AnimeSama struct {
	*Grabber
	// edition is the edition the URL resolves to, cached per run
	edition *animeSamaEdition
}

// NewAnimeSama returns a new AnimeSama grabber
func NewAnimeSama(g *Grabber) *AnimeSama {
	return &AnimeSama{Grabber: g}
}

// AnimeSamaChapter is a chapter listed on an edition page
type AnimeSamaChapter struct {
	Chapter
	// Folder is the /s2/scans/{name}/{Folder}/ directory the pages are in
	Folder int
	// Count is the number of pages, from the page-count endpoint
	Count int
}

// animeSamaEdition is what an edition page gives: its name, languages and chapters
type animeSamaEdition struct {
	// URL is the edition page, with a trailing slash
	URL string
	// Name is the h1's innerHTML, the key the API and the image paths use. It's
	// never trimmed: "One Piece " has a trailing space.
	Name string
	// Title is Name as a title
	Title string
	// Language is the edition's language code ("vf", "va")
	Language string
	// Languages are the codes the page links to
	Languages []string
	// Labels are the chapter list's option texts, the index k being folder k+1
	Labels []string
	// Pages maps a folder to its page count
	Pages map[int]int
}

// animeSamaChapterWord is the word scans.js writes before every chapter number.
// It parses the site's text, so it's not the localized chapterWords.
const animeSamaChapterWord = "Chapitre "

// animeSamaLanguageAliases maps the --language aliases to the site's codes
var animeSamaLanguageAliases = map[string]string{"fr": "vf", "en": "va"}

// animeSamaNameRe reads the h1 that holds the edition's name
var animeSamaNameRe = regexp.MustCompile(`<h1[^>]*id="titreOeuvre"[^>]*>([^<]*)</h1>`)

// animeSamaPanelRe matches a series page's panneauScan("name", "section/lang")
// call. Each argument is one quoted string, either quote style: the matching
// alternative is the one that's not empty.
var animeSamaPanelRe = regexp.MustCompile(`panneauScan\s*\(\s*(?:"([^"]*)"|'([^']*)')\s*,\s*(?:"([^"]*)"|'([^']*)')\s*\)`)

// animeSamaCallRe matches a call of the chapter list script (resetListe,
// creerListe, newSP, newSPF, finirListe), its arguments kept whole
var animeSamaCallRe = regexp.MustCompile(`(?s)\b(resetListe|creerListe|newSPF|newSP|finirListe)\s*\(\s*((?:"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|[^()"'])*)\)`)

// animeSamaNumberRe matches the chapter numbers the labels give ("25.5")
var animeSamaNumberRe = regexp.MustCompile(`^\d+(?:\.\d+)?$`)

// animeSamaInnerHTMLEscaper writes a text the way innerHTML serializes it
var animeSamaInnerHTMLEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", " ", "&nbsp;")

// animeSamaArgUnescaper undoes the escapes in a quoted script argument
var animeSamaArgUnescaper = strings.NewReplacer(`\"`, `"`, `\'`, `'`, `\\`, `\`)

// animeSamaURLRe matches a catalogue URL on any of the site's mirrors
var animeSamaURLRe = regexp.MustCompile(`(?i)^https?://(www\.)?anime-sama\.(to|tv|si|org)/catalogue/[^/?#]+`)

// Test returns true if the URL is an anime-sama.to catalogue URL. It decides
// from the URL alone.
func (a *AnimeSama) Test() (bool, error) {
	return animeSamaURLRe.MatchString(a.URL), nil
}

// FetchTitle returns the edition's title
func (a *AnimeSama) FetchTitle() (string, error) {
	edition, err := a.resolveEdition()
	if err != nil {
		return "", err
	}

	return edition.Title, nil
}

// FetchChapters returns the chapters of the edition. A folder without pages on
// the site is skipped, as the site skips it.
func (a *AnimeSama) FetchChapters() (chapters Filterables, errs []error) {
	edition, err := a.resolveEdition()
	if err != nil {
		return nil, []error{err}
	}

	lang := ""
	if edition.Language == "vf" {
		lang = "fr"
	}
	for k, label := range edition.Labels {
		folder := k + 1
		count := edition.Pages[folder]
		if count == 0 {
			continue
		}

		chapter := Chapter{
			Language: edition.Language,
			ID:       strconv.Itoa(folder),
		}
		text := strings.TrimPrefix(label, animeSamaChapterWord)
		if animeSamaNumberRe.MatchString(text) {
			chapter.Number, _ = strconv.ParseFloat(text, 64)
			chapter.Label = Label{Style: LabelNumbered, Number: text, Lang: lang}
		} else {
			chapter.Title = text
			chapter.Label = Label{Style: LabelUnnumbered}
		}

		chapters = append(chapters, &AnimeSamaChapter{Chapter: chapter, Folder: folder, Count: count})
	}

	return chapters, nil
}

// FetchChapter returns a chapter with its pages, which are numbered from 1 in
// the folder
func (a *AnimeSama) FetchChapter(f Filterable) (*Chapter, error) {
	ac := f.(*AnimeSamaChapter)
	edition, err := a.resolveEdition()
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(ac.Count),
		Language:   ac.Language,
		ID:         ac.ID,
	}
	dir := a.BaseUrl() + "/s2/scans/" + url.PathEscape(edition.Name) + "/" + strconv.Itoa(ac.Folder) + "/"
	for i := 1; i <= ac.Count; i++ {
		chapter.Pages = append(chapter.Pages, Page{
			Number: int64(i),
			URL:    dir + strconv.Itoa(i) + ".jpg",
		})
	}

	return chapter, nil
}

// LanguageOptions lists the languages the edition links to. Each language is a
// separate edition, so the list is the one the URL points at when no --language
// is given.
func (a *AnimeSama) LanguageOptions() FilterOptions {
	if a.edition == nil {
		return FilterOptions{}
	}

	return FilterOptions{
		Values:  uniqueValues(a.edition.Languages),
		Current: a.edition.Language,
	}
}

// resolveEdition returns the edition the URL points at, fetching it once. A
// series or a section without a language resolves to one of the series' scan
// editions, and --language switches to that language's edition.
func (a *AnimeSama) resolveEdition() (*animeSamaEdition, error) {
	if a.edition != nil {
		return a.edition, nil
	}

	slug, section, lang, ok := animeSamaPath(a.URL)
	if !ok {
		return nil, fmt.Errorf("anime-sama: %s isn't a catalogue URL", a.URL)
	}
	if !strings.HasPrefix(section, "scan") || lang == "" {
		var err error
		if section, lang, err = a.pickEdition(slug, section); err != nil {
			return nil, err
		}
	}

	edition, err := a.fetchEdition(a.editionURL(slug, section, lang), lang)
	if err != nil {
		return nil, err
	}

	if want := animeSamaLanguage(a.GetPreferredLanguage()); want != "" && want != lang {
		if !slices.Contains(edition.Languages, want) {
			return nil, &UnavailableFilterError{Filter: "language", Value: a.GetPreferredLanguage(), Available: edition.Languages}
		}
		if edition, err = a.fetchEdition(a.editionURL(slug, section, want), want); err != nil {
			return nil, err
		}
	}

	a.edition = edition

	return edition, nil
}

// pickEdition reads the series page and returns the scan edition to download,
// as its section and language. The edition whose section matches is preferred,
// the first one otherwise; the others are named in a warning.
func (a *AnimeSama) pickEdition(slug, section string) (string, string, error) {
	seriesURL := a.BaseUrl() + "/catalogue/" + slug + "/"
	body, err := http.GetText(http.RequestParams{URL: seriesURL, Referer: a.BaseUrl() + "/"})
	if err != nil {
		return "", "", err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return "", "", err
	}

	var editions []string
	for _, m := range animeSamaPanelRe.FindAllStringSubmatch(inlineScripts(doc), -1) {
		// one alternative of each argument matched, the other is empty
		editions = append(editions, m[3]+m[4])
	}
	if len(editions) == 0 {
		return "", "", fmt.Errorf("anime-sama: %s lists no scans", seriesURL)
	}

	picked := editions[0]
	for _, e := range editions {
		if section != "" && strings.Split(e, "/")[0] == section {
			picked = e
			break
		}
	}
	if len(editions) > 1 {
		var others []string
		for _, e := range editions {
			if e != picked {
				others = append(others, a.BaseUrl()+"/catalogue/"+slug+"/"+strings.Trim(e, "/")+"/")
			}
		}
		notice.Warn("Anime-Sama lists other scan editions of this series, each one downloaded from its own URL: %s", strings.Join(others, ", "))
	}

	parts := strings.Split(strings.Trim(picked, "/"), "/")
	lang := ""
	if len(parts) > 1 {
		lang = parts[1]
	}

	return parts[0], lang, nil
}

// fetchEdition reads an edition page: its name, languages, and chapter list
// from the page-count endpoint and the page's inline script
func (a *AnimeSama) fetchEdition(pageURL, lang string) (*animeSamaEdition, error) {
	body, err := http.GetText(http.RequestParams{URL: pageURL, Referer: a.BaseUrl() + "/"})
	if err != nil {
		return nil, err
	}
	match := animeSamaNameRe.FindStringSubmatch(body)
	if match == nil {
		return nil, fmt.Errorf("anime-sama: %s isn't a scans page", pageURL)
	}
	name := animeSamaInnerHTMLEscaper.Replace(html.UnescapeString(match[1]))

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	var languages []string
	doc.Find(`a.ct-langue[id^="switch"]`).Each(func(_ int, s *goquery.Selection) {
		languages = append(languages, strings.ToLower(strings.TrimPrefix(s.AttrOr("id", ""), "switch")))
	})

	pages, err := a.fetchPageCounts(name)
	if err != nil {
		return nil, err
	}
	taille := 0
	for folder := range pages {
		taille = max(taille, folder)
	}

	return &animeSamaEdition{
		URL:       pageURL,
		Name:      name,
		Title:     sanitizeTitle(html.UnescapeString(name)),
		Language:  lang,
		Languages: languages,
		Labels:    animeSamaLabels(inlineScripts(doc), taille),
		Pages:     pages,
	}, nil
}

// fetchPageCounts reads the page count of every folder of an edition
func (a *AnimeSama) fetchPageCounts(name string) (map[int]int, error) {
	endpoint := a.BaseUrl() + "/s2/scans/get_nb_chap_et_img.php?oeuvre=" + strings.ReplaceAll(url.QueryEscape(name), "+", "%20")
	text, err := http.GetText(http.RequestParams{URL: endpoint, Referer: a.BaseUrl() + "/"})
	if err != nil {
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, err
	}
	if msg, ok := raw["error"]; ok {
		var reason string
		_ = json.Unmarshal(msg, &reason)
		return nil, fmt.Errorf("anime-sama: %s", reason)
	}

	pages := make(map[int]int, len(raw))
	for key, value := range raw {
		folder, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		var count int
		if err := json.Unmarshal(value, &count); err != nil {
			continue
		}
		pages[folder] = count
	}

	return pages, nil
}

// editionURL returns the page of a section and language
func (a *AnimeSama) editionURL(slug, section, lang string) string {
	u := a.BaseUrl() + "/catalogue/" + slug + "/" + section + "/"
	if lang != "" {
		u += lang + "/"
	}

	return u
}

// animeSamaPath splits a catalogue URL into its series slug, section and
// language. The language is lowercased; the rest are as written.
func animeSamaPath(rawURL string) (slug, section, lang string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || !strings.EqualFold(parts[0], "catalogue") || parts[1] == "" {
		return "", "", "", false
	}
	if len(parts) > 2 {
		section = parts[2]
	}
	if len(parts) > 3 {
		lang = strings.ToLower(parts[3])
	}

	return parts[1], section, lang, true
}

// animeSamaLanguage returns the site's code for a --language value: the aliases
// fr and en, and the codes as they are. "" or "all" keeps the URL's edition.
func animeSamaLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" || lang == FilterAll {
		return ""
	}
	if code, ok := animeSamaLanguageAliases[lang]; ok {
		return code
	}

	return lang
}

// animeSamaLabels reproduces the chapter list the page's script builds from its
// calls, with taille the largest folder. Without any call the list is one
// chapter per folder.
func animeSamaLabels(script string, taille int) []string {
	calls := animeSamaCallRe.FindAllStringSubmatch(script, -1)
	var labels []string
	if len(calls) == 0 {
		for i := 1; i <= taille; i++ {
			labels = append(labels, fmt.Sprintf("%s%d", animeSamaChapterWord, i))
		}
		return labels
	}

	// the site's counter of the specials, which are one folder each but not
	// one chapter number each
	specials := 0
	for _, call := range calls {
		args := call[2]
		switch call[1] {
		case "resetListe":
			labels = nil
		case "creerListe":
			bounds := strings.Split(args, ",")
			if len(bounds) != 2 {
				continue
			}
			from, errFrom := strconv.Atoi(strings.TrimSpace(bounds[0]))
			to, errTo := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if errFrom != nil || errTo != nil {
				continue
			}
			for i := from; i <= to; i++ {
				labels = append(labels, fmt.Sprintf("%s%d", animeSamaChapterWord, i))
			}
		case "newSP":
			labels = append(labels, animeSamaChapterWord+animeSamaArg(args))
			specials++
		case "newSPF":
			labels = append(labels, animeSamaArg(args))
			specials++
		case "finirListe":
			from, err := strconv.Atoi(strings.TrimSpace(args))
			if err != nil {
				continue
			}
			for i := from; i <= taille-specials; i++ {
				labels = append(labels, fmt.Sprintf("%s%d", animeSamaChapterWord, i))
			}
		}
	}

	return labels
}

// animeSamaArg returns a script argument's value: a quoted string unescaped, a
// number as it'd print, anything else as written
func animeSamaArg(arg string) string {
	arg = strings.TrimSpace(arg)
	if len(arg) >= 2 && (arg[0] == '"' || arg[0] == '\'') && arg[len(arg)-1] == arg[0] {
		return animeSamaArgUnescaper.Replace(arg[1 : len(arg)-1])
	}
	if f, err := strconv.ParseFloat(arg, 64); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}

	return arg
}

// inlineScripts joins the text of a page's inline scripts, each one with its
// comments stripped on its own, so a quote left open in one script can't hide
// the comments of the next
func inlineScripts(doc *goquery.Document) string {
	var b strings.Builder
	doc.Find("script:not([src])").Each(func(_ int, s *goquery.Selection) {
		b.WriteString(stripJSComments(s.Text()))
		b.WriteString("\n")
	})

	return b.String()
}

// stripJSComments removes the JavaScript comments from src, and the HTML comment
// lines the site leaves in its scripts. Strings are kept whole, so a "//" in a
// URL stays.
func stripJSComments(src string) string {
	var kept []string
	for _, line := range strings.Split(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "<!--") {
			kept = append(kept, line)
		}
	}
	src = strings.Join(kept, "\n")

	var b strings.Builder
	var quote byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case quote != 0:
			b.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				b.WriteByte(src[i])
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
			b.WriteByte(c)
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				i = len(src)
				continue
			}
			i += 2 + end + 1
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				i = len(src)
				continue
			}
			i += end - 1
		default:
			b.WriteByte(c)
		}
	}

	return b.String()
}
