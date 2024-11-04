package grabber

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// Settings are grabber settings
type Settings struct {
	Bundle           bool
	MaxConcurrency   MaxConcurrency
	Language         string
	FilenameTemplate string
	Range            string
	OutputDir        string
}

// MaxConcurrency is the max concurrency for a site
type MaxConcurrency struct {
	Chapters uint8
	Pages    uint8
}

// Site is the handler interface, base of all manga sites grabbers
type Site interface {
	InitFlags(cmd *cobra.Command)
	ValidateURL() (bool, error)
	FetchChapters() (Filterables, error)
	FetchChapter(Filterable) (*Chapter, error)
	FetchTitle() (string, error)
	BaseURL() string
	GetFilenameTemplate() string
	GetMaxConcurrency() MaxConcurrency
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
	re := regexp.MustCompile(`([a-z\d]{8}(:?-[a-z\d]{4}){3}-[a-z\d]{12})`)
	return uuid.Parse(re.FindString(s))
}
