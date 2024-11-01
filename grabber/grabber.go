package grabber

import (
	"net/url"

	"github.com/spf13/cobra"
)

// Grabber is the base struct for all grabbers/sites
type Grabber struct {
	// URL is the manga index URL
	URL string
	// Settings are the grabber settings
	Settings *Settings
}

// IdentifySite returns the site passing the ValidateURL() for the specified url
func (g *Grabber) IdentifySite() (Site, []error) {
	sites := []Site{
		&PlainHTML{Grabber: g},
		&Inmanga{Grabber: g},
		&Mangadex{Grabber: g},
		&Tcb{Grabber: g},
	}
	var errs []error

	for _, s := range sites {
		ok, err := s.ValidateURL()
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

// BaseURL returns the base url of the site
func (g Grabber) BaseURL() string {
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
	// TODO
	g.SetMaxConcurrency(MaxConcurrency{
		Chapters: 5,  //maxUint8Flag(cmd.Flag("concurrency"), 5),
		Pages:    10, // maxUint8Flag(cmd.Flag("concurrency-pages"), 10),
	})
	g.Settings.Language = "en" // cmd.Flag("language").Value.String()
	g.Settings.FilenameTemplate = "{{.Series}} {{.Number}} - {{.Title}}"
}
