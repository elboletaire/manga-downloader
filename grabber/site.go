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
	// FilenameTemplate is the template for the filename
	FilenameTemplate string
	// Range is the range to be downloaded (in string, i.e. "1-10,23,45-50")
	Range string
	// OutputDir is the output directory for the downloaded files
	OutputDir string
	// ForceDownload forces chapter download even if it is found in local folder
	ForceDownload bool
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
	// GetMaxConcurrency returns the max concurrency for the site
	GetMaxConcurrency() MaxConcurrency
	// GetPreferredLanguage returns the preferred language for the site
	GetPreferredLanguage() string
}

// IdentifySite returns the site passing the Test() for the specified url
func (g *Grabber) IdentifySite() (Site, []error) {
	sites := []Site{
		&PlainHTML{Grabber: g},
		&Inmanga{Grabber: g},
		&Mangadex{Grabber: g},
		&Tcb{Grabber: g},
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

// InitFlags initializes the command flags
func (g *Grabber) InitFlags(cmd *cobra.Command) {
	g.SetMaxConcurrency(MaxConcurrency{
		Chapters: maxUint8Flag(cmd.Flag("concurrency"), 5),
		Pages:    maxUint8Flag(cmd.Flag("concurrency-pages"), 10),
	})
	g.Settings.Language = cmd.Flag("language").Value.String()
	g.Settings.FilenameTemplate = cmd.Flag("filename-template").Value.String()
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
