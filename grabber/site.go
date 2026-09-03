// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	// Scanlator is the preferred scanlation group for sites that host several
	// groups' versions of the same chapters ("all" keeps every group's)
	Scanlator string
	// FilenameTemplate is the template for the filename
	FilenameTemplate string
	// Format is the desired output format ("cbz" or "raw")
	Format string
	// ConvertImages is the comma separated list of source image formats to
	// transcode to JPEG when packing ("avif", "avif,webp" or "none"), so pages
	// served in formats e-readers can't display end up readable
	ConvertImages string
	// Range is the range to be downloaded (in string, i.e. "1-10,23,45-50")
	Range string
	// OutputDir is the output directory for the downloaded files
	OutputDir string
	// BrowserVisible shows the browser window for sites that need one, so
	// interactive challenges (e.g. cloudflare) can be solved manually
	BrowserVisible bool
	// Retry is the number of retries for failed page downloads
	Retry uint8
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
	// InitFlags initializes the command flags
	InitFlags(cmd *cobra.Command)
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
	// GetFormat returns the desired output format ("cbz" or "raw")
	GetFormat() string
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

// IdentifySite returns the site passing the Test() for the specified url
func (g *Grabber) IdentifySite() (Site, []error) {
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
		NewVortexscans(g),
		NewBigsolo(g),
		NewDrakecomic(g),
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
		NewMangahere(g),
		NewPlainHTML(g),
		NewInmanga(g),
		NewMangadex(g),
		NewMangafire(g),
		NewFanfox(g),
		NewFlamecomics(g),
		NewQimanga(g),
		NewBluesolo(g),
		NewLuascans(g),
		NewProjectsuki(g),
		NewTcb(g),
		NewJestful(g),
	}
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

	return nil, errs
}

// BaseUrl returns the base url of the site
func (g Grabber) BaseUrl() string {
	u, _ := url.Parse(g.URL)
	return u.Scheme + "://" + u.Host
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

// GetFilenameTemplate returns the defined filename template
func (g Grabber) GetFilenameTemplate() string {
	return g.Settings.FilenameTemplate
}

// GetRetries returns the number of retries for failed page downloads
func (g Grabber) GetRetries() uint8 {
	return g.Settings.Retry
}

// GetFormat returns the desired output format ("cbz" or "raw")
func (g Grabber) GetFormat() string {
	return g.Settings.Format
}

// GetConvertImages returns the set of source image formats to transcode to
// JPEG when packing. The value is validated at startup, so a parse error here
// can only mean it never went through the command flags; an empty set simply
// converts nothing.
func (g Grabber) GetConvertImages() ConvertFormats {
	formats, _ := ParseConvertFormats(g.Settings.ConvertImages)
	return formats
}

// InitFlags initializes the command flags
func (g *Grabber) InitFlags(cmd *cobra.Command) {
	g.SetMaxConcurrency(MaxConcurrency{
		Chapters: maxUint8Flag(cmd.Flag("concurrency"), 5),
		Pages:    maxUint8Flag(cmd.Flag("concurrency-pages"), 10),
	})
	g.Settings.Language = cmd.Flag("language").Value.String()
	g.Settings.Scanlator = cmd.Flag("scanlator").Value.String()
	g.Settings.FilenameTemplate = cmd.Flag("filename-template").Value.String()
	g.Settings.Retry = maxUint8Flag(cmd.Flag("retry"), 3)
	g.Settings.Format = cmd.Flag("format").Value.String()
	g.Settings.ConvertImages = cmd.Flag("convert-images").Value.String()
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

// getUuid returns the first uuid found in the passed string
func getUuid(s string) string {
	re := regexp.MustCompile(`([\w\d]{8}(:?-[\w\d]{4}){3}-[\w\d]{12})`)
	return re.FindString(s)
}

// maxUint8Flag returns the max value between the flag uint8 value and the passed max
func maxUint8Flag(flag *pflag.Flag, max uint8) uint8 {
	v, _ := strconv.ParseUint(flag.Value.String(), 10, 8)
	if v > uint64(max) {
		return max
	}
	return uint8(v)
}
