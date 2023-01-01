package grabber

import (
	"net/url"
	"regexp"

	"github.com/elboletaire/manga-downloader/models"
)

type Grabber struct {
	URL string
}

func (g *Grabber) IdentifySite() models.Site {
	sites := []models.Site{
		&InManga{Grabber: *g},
		&MangaDex{Grabber: *g},
		&Manganelo{Grabber: *g},
	}

	for _, s := range sites {
		if s.Test() {
			return s
		}
	}

	return nil
}

func (g Grabber) GetBaseUrl() string {
	u, _ := url.Parse(g.URL)
	return u.Scheme + "://" + u.Host
}

func NewSite(url string) models.Site {
	g := &Grabber{
		URL: url,
	}

	return g.IdentifySite()
}

func GetUUID(s string) string {
	re := regexp.MustCompile(`([\w\d]{8}(:?-[\w\d]{4}){3}-[\w\d]{12})`)
	return re.FindString(s)
}
