package grabber

import (
	"net/url"
	"regexp"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Grabber is the base struct for all grabbers/sites
type Grabber struct {
	URL               string
	MaxConcurrency    MaxConcurrency
	PreferredLanguage string
	FilenameTemplate  string
}

type MaxConcurrency struct {
	Chapters uint8
	Pages    uint8
}

// Site is the handler interface, base of all manga sites grabbers
type Site interface {
	InitFlags(cmd *cobra.Command)
	Test() bool
	FetchChapters() Filterables
	FetchChapter(Filterable) Chapter
	GetBaseUrl() string
	GetFilenameTemplate() string
	GetMaxConcurrency() MaxConcurrency
	GetTitle() string
	GetPreferredLanguage() string
}

// IdentifySite returns the site passing the Test() for the specified url
func (g *Grabber) IdentifySite() Site {
	sites := []Site{
		&InManga{Grabber: *g},
		&MangaDex{Grabber: *g},
		&Tcb{Grabber: *g},
		&Manganelo{Grabber: *g},
	}

	for _, s := range sites {
		if s.Test() {
			return s
		}
	}

	return nil
}

// GetBaseUrl returns the base url of the site
func (g Grabber) GetBaseUrl() string {
	u, _ := url.Parse(g.URL)
	return u.Scheme + "://" + u.Host
}

// GetPreferredLanguage returns the preferred language for the site
func (g Grabber) GetPreferredLanguage() string {
	return g.PreferredLanguage
}

// GetMaxConcurrency returns the max concurrency for the site
func (g Grabber) GetMaxConcurrency() MaxConcurrency {
	return g.MaxConcurrency
}

// SetMaxConcurrency sets the max concurrency for the site
func (g *Grabber) SetMaxConcurrency(m MaxConcurrency) {
	g.MaxConcurrency = m
}

// GetFilenameTemplate returns the defined filename template
func (g Grabber) GetFilenameTemplate() string {
	return g.FilenameTemplate
}

// InitFlags initializes the command flags
func (g *Grabber) InitFlags(cmd *cobra.Command) {
	g.SetMaxConcurrency(MaxConcurrency{
		Chapters: maxUint8Flag(cmd.Flag("concurrency"), 5),
		Pages:    maxUint8Flag(cmd.Flag("concurrency-pages"), 10),
	})
	g.PreferredLanguage = cmd.Flag("language").Value.String()
	g.FilenameTemplate = cmd.Flag("filename-template").Value.String()
}

// NewSite returns a new site based on the passed url
func NewSite(url string) Site {
	g := &Grabber{
		url,
		MaxConcurrency{},
		"",
		"",
	}

	return g.IdentifySite()
}

// GetUUID returns the first uuid found in the passed string
func GetUUID(s string) string {
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
