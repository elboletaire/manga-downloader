package grabber

import (
	"fmt"
	"os"
	"regexp"

	"github.com/elboletaire/manga-downloader/models"
)

func NewSite(url string) models.Site {
	i := InManga{
		URL: url,
	}
	if i.Test() {
		return i
	}
	m := MangaDex{
		URL: url,
	}
	if m.Test() {
		return m
	}

	fmt.Println("Site not recognised")
	os.Exit(1)

	return nil
}

func GetUUID(s string) string {
	re := regexp.MustCompile(`([\w\d]{8}(:?-[\w\d]{4}){3}-[\w\d]{12})`)
	return re.FindString(s)
}
