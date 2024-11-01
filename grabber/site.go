package grabber

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

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
	ValidateURL() (bool, error)
	// FetchChapters fetches the chapters for the manga
	FetchChapters() (Filterables, error)
	// FetchChapter fetches the specified chapter
	FetchChapter(Filterable) (*Chapter, error)
	// FetchTitle fetches the manga title
	FetchTitle() (string, error)
	// BaseURL returns the base url of the site
	BaseURL() string
	// GetFilenameTemplate returns the filename template
	GetFilenameTemplate() string
	// GetMaxConcurrency returns the max concurrency for the site
	GetMaxConcurrency() MaxConcurrency
	// GetPreferredLanguage returns the preferred language for the site
	GetPreferredLanguage() string
}

// NewSite returns a new site based on the passed url
func NewSite(siteURL string, settings *Settings) (Site, []error) {
	if !strings.HasPrefix(siteURL, "http") {
		return nil, []error{errors.New("invalid url")}
	}

	g := &Grabber{
		siteURL,
		settings,
	}

	return g.IdentifySite()
}

// getUUID returns the first uuid found in the passed string
func getUUID(s string) (uuid.UUID, error) {
	re := regexp.MustCompile(`([\w\d]{8}(:?-[\w\d]{4}){3}-[\w\d]{12})`)
	return uuid.Parse(re.FindString(s))
}

// maxUint8Flag returns the max value between the flag uint8 value and the passed max
func maxUint8Flag(flag *pflag.Flag, max uint8) uint8 {
	v, _ := strconv.ParseUint(flag.Value.String(), 10, 8)
	if v > uint64(max) {
		return max
	}
	return uint8(v)
}
