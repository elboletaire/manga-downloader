// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Grabber is the base struct for all grabbers/sites
type Grabber struct {
	// URL is the manga index URL
	URL string
	// Settings are the grabber settings
	Settings *Settings
}

// Settings are grabber settings
type Settings struct {
	// Bundle is a flag to indicate if the chapters should be bundled into a single file
	Bundle bool
	// MaxConcurrency determines max download concurrency
	MaxConcurrency MaxConcurrency
	// Language is the preferred language for downloading chapters
	Language string
	// Scanlator keeps only the releases of this scanlation group, for sites
	// that host several groups' versions of the same chapters. Empty or "all"
	// keeps every group's releases
	Scanlator string
	// FilenameTemplate is the template for the filename
	FilenameTemplate string
	// Format is the desired output format ("cbz", "raw" or "epub")
	Format string
	// ReadingDirection is the way an EPUB's pages turn: "rtl" (manga) or "ltr".
	// Only EPUB output reads it
	ReadingDirection string
	// ConvertImages is the comma separated list of source image formats to
	// transcode to JPEG when packing ("avif", "avif,webp" or "none"), so pages
	// served in formats e-readers can't display end up readable
	ConvertImages string
	// Range is the range to be downloaded (in string, i.e. "1-10,23,45-50")
	Range string
	// Volumes packs one file per volume instead of one per chapter: either
	// "auto" (the volumes the site files chapters under) or chapter ranges
	// separated by slashes, one group per volume ("1-7/8-16"). Empty packs
	// per chapter, and it can't be combined with Bundle.
	Volumes string
	// SkipPages is the range of pages to exclude from every downloaded
	// chapter (i.e. "2,-1"), in the same syntax as Range but additionally
	// accepting negative positions, which count from the end of each
	// chapter's own page list
	SkipPages string
	// OutputDir is the output directory for the downloaded files
	OutputDir string
	// BrowserVisible shows the browser window for sites that need one, so
	// interactive challenges (e.g. cloudflare) can be solved manually
	BrowserVisible bool
	// Retry is the number of retries for failed page downloads
	Retry uint8
}

// Normalize caps the settings at the limits the CLI advertises: 5 concurrent
// chapters, 10 concurrent pages and 3 retries. Values above a limit are
// clamped rather than rejected.
func (s *Settings) Normalize() {
	if s.MaxConcurrency.Chapters > 5 {
		s.MaxConcurrency.Chapters = 5
	}
	if s.MaxConcurrency.Pages > 10 {
		s.MaxConcurrency.Pages = 10
	}
	if s.Retry > 3 {
		s.Retry = 3
	}
}

// MaxConcurrency is the max concurrency for a site
type MaxConcurrency struct {
	// Chapters is the max concurrency for chapters
	Chapters uint8
	// Pages is the max concurrency for pages
	Pages uint8
}

// Site is the handler interface, base of all manga sites grabbers
type Site interface {
	// Test tests if the site is the one for the specified url
	Test() (bool, error)
	// FetchChapters fetches the chapters for the manga
	FetchChapters() (Filterables, []error)
	// FetchChapter fetches the specified chapter
	FetchChapter(Filterable) (*Chapter, error)
	// FetchTitle fetches the manga title
	FetchTitle() (string, error)
	// BaseUrl returns the base url of the site
	BaseUrl() string
	// GetFilenameTemplate returns the filename template
	GetFilenameTemplate() string
	// GetFormat returns the desired output format ("cbz", "raw" or "epub")
	GetFormat() string
	// GetReadingDirection returns the way an EPUB's pages turn ("rtl" or "ltr")
	GetReadingDirection() string
	// GetConvertImages returns the set of source image formats to transcode to
	// JPEG when packing
	GetConvertImages() ConvertFormats
	// GetMaxConcurrency returns the max concurrency for the site
	GetMaxConcurrency() MaxConcurrency
	// GetPreferredLanguage returns the preferred language for the site
	GetPreferredLanguage() string
	// GetPreferredScanlator returns the preferred scanlation group for the site
	GetPreferredScanlator() string
	// GetRetries returns the number of retries for failed page downloads
	GetRetries() uint8
}

// PagePacer is implemented by sites whose image servers limit the request
// rate, which a concurrency cap alone can't enforce. WaitPage blocks until
// the next page request may go out.
type PagePacer interface {
	WaitPage()
}

// earlySites are grabbers tried before every built-in one. Only the demo build
// registers any (see demo/), so they're empty in release builds.
var earlySites []func(*Grabber) Site

// RegisterEarly adds a grabber that IdentifySite tries ahead of the built-in ones
func RegisterEarly(newSite func(*Grabber) Site) {
	earlySites = append(earlySites, newSite)
}

// IdentifySite returns the site passing the Test() for the specified url
func (g *Grabber) IdentifySite() (Site, []error) {
	// PlainHTML is pre-created so IdentifySite can call its chapter-URL
	// resolution as a last resort after every other grabber has declined,
	// reusing the already-fetched doc and avoiding extra requests for URLs
	// that a later, domain-matched grabber (tcb, jestful, …) would serve.
	plainHTML := NewPlainHTML(g)
	sites := []Site{
		// Sites matching by domain/URL (no fetch) go before PlainHTML: their
		// Test() is precise and free, and it keeps PlainHTML's generic
		// selectors from shadowing a page that happens to share markup
		// (e.g. mangahere's #chapterlist wrapper vs the themesia theme's).
		NewPlainHTMLBrowser(g),
		NewBaozimh(g),
		NewWeebCentral(g),
		NewLeerCapitulo(g),
		NewMangak(g),
		NewManganelo(g),
		NewMgeko(g),
		NewMangapark(g),
		NewGuya(g),
		NewAtsumaru(g),
		NewGlobalcomix(g),
		NewVortexscans(g),
		NewAzorafly(g),
		NewBigsolo(g),
		NewDrakecomic(g),
		NewElftoon(g),
		NewFmteam(g),
		NewGenzToon(g),
		NewHivetoons(g),
		NewKaynscan(g),
		NewHijala(g),
		NewMangaball(g),
		NewMangataro(g),
		NewMangitto(g),
		NewMangalib(g),
		NewMangadenizi(g),
		NewRoliascan(g),
		NewSacachispa(g),
		NewTeamshadowi(g),
		NewTaiyo(g),
		NewStonescape(g),
		NewUtoon(g),
		NewComix(g),
		NewMkissa(g),
		NewWitchtoons(g),
		NewWebtoons(g),
		NewMangahere(g),
		NewMangatown(g),
		NewMangaplus(g),
		NewChainsawManOnline(g),
		NewGigaviewer(g),
		NewHeavenlyDelusion(g),
		NewToivoLite(g),
		NewInmanga(g),
		NewMangadex(g),
		NewMangafire(g),
		NewFanfox(g),
		NewFlamecomics(g),
		NewQimanga(g),
		NewBluesolo(g),
		NewLuascans(g),
		NewProjectsuki(g),
		NewHeavenManga(g),
		NewLelScans(g),
		plainHTML,
		NewTcb(g),
		NewJestful(g),
	}
	var early []Site
	for _, newSite := range earlySites {
		early = append(early, newSite(g))
	}
	sites = append(early, sites...)
	var errs []error

	for _, s := range sites {
		ok, err := s.Test()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ok {
			return s, errs
		}
	}

	// Last resort: if PlainHTML fetched the page but found no rows (e.g. a
	// chapter URL whose series page a later grabber would serve), try its
	// chapter-URL resolution now that every grabber has declined.
	if plainHTML.doc != nil && plainHTML.resolveSeriesURL() {
		return plainHTML, errs
	}

	// Matched by markup, so they run after the fallback: themesia chapter pages
	// are WordPress posts too, and the fallback resolves those to their series.
	// Their errors surface only when nothing matches.
	var markupErrs []error
	for _, s := range []Site{NewWordPressCategory(g), NewBlogger(g)} {
		ok, err := s.Test()
		if err != nil {
			markupErrs = append(markupErrs, err)
			continue
		}
		// a match makes the errors of the grabbers that declined noise
		if ok {
			return s, nil
		}
	}

	return nil, append(errs, markupErrs...)
}

// pageLanguage returns the primary subtag of the page's <html lang>, lowercased
// ("es-ES" is "es"), or "" when the page doesn't say.
func pageLanguage(doc *goquery.Document) string {
	lang := strings.ToLower(strings.TrimSpace(doc.Find("html").AttrOr("lang", "")))
	primary, _, _ := strings.Cut(lang, "-")
	return primary
}

// BaseUrl returns the base url of the site
func (g Grabber) BaseUrl() string {
	u, _ := url.Parse(g.URL)
	return u.Scheme + "://" + u.Host
}

// retryFlakyHosts are the sites whose origin fails pages at random with a
// 404 or 5xx (see Page.RetryFlaky).
var retryFlakyHosts = []string{"mangazin.org"}

// retryFlaky reports whether the site's pages opt in to Page.RetryFlaky
func (g *Grabber) retryFlaky() bool {
	if g == nil {
		return false
	}
	u, _ := url.Parse(g.URL)
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return slices.Contains(retryFlakyHosts, host)
}

// GetPreferredLanguage returns the preferred language for the site
func (g Grabber) GetPreferredLanguage() string {
	return g.Settings.Language
}

// GetPreferredScanlator returns the preferred scanlation group for the site
func (g Grabber) GetPreferredScanlator() string {
	return g.Settings.Scanlator
}

// GetMaxConcurrency returns the max concurrency for the site
func (g Grabber) GetMaxConcurrency() MaxConcurrency {
	return g.Settings.MaxConcurrency
}

// SetMaxConcurrency sets the max concurrency for the site
func (g *Grabber) SetMaxConcurrency(m MaxConcurrency) {
	g.Settings.MaxConcurrency = m
}

// SetSettings updates the settings pointer. The controller calls this when
// reusing a site across filter-only reloads so the site and the new settings
// share the same pointer, keeping later Configure changes visible.
func (g *Grabber) SetSettings(s *Settings) { g.Settings = s }

// GetFilenameTemplate returns the defined filename template
func (g Grabber) GetFilenameTemplate() string {
	return g.Settings.FilenameTemplate
}

// GetRetries returns the number of retries for failed page downloads
func (g Grabber) GetRetries() uint8 {
	return g.Settings.Retry
}

// GetFormat returns the desired output format ("cbz", "raw" or "epub")
func (g Grabber) GetFormat() string {
	return g.Settings.Format
}

// GetReadingDirection returns the way an EPUB's pages turn ("rtl" or "ltr")
func (g Grabber) GetReadingDirection() string {
	return g.Settings.ReadingDirection
}

// GetConvertImages returns the set of source image formats to transcode to
// JPEG when packing. The value is validated at startup, so a parse error here
// can only mean it never went through the command flags; an empty set simply
// converts nothing.
func (g Grabber) GetConvertImages() ConvertFormats {
	formats, _ := ParseConvertFormats(g.Settings.ConvertImages)
	return formats
}

// NewSite returns a new site based on the passed url
func NewSite(url string, settings *Settings) (Site, []error) {
	if !strings.HasPrefix(url, "http") {
		return nil, []error{errors.New("invalid url")}
	}

	g := &Grabber{
		url,
		settings,
	}

	return g.IdentifySite()
}

// chapterSlugNumRe extracts the chapter number from a URL slug ending in
// "-chapter-N" or "-chapter-N-D" (where N is the whole part and D the decimal),
// e.g. "...chapter-63-2" → 63.2.
var chapterSlugNumRe = regexp.MustCompile(`-chapter-(\d+)(?:-(\d+))?/?$`)

// parseChapterSlugNum returns the chapter number encoded in a URL slug, and
// its string form as written in the slug ("13.5"), or ok=false if the URL
// carries no chapter number.
func parseChapterSlugNum(url string) (num float64, numStr string, ok bool) {
	m := chapterSlugNumRe.FindStringSubmatch(url)
	if m == nil {
		return 0, "", false
	}
	numStr = m[1]
	if m[2] != "" {
		numStr += "." + m[2]
	}
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, "", false
	}
	return num, numStr, true
}

// hostMatchesDomain reports whether rawURL's host is exactly domain or a
// subdomain of it (e.g. w1.example.com matches example.com). The comparison
// is case-insensitive and only inspects the host component of the URL, so a
// domain name that appears only in the path or query string does not match.
func hostMatchesDomain(rawURL, domain string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	d := strings.ToLower(domain)
	return host == d || strings.HasSuffix(host, "."+d)
}

// getUuid returns the first uuid found in the passed string
func getUuid(s string) string {
	re := regexp.MustCompile(`([\w\d]{8}(:?-[\w\d]{4}){3}-[\w\d]{12})`)
	return re.FindString(s)
}
