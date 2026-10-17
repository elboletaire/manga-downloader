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

func TestGigaviewerSeries(t *testing.T) {
	cases := map[string]string{
		"少年ジャンプ＋（夫婦と16歳）": "夫婦と16歳",
		"OUR FEEL（アワフィール）| 女性マンガレーベル、第1・3木曜日更新!!（うさぴょん）":                                         "うさぴょん",
		"まんがタイムSquare｜読者と作品の広場（スクエア）。（ふたりの距離の10年後）":                                              "ふたりの距離の10年後",
		"コミックガルド（姉の引き立て役に徹してきましたが、今日でやめます（タテ読みフルカラー））":                                           "姉の引き立て役に徹してきましたが、今日でやめます（タテ読みフルカラー）",
		"コミック アース・スター｜毎週木曜・最新話更新！無料で漫画が読めるWEBコミック誌（魔物（マンドラゴラ）ってバレたら討伐ですか？ ～花の魔女のほほえみは勘違いの種を蒔く～）": "魔物（マンドラゴラ）ってバレたら討伐ですか？ ～花の魔女のほほえみは勘違いの種を蒔く～",
		"no parentheses": "no parentheses",
	}
	for channel, want := range cases {
		if got := gigaviewerSeries(channel); got != want {
			t.Errorf("gigaviewerSeries(%q) = %q, want %q", channel, got, want)
		}
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
		// live titles that were numbered 0 before
		{"【第153話】地底湖の決戦③", "夫婦", "地底湖の決戦③", "", 153},
		{"【第112話前編】予定", "夫婦", "予定", "前編", 112},
		{"［第五十七話］乾季の呼び声\u3000後編②", "夫婦", "乾季の呼び声\u3000後編②", "", 57},
		{"第八十話（3）", "夫婦", "", "3", 80},
		{"第八十話（１）", "夫婦", "", "1", 80},
		{"第二十七話②", "夫婦", "", "2", 27},
		{"[第19話②] みんな〇んじゃえ", "夫婦", "みんな〇んじゃえ", "2", 19},
		{"第二十七話① 後日談", "夫婦", "後日談", "1", 27},
		{"第五十七話（3）", "夫婦", "", "3", 57},
		{"第12＆13話", "夫婦", "", "", 12},
		{"第126.5話", "夫婦", "", "", 126.5},
		{"第４９．５話\u3000パラオへ", "夫婦", "パラオへ", "", 49.5},
		{"Season26 中編", "夫婦", "", "中編", 26},
		{"Season26 前編 魔剣", "夫婦", "魔剣", "前編", 26},
		{"【第110話後編】ヤる時", "ぐらんぶる", "ヤる時", "後編", 110},
		{"第43話 告知", "夫婦", "告知", "", 43},
		{"Case.25\u3000魔剣", "夫婦", "魔剣", "", 25},
		{"Episode.3", "夫婦", "", "", 3},
		{"Op.7-1", "夫婦", "", "1", 7},
		{"#8 身内は身内でよそはよそ", "夫婦", "身内は身内でよそはよそ", "", 8},
		{"10", "夫婦", "", "", 10},
		// only a title's start is a number: a subtitle's is not
		{"身内 第3話", "夫婦", "身内 第3話", "", 0},
		{"2026年前期・第1回受賞作", "夫婦", "2026年前期・第1回受賞作", "", 0},
		{"最終話 さよならシナモン", "夫婦", "最終話 さよならシナモン", "", 0},
	}
	for _, c := range cases {
		title, number, part := gigaviewerTitle(c.raw, c.series)
		if title != c.title || number != c.number || part != c.part {
			t.Errorf("gigaviewerTitle(%q, %q) = %q, %v, %q; want %q, %v, %q",
				c.raw, c.series, title, number, part, c.title, c.number, c.part)
		}
	}
}

// TestGigaviewerNotice checks which feed items are dropped as announcements or ads
func TestGigaviewerNotice(t *testing.T) {
	isNotice := func(raw string) bool {
		_, number, _ := gigaviewerTitle(raw, "夫婦")
		return gigaviewerIsNotice(raw, number)
	}
	for _, title := range []string{"休載のお知らせ", "[休載イラスト] 聖母の断罪", "単行本告知イラスト", "更新日変更のお知らせ", "単行本第④巻のお知らせ", "[休載イラスト＋第2回人気投票企画]魔都精兵のスレイブ"} {
		if !isNotice(title) {
			t.Errorf("%q is not dropped", title)
		}
	}
	for _, title := range []string{"読切", "番外編", "特別編", "予告編", "番外編①", "第43話 告知"} {
		if isNotice(title) {
			t.Errorf("%q is dropped, want kept", title)
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
