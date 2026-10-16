// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gigaviewerFeedFixture is a series feed, newest episode first
const gigaviewerFeedFixture = `<?xml version="1.0"?>
<rss version="2.0" xmlns:giga="https://gigaviewer.com">
    <channel>
        <title>少年ジャンプ＋（夫婦と16歳）</title>
        <item>
            <title>[特別読切]番外編</title>
            <link>https://shonenjumpplus.com/episode/3</link>
        </item>
        <item>
            <title>[2話]夫婦と16歳</title>
            <link>https://shonenjumpplus.com/episode/2</link>
        </item>
        <item>
            <title>[1話]夫婦と16歳</title>
            <link>https://shonenjumpplus.com/episode/1</link>
        </item>
    </channel>
</rss>`

// gigaviewerFreeFeedFixture is the free_only view of the same series: the
// special is paid
const gigaviewerFreeFeedFixture = `<?xml version="1.0"?>
<rss version="2.0" xmlns:giga="https://gigaviewer.com">
    <channel>
        <title>少年ジャンプ＋（夫婦と16歳）</title>
        <item>
            <title>[2話]夫婦と16歳</title>
            <link>https://shonenjumpplus.com/episode/2</link>
        </item>
        <item>
            <title>[1話]夫婦と16歳</title>
            <link>https://shonenjumpplus.com/episode/1</link>
        </item>
    </channel>
</rss>`

// gigaviewerEpisodePage is an episode page, with the JSON it embeds
func gigaviewerEpisodePage(seriesID string, public bool, structure string) string {
	data := fmt.Sprintf(`{"readableProduct":{"id":"1","isPublic":%t,"series":{"id":%q},"pageStructure":%s}}`,
		public, seriesID, structure)
	return fmt.Sprintf(`<!doctype html><html><body><script id="episode-json" type="text/json" data-value="%s"></script></body></html>`,
		html.EscapeString(data))
}

func TestGigaviewerTest(t *testing.T) {
	yes := func(url string) {
		t.Helper()
		got, err := NewGigaviewer(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || !got {
			t.Errorf("Test(%q) = %v, %v; want true, nil", url, got, err)
		}
	}
	no := func(url string) {
		t.Helper()
		got, err := NewGigaviewer(&Grabber{URL: url, Settings: &Settings{}}).Test()
		if err != nil || got {
			t.Errorf("Test(%q) = %v, %v; want false, nil", url, got, err)
		}
	}

	yes("https://shonenjumpplus.com/episode/9253191257227141392")
	yes("https://www.shonenjumpplus.com/rss/series/17107419589536330341")
	no("https://example.com/?url=shonenjumpplus.com")
	no("https://shonenjumpplus.com.example.com/episode/1")
	no("https://mangadex.org/")
}

func TestGigaviewerSeriesIdentifier(t *testing.T) {
	g := NewGigaviewer(&Grabber{URL: "https://shonenjumpplus.com/rss/series/17107419589536330341", Settings: &Settings{}})
	id, err := g.seriesIdentifier()
	if err != nil || id != "17107419589536330341" {
		t.Errorf("seriesIdentifier() = %q, %v; want the feed's series ID", id, err)
	}

	g = NewGigaviewer(&Grabber{URL: "https://shonenjumpplus.com/", Settings: &Settings{}})
	if _, err := g.seriesIdentifier(); err == nil {
		t.Error("seriesIdentifier() of the site root = nil error, want one")
	}
}

func TestGigaviewerTitle(t *testing.T) {
	cases := []struct {
		raw, series, title, part string
		number                   float64
	}{
		// the series name after the marker is not a title
		{"[26話]夫婦と16歳", "夫婦と16歳", "", "", 26},
		{"[139話 後編]SPY×FAMILY", "SPY×FAMILY", "", "後編", 139},
		{"[139話 前編]SPY×FAMILY", "SPY×FAMILY", "", "前編", 139},
		{"[第１話～第４話]ふつうの軽音部", "ふつうの軽音部", "", "", 1},
		// a real subtitle stays
		{"[139話 前編]番外編", "SPY×FAMILY", "番外編", "前編", 139},
		// a special has no number marker and keeps its title as it is
		{"[特別読切]番外編", "夫婦と16歳", "[特別読切]番外編", "", 0},
		// the bare formats of other publishers
		{"第39.5話「友と食卓」", "魔導具師", "「友と食卓」", "", 39.5},
		{"第655回", "働かないふたり", "", "", 655},
		{"第1章「そしてエルフは旅に出る」", "最後のエルフ", "「そしてエルフは旅に出る」", "", 1},
		{"≪＃２≫\u3000俺の金ピカ", "宇宙兄弟", "俺の金ピカ", "", 2},
		{"最終話《＃４３２》弟 日々人と兄 六太", "宇宙兄弟", "弟 日々人と兄 六太", "", 432},
		{"第10話\u3000回りだす歯車①", "異なる形の者どもよ", "回りだす歯車①", "", 10},
		{"第3話", "うさぴょん", "", "", 3},
		{"番外編①", "まるせっせんす", "番外編①", "", 0},
		{"読切", "だから", "読切", "", 0},
	}
	for _, c := range cases {
		title, number, part := gigaviewerTitle(c.raw, c.series)
		if title != c.title || number != c.number || part != c.part {
			t.Errorf("gigaviewerTitle(%q, %q) = %q, %v, %q; want %q, %v, %q",
				c.raw, c.series, title, number, part, c.title, c.number, c.part)
		}
	}
}

func TestGigaviewerFetchChapters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rss/series/123" && r.URL.Query().Get("free_only") == "1":
			fmt.Fprint(w, gigaviewerFreeFeedFixture)
		case r.URL.Path == "/rss/series/123":
			fmt.Fprint(w, gigaviewerFeedFixture)
		case r.URL.Path == "/episode/2":
			fmt.Fprint(w, gigaviewerEpisodePage("123", true, `null`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	for _, url := range []string{srv.URL + "/rss/series/123", srv.URL + "/episode/2"} {
		g := NewGigaviewer(&Grabber{URL: url, Settings: &Settings{}})

		title, err := g.FetchTitle()
		if err != nil || title != "夫婦と16歳" {
			t.Fatalf("%s: FetchTitle() = %q, %v; want the series name", url, title, err)
		}

		chapters, errs := g.FetchChapters()
		if len(errs) != 0 || len(chapters) != 3 {
			t.Fatalf("%s: FetchChapters() = %d chapters, errs %v; want 3", url, len(chapters), errs)
		}

		// oldest first, the paid special last among them
		wantNumbers := []float64{1, 2, 0}
		wantLocked := []bool{false, false, true}
		wantIDs := []string{"1", "2", "3"}
		wantLabels := []string{"Chapter 1", "Chapter 2", "[特別読切]番外編"}
		for i, f := range chapters {
			c := f.(*GigaviewerChapter)
			if c.GetNumber() != wantNumbers[i] || LockedOf(c) != wantLocked[i] || c.ID != wantIDs[i] || ChapterLabel(c) != wantLabels[i] {
				t.Errorf("%s: chapter %d = number %v locked %v id %q label %q, want %v %v %q %q", url, i,
					c.GetNumber(), LockedOf(c), c.ID, ChapterLabel(c), wantNumbers[i], wantLocked[i], wantIDs[i], wantLabels[i])
			}
		}
	}
}

func TestGigaviewerFetchChapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/episode/2":
			fmt.Fprint(w, gigaviewerEpisodePage("123", true,
				`{"choJuGiga":"baku","pages":[{"type":"main","src":"https://img.example.com/1.jpg"},{"type":"link"},{"type":"main","src":"https://img.example.com/2.jpg"}]}`))
		case "/episode/3":
			fmt.Fprint(w, gigaviewerEpisodePage("123", false, `null`))
		case "/episode/4":
			fmt.Fprint(w, gigaviewerEpisodePage("123", true,
				`{"choJuGiga":"jigsaw","pages":[{"type":"main","src":"https://img.example.com/1.jpg"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	g := NewGigaviewer(&Grabber{URL: srv.URL + "/episode/2", Settings: &Settings{}})

	chapter, err := g.FetchChapter(&GigaviewerChapter{Chapter{Title: "[2話]夫婦と16歳", Number: 2, ID: "2"}})
	if err != nil {
		t.Fatalf("FetchChapter(2) error: %v", err)
	}
	if len(chapter.Pages) != 2 || chapter.Pages[0].URL != "https://img.example.com/1.jpg" || chapter.Pages[1].Number != 2 {
		t.Errorf("pages = %+v, want the two main pages numbered 1 and 2", chapter.Pages)
	}
	if chapter.Pages[0].Transform == nil {
		t.Error("baku pages have no Transform")
	}

	if _, err := g.FetchChapter(&GigaviewerChapter{Chapter{Title: "[3話]", Number: 3, ID: "3", Locked: true}}); err == nil {
		t.Error("FetchChapter of a paid episode = nil error, want one")
	}

	if _, err := g.FetchChapter(&GigaviewerChapter{Chapter{Title: "[4話]", Number: 4, ID: "4"}}); err == nil {
		t.Error("FetchChapter with an unknown scrambling = nil error, want one")
	}
}

// TestGigaviewerUnscramble checks the tile transposition on a synthetic page:
// the tile at column x, row y must come out where the tile at row x, column y
// was, and the strips past the 4x4 grid must stay put.
func TestGigaviewerUnscramble(t *testing.T) {
	const size = 330 // cells of 80px, and a 10px strip past the grid
	const cell = 80
	tileColor := func(x, y int) color.RGBA {
		return color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 128, A: 255}
	}

	scrambled := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := color.RGBA{R: 255, A: 255}
			if x < 4*cell && y < 4*cell {
				c = tileColor(x/cell, y/cell)
			}
			scrambled.Set(x, y, c)
		}
	}
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, scrambled); err != nil {
		t.Fatal(err)
	}

	out, err := gigaviewerUnscramble(buf.Bytes())
	if err != nil {
		t.Fatalf("gigaviewerUnscramble() error: %v", err)
	}
	got, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decoding the result: %v", err)
	}

	// JPEG is lossy, so colours only have to be close
	near := func(a, b color.Color) bool {
		ra, ga, ba, _ := a.RGBA()
		rb, gb, bb, _ := b.RGBA()
		similar := func(x, y uint32) bool {
			d := int(x>>8) - int(y>>8)
			return d > -12 && d < 12
		}
		return similar(ra, rb) && similar(ga, gb) && similar(ba, bb)
	}
	for row := range 4 {
		for col := range 4 {
			// the centre of the tile at column col, row row after descrambling
			x, y := col*cell+cell/2, row*cell+cell/2
			if want := tileColor(row, col); !near(got.At(x, y), want) {
				t.Errorf("tile col %d row %d = %v, want %v", col, row, got.At(x, y), want)
			}
		}
	}
	if !near(got.At(size-5, 5), color.RGBA{R: 255, A: 255}) {
		t.Errorf("right strip moved: got %v", got.At(size-5, 5))
	}
}

func TestGigaviewerUnscrambleRejectsNonImage(t *testing.T) {
	if _, err := gigaviewerUnscramble([]byte(strings.Repeat("x", 64))); err == nil {
		t.Error("gigaviewerUnscramble of non-image bytes = nil error, want one")
	}
}
