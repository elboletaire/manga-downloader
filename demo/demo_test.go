//go:build demo

package demo

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
)

const (
	inmangaURL  = "https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935"
	horimiyaURL = "https://mangadex.org/title/a25e46ec-30f7-4db6-89df-cacbc1d9a900"
)

func loaded(t *testing.T) []*series {
	t.Helper()
	all, err := loadSeries(seriesFiles)
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// identify finds the site of url the way the CLI does, with the fake grabbers
// registered
func identify(t *testing.T, url string, settings *grabber.Settings) grabber.Site {
	t.Helper()
	for _, data := range loaded(t) {
		grabber.RegisterEarly(func(g *grabber.Grabber) grabber.Site { return newSite(g, data) })
	}
	site, _ := grabber.NewSite(url, settings)
	return site
}

func TestClaimsOnlyTheTapesURLs(t *testing.T) {
	all := loaded(t)
	if len(all) != 3 {
		t.Fatalf("got %d series, want the 3 the tapes use", len(all))
	}
	g := &grabber.Grabber{URL: "https://inmanga.com/ver/manga/Other/0000", Settings: &grabber.Settings{}}
	for _, data := range all {
		if ok, _ := newSite(g, data).Test(); ok {
			t.Errorf("%v claims a URL it has no data for", data.URLs)
		}
	}

	g.URL = inmangaURL + "/"
	claimed := 0
	for _, data := range all {
		if ok, _ := newSite(g, data).Test(); ok {
			claimed++
		}
	}
	if claimed != 1 {
		t.Errorf("%d series claim the inmanga URL, want 1", claimed)
	}
}

func TestServesTitleChaptersAndPages(t *testing.T) {
	site := identify(t, inmangaURL, &grabber.Settings{})
	if title, _ := site.FetchTitle(); title != "One Piece" {
		t.Errorf("title = %q", title)
	}
	chapters, errs := site.FetchChapters()
	if len(errs) > 0 || len(chapters) != 8 {
		t.Fatalf("got %d chapters, %v", len(chapters), errs)
	}
	chapter, err := site.FetchChapter(chapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if chapter.Number != 1140 || chapter.Title != "Capítulo 1140" || len(chapter.Pages) != 15 {
		t.Errorf("chapter = %v %q with %d pages", chapter.Number, chapter.Title, len(chapter.Pages))
	}
	if chapter.Pages[0].URL != pageURL(chapter.ID, 1) {
		t.Errorf("page URL = %q", chapter.Pages[0].URL)
	}
}

func TestFiltersByLanguageAndModelsVolumesAndGroups(t *testing.T) {
	site := identify(t, horimiyaURL, &grabber.Settings{Language: "en"})
	chapters, _ := site.FetchChapters()
	if len(chapters) != 13 {
		t.Fatalf("got %d chapters, want 13", len(chapters))
	}
	first := chapters[0].(*grabber.Chapter)
	if first.Volume != 1 || first.Group != "Village Idiot Scanlations" || first.Language != "en" {
		t.Errorf("first = volume %d, group %q, language %q", first.Volume, first.Group, first.Language)
	}
	if _, ok := site.(grabber.LanguageLister); !ok {
		t.Error("a series with languages must list them")
	}

	site = identify(t, horimiyaURL, &grabber.Settings{Language: "es-la"})
	if chapters, _ := site.FetchChapters(); len(chapters) != 0 {
		t.Errorf("got %d chapters for a language the data has none in", len(chapters))
	}

	if _, ok := identify(t, inmangaURL, &grabber.Settings{}).(grabber.LanguageLister); ok {
		t.Error("a single-language series must not list languages")
	}
}

func TestPageDelayIsStableAndVaried(t *testing.T) {
	seen := make(map[time.Duration]bool)
	for n := 1; n <= 5; n++ {
		u := pageURL("chapter", n)
		d := pageDelay(u)
		if d != pageDelay(u) {
			t.Errorf("pageDelay(%q) changed between calls", u)
		}
		if d < minPageDelay || d >= maxPageDelay {
			t.Errorf("pageDelay(%q) = %v, want within [%v, %v)", u, d, minPageDelay, maxPageDelay)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Errorf("pageDelay gives %d distinct delays for 5 pages, want them to vary", len(seen))
	}
}

func TestParseDelayScale(t *testing.T) {
	scale, err := parseDelayScale("")
	if err != nil || scale != 1 {
		t.Errorf("unset: got %v, %v, want 1", scale, err)
	}
	scale, err = parseDelayScale("0.4")
	if err != nil || scale != 0.4 {
		t.Errorf("0.4: got %v, %v, want 0.4", scale, err)
	}
	for _, raw := range []string{"abc", "0", "-1", "NaN", "Inf"} {
		if _, err := parseDelayScale(raw); err == nil || !strings.Contains(err.Error(), delayEnv) {
			t.Errorf("%q: got %v, want an error naming %s", raw, err, delayEnv)
		}
	}
}

func TestPagesAreScaledPlaceholdersAndOtherHostsFail(t *testing.T) {
	http.SetRoundTripper(pages{scale: 0.01})
	defer http.SetRoundTripper(nil)

	page := pageURL("chapter", 1)
	want := time.Duration(float64(pageDelay(page)) * 0.01)
	start := time.Now()
	body, err := http.Get(http.RequestParams{URL: page, DisableRetry: true})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if elapsed := time.Since(start); elapsed < want || elapsed >= minPageDelay {
		t.Errorf("page answered after %v, want at least %v and under %v", elapsed, want, minPageDelay)
	}
	if !bytes.HasPrefix(data, []byte{0xff, 0xd8, 0xff}) {
		t.Errorf("page is not a JPEG: %d bytes", len(data))
	}

	const other = "https://inmanga.com/anything"
	_, err = http.GetText(http.RequestParams{URL: other, DisableRetry: true})
	if err == nil || !strings.Contains(err.Error(), other) {
		t.Errorf("other host: got %v, want an error naming it", err)
	}
}
