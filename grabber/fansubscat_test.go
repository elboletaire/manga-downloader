// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fansubsCatSeriesPath = "/jojo-s-bizarre-adventure-part-1-phantom-blood/jojo-s-en-catala"

// fansubsCatSeriesPage returns the trimmed series page, with its catalogue type replaced by catalogueType
func fansubsCatSeriesPage(t *testing.T, catalogueType string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(identifyFixtureDir, "fansubscat_series.html"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(string(body), `value="manga"`, `value="`+catalogueType+`"`, 1)
}

// fansubsCatServer serves page at the series path, and the reader's answer to
// get_file_data.php for each file ID it's given, as the site's own endpoint does
func fansubsCatServer(t *testing.T, page string, files map[string]string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fansubsCatSeriesPath {
			_, _ = w.Write([]byte(page))
			return
		}
		if r.URL.Path == "/get_file_data.php" && r.Method == http.MethodPost {
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				http.Error(w, "expected an AJAX request", http.StatusBadRequest)
				return
			}
			if body, ok := files[r.FormValue("file_id")]; ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestFansubsCatTest(t *testing.T) {
	cases := []struct {
		name   string
		page   string
		target string
		want   bool
	}{
		{"manga series", fansubsCatSeriesPage(t, "manga"), fansubsCatSeriesPath, true},
		{"other catalogue", fansubsCatSeriesPage(t, "anime"), fansubsCatSeriesPath, false},
		{"other site", "<html><body>not a catalogue</body></html>", "/manga/one", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(c.page))
			}))
			defer srv.Close()

			got, err := NewFansubsCat(&Grabber{URL: srv.URL + c.target}).Test()
			if err != nil {
				t.Fatalf("Test() error = %v", err)
			}
			if got != c.want {
				t.Errorf("Test() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestFansubsCatResolveSeriesURL(t *testing.T) {
	cases := map[string]string{
		"https://manga.fansubs.cat/jojo-s-bizarre-adventure-part-1-phantom-blood/jojo-s-en-catala":         "https://manga.fansubs.cat/jojo-s-bizarre-adventure-part-1-phantom-blood/jojo-s-en-catala",
		"https://manga.fansubs.cat/jojo-s-bizarre-adventure-part-1-phantom-blood/jojo-s-en-catala?f=20089": "https://manga.fansubs.cat/jojo-s-bizarre-adventure-part-1-phantom-blood/jojo-s-en-catala",
		"https://manga.fansubs.cat/atzar#top": "https://manga.fansubs.cat/atzar",
	}
	for in, want := range cases {
		f := &FansubsCat{Grabber: &Grabber{URL: in}}
		if err := f.resolveSeriesURL(); err != nil {
			t.Fatalf("resolveSeriesURL(%q) error = %v", in, err)
		}
		if f.URL != want {
			t.Errorf("resolveSeriesURL(%q) = %q, want %q", in, f.URL, want)
		}
	}
}

func TestFansubsCatFetchTitleAndChapters(t *testing.T) {
	page := fansubsCatSeriesPage(t, "manga")
	srv := fansubsCatServer(t, page, nil)

	// a reader link lists the whole series
	f := NewFansubsCat(&Grabber{URL: srv.URL + fansubsCatSeriesPath + "?f=20089"})
	title, err := f.FetchTitle()
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "Jojo's Bizarre Adventure: Part 1 - Phantom Blood" {
		t.Errorf("FetchTitle() = %q", title)
	}

	chapters, errs := f.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("FetchChapters() = %d chapters, want 3", len(chapters))
	}

	want := []struct {
		id     string
		number float64
		volume int
		title  string
	}{
		{"20089", 1, 1, "Pròleg"},
		{"20239", 2, 1, "Dio Brando, l'invasor - Part 1"},
		{"20827", 15, 2, "La joventut amb en Dio - Part 4"},
	}
	for i, w := range want {
		c := chapters[i].(*FansubsCatChapter)
		if c.ID != w.id || c.GetNumber() != w.number || c.GetVolume() != w.volume || c.GetTitle() != w.title {
			t.Errorf("chapter %d = (%q, %v, %d, %q), want (%q, %v, %d, %q)", i,
				c.ID, c.GetNumber(), c.GetVolume(), c.GetTitle(), w.id, w.number, w.volume, w.title)
		}
		if c.Group != "JoJo's en Català" || c.Language != "ca" || c.Label.Style != LabelNumbered {
			t.Errorf("chapter %d group %q, language %q, label %v", i, c.Group, c.Language, c.Label.Style)
		}
	}
}

func TestFansubsCatFetchChaptersUnnumbered(t *testing.T) {
	// a one-shot row names only its title, and sits in a division with no volume
	page := `<div class="version-tab" data-version-id="1"><div class="version-fansub-icons"><div class="fansub"><img title="Lluna Plena no Fansub"></div></div></div>
<div id="version-content-1"><div class="division"><div class="division-title">One-shot</div>
<div class="file-launcher episode" data-file-id="13502"><div class="episode-title"><b>La història dels herois</b></div></div>
</div></div>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	chapters, errs := NewFansubsCat(&Grabber{URL: srv.URL + "/one-shot"}).FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	if len(chapters) != 1 {
		t.Fatalf("FetchChapters() = %d chapters, want 1", len(chapters))
	}
	c := chapters[0].(*FansubsCatChapter)
	if c.GetNumber() != 0 || c.GetTitle() != "La història dels herois" || c.Label.Style != LabelUnnumbered || c.GetVolume() != 0 {
		t.Errorf("one-shot = number %v, title %q, label %v, volume %d", c.GetNumber(), c.GetTitle(), c.Label.Style, c.GetVolume())
	}
	if c.Group != "Lluna Plena no Fansub" {
		t.Errorf("one-shot group = %q", c.Group)
	}
}

func TestFansubsCatFetchChapter(t *testing.T) {
	files := map[string]string{
		"20089": `{"result":"ok","data":{"file_id":20089,"pages":["https:\/\/cdn.example\/Manga\/20089\/pb_1x01.jpg","https:\/\/cdn.example\/Manga\/20089\/pb_1x02.jpg"]}}`,
		"404":   `{"result":"error"}`,
	}
	srv := fansubsCatServer(t, fansubsCatSeriesPage(t, "manga"), files)
	f := NewFansubsCat(&Grabber{URL: srv.URL + fansubsCatSeriesPath})

	listed := &FansubsCatChapter{Chapter{ID: "20089", Number: 1, Title: "Pròleg", Language: "ca", Label: Label{Style: LabelNumbered}}}
	listed.SetGroups("JoJo's en Català")
	chapter, err := f.FetchChapter(listed)
	if err != nil {
		t.Fatalf("FetchChapter() error = %v", err)
	}
	if chapter.PagesCount != 2 || len(chapter.Pages) != 2 {
		t.Fatalf("FetchChapter() pages = %d (count %d), want 2", len(chapter.Pages), chapter.PagesCount)
	}
	if chapter.Pages[1].URL != "https://cdn.example/Manga/20089/pb_1x02.jpg" || chapter.Pages[1].Number != 2 {
		t.Errorf("page 2 = %+v", chapter.Pages[1])
	}
	if chapter.ID != "20089" || chapter.Group != "JoJo's en Català" || chapter.Title != "Pròleg" {
		t.Errorf("FetchChapter() kept ID %q, group %q, title %q", chapter.ID, chapter.Group, chapter.Title)
	}

	if _, err := f.FetchChapter(&FansubsCatChapter{Chapter{ID: "404"}}); err == nil {
		t.Error("FetchChapter() without pages returned no error")
	}
}

func TestFansubsCatFetchChaptersRows(t *testing.T) {
	// two fansubs of one series, each with its own release of chapter 1; the
	// first row has a plain title, the decimal one a comma, and the last a
	// variant's <b> after its title
	page := `<div class="version-tab" data-version-id="1"><div class="version-fansub-icons"><div class="fansub"><img title="Enamorats Fansub"></div></div></div>
<div class="version-tab" data-version-id="2"><div class="version-fansub-icons"><div class="fansub"><img title="Altres Fansub"></div></div></div>
<div id="version-content-1"><div class="division"><div class="division-title">Volum 1</div>
<div class="file-launcher episode" data-file-id="101"><div class="episode-title"><b>Capítol 1</b><br>Enamorar-se de l'amor</div></div>
<div class="file-launcher episode" data-file-id="102"><div class="episode-title"><b>Capítol 0,5</b><br>#0,5 - Prefaci</div></div>
<div class="file-launcher episode" data-file-id="103"><div class="episode-title"><b>Capítol 3</b><br>Ombres<br><b>Variant B</b></div></div>
</div></div>
<div id="version-content-2"><div class="division"><div class="division-title">Volum 1</div>
<div class="file-launcher episode" data-file-id="201"><div class="episode-title"><b>Capítol 1</b><br>#001 - Enamorar-se de l'amor</div></div>
</div></div>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	chapters, errs := NewFansubsCat(&Grabber{URL: srv.URL + "/series"}).FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}
	want := []struct {
		id     string
		number float64
		title  string
		group  string
	}{
		{"101", 1, "Enamorar-se de l'amor", "Enamorats Fansub"},
		{"102", 0.5, "Prefaci", "Enamorats Fansub"},
		{"103", 3, "Ombres Variant B", "Enamorats Fansub"},
		{"201", 1, "Enamorar-se de l'amor", "Altres Fansub"},
	}
	if len(chapters) != len(want) {
		t.Fatalf("FetchChapters() = %d chapters, want %d", len(chapters), len(want))
	}
	for i, w := range want {
		c := chapters[i].(*FansubsCatChapter)
		if c.ID != w.id || c.GetNumber() != w.number || c.GetTitle() != w.title || c.Group != w.group {
			t.Errorf("chapter %d = (%q, %v, %q, %q), want (%q, %v, %q, %q)", i,
				c.ID, c.GetNumber(), c.GetTitle(), c.Group, w.id, w.number, w.title, w.group)
		}
		if c.Label.Style != LabelNumbered {
			t.Errorf("chapter %d label style = %v, want LabelNumbered", i, c.Label.Style)
		}
	}
}
