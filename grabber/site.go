package grabber

import (
	"net/url"
	"regexp"
)

// Grabber is the base struct for all grabbers/sites
type Grabber struct {
	URL string
}

// Site is the handler interface, base of all manga sites grabbers
type Site interface {
	Test() bool
	FetchChapters(string) Filterables
	FetchChapter(Filterable) Chapter
	GetBaseUrl() string
	GetTitle(string) string
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

// NewSite returns a new site based on the passed url
func NewSite(url string) Site {
	g := &Grabber{
		URL: url,
	}

	return g.IdentifySite()
}

// GetUUID returns the first uuid found in the passed string
func GetUUID(s string) string {
	re := regexp.MustCompile(`([\w\d]{8}(:?-[\w\d]{4}){3}-[\w\d]{12})`)
	return re.FindString(s)
}
