// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestGuyaSlug(t *testing.T) {
	cases := []struct {
		url     string
		want    string
		wantErr bool
	}{
		{"https://guya.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/", "Kaguya-Wants-To-Be-Confessed-To", false},
		{"https://guya.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/281/7/", "Kaguya-Wants-To-Be-Confessed-To", false},
		{"https://guya.cubari.moe/read/manga/Oshi-No-Ko/", "Oshi-No-Ko", false},
		{"https://danke.moe/read/manga/100-girlfriends/", "100-girlfriends", false},
		{"https://guya.moe/", "", true},
	}

	for _, c := range cases {
		g := Guya{Grabber: &Grabber{URL: c.url}}
		got, err := g.slug()
		if (err != nil) != c.wantErr {
			t.Errorf("slug(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("slug(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestGuyaTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://guya.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/", true},
		{"https://guya.cubari.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/", true},
		{"https://danke.moe/read/manga/100-girlfriends/", true},
		{"https://cubari.moe/read/gist/...", false},
		{"https://mangak.io/some-series", false},
	}

	for _, c := range cases {
		g := Guya{Grabber: &Grabber{URL: c.url}}
		got, _ := g.Test()
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestGuyaOrderedGroups(t *testing.T) {
	cases := []struct {
		name          string
		groups        map[string][]string
		preferredSort []string
		want          []string
	}{
		{
			name:          "preferred_sort order first",
			groups:        map[string][]string{"4": {"01.png"}, "3": {"01.png"}, "1": {"01.png"}},
			preferredSort: []string{"7", "3", "2", "1", "4"},
			want:          []string{"3", "1", "4"},
		},
		{
			name:          "unlisted groups after the preferred ones, by numeric id",
			groups:        map[string][]string{"10": {"01.png"}, "9": {"01.png"}, "3": {"01.png"}},
			preferredSort: []string{"3"},
			want:          []string{"3", "9", "10"},
		},
		{
			name:          "no preferred_sort (danke.moe)",
			groups:        map[string][]string{"70": {"01.png"}, "67": {"01.png"}, "74": {"01.png"}},
			preferredSort: []string{},
			want:          []string{"67", "70", "74"},
		},
		{
			name:          "no groups",
			groups:        map[string][]string{},
			preferredSort: []string{"7", "3"},
			want:          []string{},
		},
	}

	for _, c := range cases {
		got := orderedGroups(c.groups, c.preferredSort)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: orderedGroups() = %v, want %v", c.name, got, c.want)
		}
	}
}

// guyaServer serves a series feed with one chapter scanlated by two groups
// and one by a single group, as the guyamoe api lists them
func guyaServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/series/Series/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"slug":"Series","title":"Series",`+
			`"groups":{"1":"Alpha Scans","2":"Beta | Gamma"},`+
			`"preferred_sort":[],`+
			`"chapters":{`+
			`"1":{"title":"First","folder":"0001_abc","groups":{"2":["01.png"],"1":["01.png","02.png"]}},`+
			`"2":{"title":"","folder":"0002_def","groups":{"1":["01.png"]}}`+
			`}}`)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func TestGuyaFetchChaptersPerGroup(t *testing.T) {
	server := guyaServer(t)
	g := Guya{Grabber: &Grabber{URL: server.URL + "/read/manga/Series/", Settings: &Settings{}}}

	chapters, errs := g.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters() errors = %v", errs)
	}

	type release struct{ id, group, title string }
	want := map[float64][]release{
		1: {
			{"0001_abc/1", "Alpha Scans", "First"},
			{"0001_abc/2", "Beta | Gamma", "First"},
		},
		2: {
			{"0002_def/1", "Alpha Scans", "Chapter 2"},
		},
	}
	got := map[float64][]release{}
	for _, c := range chapters {
		gc := c.(*GuyaChapter)
		got[gc.Number] = append(got[gc.Number], release{gc.ID, gc.Group, ChapterLabel(gc)})
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FetchChapters() = %v, want %v", got, want)
	}

	// the pages of one release are that group's files only
	for _, c := range chapters {
		gc := c.(*GuyaChapter)
		if gc.ID != "0001_abc/1" {
			continue
		}
		chapter, err := g.FetchChapter(gc)
		if err != nil {
			t.Fatalf("FetchChapter() error = %v", err)
		}
		if chapter.ID != "0001_abc/1" || chapter.Group != "Alpha Scans" {
			t.Errorf("FetchChapter() ID %q, Group %q, want 0001_abc/1, Alpha Scans", chapter.ID, chapter.Group)
		}
		if len(chapter.Pages) != 2 || !strings.Contains(chapter.Pages[0].URL, "/0001_abc/1/01.png") {
			t.Errorf("FetchChapter() pages = %+v, want the group's two files", chapter.Pages)
		}
	}
}

func TestGuyaTestByAPI(t *testing.T) {
	server := guyaServer(t)

	cases := []struct {
		path string
		want bool
	}{
		{"/read/manga/Series/", true},
		{"/read/manga/Series/1/3/", true},
		{"/read/manga/Missing/", false},
		{"/manga/Series/", false},
	}
	for _, c := range cases {
		g := Guya{Grabber: &Grabber{URL: server.URL + c.path, Settings: &Settings{}}}
		got, err := g.Test()
		if err != nil || got != c.want {
			t.Errorf("Test(%q) = %v, %v, want %v", c.path, got, err, c.want)
		}
	}
}
