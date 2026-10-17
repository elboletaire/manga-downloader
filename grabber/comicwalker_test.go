// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// comicWalkerWorkFixture is the work JSON of a live series: episode 1 is open,
// the 3rd and 4th are split, the promotion has no number, and episode 2 is
// closed (isActive false). The web_trial copy repeats episode 1.
const comicWalkerWorkFixture = `{"work":{"title":"目覚めたら最強装備"},
"firstEpisodes":{"total":5,"result":[
 {"id":"e1","title":"第1話","type":"normal","serviceId":"web","isActive":true},
 {"id":"e2","title":"第2話","type":"normal","serviceId":"web","isActive":false},
 {"id":"e3","title":"第3話前編","type":"normal","serviceId":"web","isActive":true},
 {"id":"e4","title":"第3話後編","type":"normal","serviceId":"web","isActive":true},
 {"id":"e5","title":"1巻発売記念！","type":"pr","serviceId":"web","isActive":true}]},
"latestEpisodes":{"total":5,"result":[
 {"id":"e3","title":"第3話前編","type":"normal","serviceId":"web","isActive":true},
 {"id":"t1","title":"第１集　これは夢じゃない。現実だ。","type":"normal","serviceId":"web_trial","isActive":true}]}}`

// comicWalkerViewerFixture is a viewer response with one XOR page and one raw page
const comicWalkerViewerFixture = `{"manuscripts":[
 {"drmMode":"xor","drmHash":"daf3f44c4896df36","drmImageUrl":"https://cdn.example.com/1.webp"},
 {"drmMode":"raw","drmHash":"","drmImageUrl":"https://cdn.example.com/2.webp"}]}`

func TestComicWalkerTest(t *testing.T) {
	for url, want := range map[string]bool{
		"https://comic-walker.com/detail/KC_000061_S":                                true,
		"https://comic-walker.com/detail/KC_000061_S/episodes/KC_0000610000100011_E": true,
		"https://shonenjumpplus.com/detail/KC_000061_S":                              false,
	} {
		got, err := NewComicWalker(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || got != want {
			t.Errorf("Test(%s) = %v, %v; want %v", url, got, err, want)
		}
	}
}

func TestComicWalkerTitle(t *testing.T) {
	tests := []struct {
		raw    string
		title  string
		number float64
		part   string
	}{
		{"第1話", "", 1, ""},
		{"第２話", "", 2, ""},
		{"第3話前編", "", 3, "前編"},
		{"第3話後編", "", 3, "後編"},
		{"第１集　これは夢じゃない。現実だ。", "これは夢じゃない。現実だ。", 1, ""},
		{"第60話後編", "", 60, "後編"},
		{"第五十七話", "", 57, ""},
		{"1巻発売記念！", "1巻発売記念！", 0, ""},
	}
	for _, tt := range tests {
		title, number, part := comicWalkerTitle(tt.raw)
		if title != tt.title || number != tt.number || part != tt.part {
			t.Errorf("comicWalkerTitle(%q) = %q, %v, %q; want %q, %v, %q", tt.raw, title, number, part, tt.title, tt.number, tt.part)
		}
	}
}

func TestComicWalkerKey(t *testing.T) {
	key, err := comicWalkerKey("daf3f44c4896df36")
	if err != nil {
		t.Fatalf("comicWalkerKey error: %v", err)
	}
	if want := []byte{0xda, 0xf3, 0xf4, 0x4c, 0x48, 0x96, 0xdf, 0x36}; !bytes.Equal(key, want) {
		t.Errorf("key = % x, want % x", key, want)
	}
	if _, err := comicWalkerKey("daf3"); err == nil {
		t.Error("a short drmHash is accepted, want an error")
	}
	if _, err := comicWalkerKey("zzzz44c4896df36"); err == nil {
		t.Error("a non-hex drmHash is accepted, want an error")
	}
}

func TestComicWalkerXORRoundTrip(t *testing.T) {
	key, _ := comicWalkerKey("daf3f44c4896df36")
	plain := []byte("RIFF\x00\x00\x00\x00WEBPVP8 0123456789abcdef")
	scrambled := make([]byte, len(plain))
	for i, b := range plain {
		scrambled[i] = b ^ key[i%len(key)]
	}
	got, err := comicWalkerXOR(key)(scrambled)
	if err != nil || !bytes.Equal(got, plain) {
		t.Errorf("XOR decode = %q, %v; want %q", got, err, plain)
	}
}

func TestComicWalkerFetchChapters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/contents/details/work" && r.URL.Query().Get("workCode") == "KC_000061_S" {
			fmt.Fprint(w, comicWalkerWorkFixture)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	for _, url := range []string{srv.URL + "/detail/KC_000061_S", srv.URL + "/detail/KC_000061_S/episodes/e1"} {
		g := NewComicWalker(&Grabber{URL: url, Settings: &Settings{}})

		title, err := g.FetchTitle()
		if err != nil || title != "目覚めたら最強装備" {
			t.Fatalf("%s: FetchTitle() = %q, %v", url, title, err)
		}

		chapters, errs := g.FetchChapters()
		if len(errs) != 0 {
			t.Fatalf("%s: FetchChapters() errors %v", url, errs)
		}
		// episode 1, 2, 3前, 3後; the promotion, the web_trial copy and the repeated e3 are out
		wantIDs := []string{"e1", "e2", "e3", "e4"}
		wantLocked := []bool{false, true, false, false}
		if len(chapters) != len(wantIDs) {
			t.Fatalf("%s: FetchChapters() = %d chapters, want %d", url, len(chapters), len(wantIDs))
		}
		for i, f := range chapters {
			c := f.(*ComicWalkerChapter)
			if c.ID != wantIDs[i] || LockedOf(c) != wantLocked[i] {
				t.Errorf("%s: chapter %d = id %q locked %v, want %q %v", url, i, c.ID, LockedOf(c), wantIDs[i], wantLocked[i])
			}
		}
	}
}

func TestComicWalkerFetchChapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("episodeId") {
		case "e1":
			fmt.Fprint(w, comicWalkerViewerFixture)
		case "e2":
			fmt.Fprint(w, `{"promotionsEnd":[],"manuscripts":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	g := NewComicWalker(&Grabber{URL: srv.URL + "/detail/KC_000061_S", Settings: &Settings{}})

	chapter, err := g.FetchChapter(&ComicWalkerChapter{Chapter{Title: "第1話", Number: 1, ID: "e1"}})
	if err != nil {
		t.Fatalf("FetchChapter(e1) error: %v", err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[0].Transform == nil || chapter.Pages[1].Transform != nil {
		t.Errorf("pages = %+v, want an XOR page and a raw page", chapter.Pages)
	}
	if chapter.ID != "e1" || chapter.PagesCount != 2 {
		t.Errorf("chapter = id %q pages %d, want e1 and 2", chapter.ID, chapter.PagesCount)
	}

	if _, err := g.FetchChapter(&ComicWalkerChapter{Chapter{Title: "第2話", Number: 2, ID: "e2"}}); err == nil {
		t.Error("FetchChapter(e2) succeeded for an episode with no pages, want an error")
	}
}
