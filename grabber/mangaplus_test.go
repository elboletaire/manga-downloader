// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

const mangaplusTestTitleURL = "https://mangaplus.shueisha.co.jp/titles/100274"

// mangaplusTestTitleID is the title_id the fixtures below are for
const mangaplusTestTitleID = 100274

// mangaplusTestChapter is a chapter of a fixture series, encoded the way the
// API sends one: fields at their default value are left out, as protobuf does
type mangaplusTestChapter struct {
	id            uint32
	name          string
	subTitle      string
	chapterType   uint64
	alreadyViewed bool
	viewedForFree bool
}

// encode returns the chapter as a Chapter message body
func (c mangaplusTestChapter) encode() []byte {
	fields := [][]byte{
		pbTestVarint(1, mangaplusTestTitleID),
		pbTestVarint(2, uint64(c.id)),
		pbTestString(3, c.name),
		pbTestString(4, c.subTitle),
	}
	if c.chapterType != 0 {
		fields = append(fields, pbTestVarint(16, c.chapterType))
	}
	if c.alreadyViewed {
		fields = append(fields, pbTestVarint(8, 1))
	}
	if c.viewedForFree {
		fields = append(fields, pbTestVarint(11, 1))
	}

	return pbTestConcat(fields...)
}

// mangaplusTestMangaPage encodes the only Page variant that holds an image
func mangaplusTestMangaPage(imageURL string) []byte {
	mangaPage := pbTestConcat(
		pbTestString(1, imageURL),
		pbTestVarint(2, 784),
		pbTestVarint(3, 1145),
	)

	return pbTestBytes(mangaplusViewerPagesField, pbTestBytes(mangaplusPageMangaPageField, mangaPage))
}

// mangaplusTestPageVariant encodes one of the Page variants that aren't pages:
// banner_list (2), last_page (3), advertisement (4) and insert_banner_list (5)
func mangaplusTestPageVariant(variant int, payload []byte) []byte {
	return pbTestBytes(mangaplusViewerPagesField, pbTestBytes(variant, payload))
}

// mangaplusTestLanguage is one title_languages entry of the fixtures: the
// title_id an edition of the series is pinned to, and the language enum it's
// published as
type mangaplusTestLanguage struct {
	titleID  uint32
	language uint64
}

// mangaplusTestEdition is another edition of the same series, as the fake API
// serves it: the language it reports and its own chapter list, which is what
// differs per edition on the real API (one edition per language, each with its
// own title_id)
type mangaplusTestEdition struct {
	language uint64
	chapters []mangaplusTestChapter
}

// mangaplusTestTitleView encodes a TitleDetailView body
func mangaplusTestTitleView(name string, language uint64, languages []mangaplusTestLanguage, chapters ...mangaplusTestChapter) []byte {
	title := [][]byte{pbTestVarint(1, mangaplusTestTitleID), pbTestString(2, name)}
	if language != 0 {
		title = append(title, pbTestVarint(7, language))
	}

	fields := [][]byte{pbTestBytes(1, pbTestConcat(title...))}
	// title_languages(27): title_id(1), language(2)
	for _, l := range languages {
		fields = append(fields, pbTestBytes(mangaplusTitleLanguagesField, pbTestConcat(
			pbTestVarint(1, uint64(l.titleID)),
			pbTestVarint(2, l.language),
		)))
	}
	for _, chapter := range chapters {
		fields = append(fields, pbTestBytes(mangaplusChapterListV2Field, chapter.encode()))
	}

	return pbTestConcat(fields...)
}

// mangaplusTestWindowChapters returns more chapters than the free window
// lists: it's what keeps the notice about landing on a restricted edition
// (covered on its own below) out of the tests that aren't about it
func mangaplusTestWindowChapters() []mangaplusTestChapter {
	chapters := make([]mangaplusTestChapter, 0, mangaplusFreeWindowChapters+1)
	for i := 1; i <= mangaplusFreeWindowChapters+1; i++ {
		chapters = append(chapters, mangaplusTestChapter{
			id:       uint32(200 + i),
			name:     fmt.Sprintf("#%03d", i),
			subTitle: fmt.Sprintf("Chapter %d: Misión", i),
		})
	}

	return chapters
}

// mangaplusTestPopup encodes an ErrorPopup
func mangaplusTestPopup(subject, body string) []byte {
	return pbTestConcat(pbTestString(mangaplusErrorPopupSubjectField, subject), pbTestString(mangaplusErrorPopupBodyField, body))
}

// mangaplusTestErrorResponse encodes the error envelope of a logical failure,
// which the API answers with HTTP 200 like any other response
func mangaplusTestErrorResponse(code string) []byte {
	popup := mangaplusTestPopup("Invalid Parameter", fmt.Sprintf("There are issues connecting to Manga+. Please try again later.(%s)", code))

	return pbTestBytes(mangaplusErrorField, pbTestConcat(pbTestBytes(mangaplusErrorPopupField, popup)))
}

// mangaplusTestRequest is one call the fake API server served
type mangaplusTestRequest struct {
	method string
	path   string
	query  url.Values
	agent  string
}

// mangaplusTestServer fakes the app API: it registers devices, serves one
// series' title detail and its chapters' viewer responses, and records what it
// was asked for. Every viewer response mints a token of its own, the way the
// real API does.
type mangaplusTestServer struct {
	*httptest.Server
	// title is the name the fake series reports
	title string
	// language is the language enum the edition the server was built with
	// reports (0 being English, the enum's first value, which the wire leaves
	// out)
	language uint64
	// languages is the title_languages list (field 27) every title view carries:
	// the editions the series is published as, one per language
	languages []mangaplusTestLanguage
	// editions maps the title_id of the series' other editions to what each of
	// them reports, the server's own language and chapter list serving as the
	// edition its URL points at. A title_id absent from it is served that same
	// default, so a test can tell an unexpected refetch by its call count
	editions map[uint32]mangaplusTestEdition
	// secret is the device secret /register hands out
	secret string
	// chapters is what title_detailV3 returns
	chapters []mangaplusTestChapter
	// pages maps a chapter_id to the image URLs its viewer response returns
	pages map[uint32][]string
	// errorCode, when set, makes every call answer the API's error envelope
	// with that code, which is how the API reports its logical failures (over
	// HTTP 200, like any other response)
	errorCode string
	// errorTitleIDs, when set, makes title_detailV3 answer the API's error
	// envelope with that code for those title_ids only. It's how a test can
	// make one edition's fetch fail while the URL's succeeds — the switch is
	// only reached after that first fetch — which is what leaves a grabber
	// with no cached detail to skip the switch by
	errorTitleIDs map[uint32]string
	// omitToken, when set, makes viewer responses omit the plus_vw_token
	// cookie, which is the only place that token exists
	omitToken bool
	// requests is every call served, in order
	requests []mangaplusTestRequest
	// viewerCalls counts manga_viewer_v3 requests, which mint a new token each
	viewerCalls int
}

func newMangaplusTestServer(t *testing.T, chapters []mangaplusTestChapter, pages map[uint32][]string) *mangaplusTestServer {
	t.Helper()

	s := &mangaplusTestServer{
		title:    "Test Series",
		secret:   strings.Repeat("a", 32),
		chapters: chapters,
		pages:    pages,
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)

	return s
}

func (s *mangaplusTestServer) handle(w http.ResponseWriter, r *http.Request) {
	s.requests = append(s.requests, mangaplusTestRequest{
		method: r.Method,
		path:   r.URL.Path,
		query:  r.URL.Query(),
		agent:  r.UserAgent(),
	})

	switch r.URL.Path {
	case "/register":
		if s.errorCode != "" {
			_, _ = w.Write(mangaplusTestErrorResponse(s.errorCode))
			return
		}
		_, _ = w.Write(pbTestBytes(mangaplusSuccessField, pbTestBytes(
			mangaplusRegistrationDataField,
			pbTestString(mangaplusRegistrationSecretField, s.secret),
		)))
	case "/title_detailV3":
		if s.errorCode != "" {
			_, _ = w.Write(mangaplusTestErrorResponse(s.errorCode))
			return
		}
		titleID, _ := strconv.ParseUint(r.URL.Query().Get("title_id"), 10, 32)
		if code, ok := s.errorTitleIDs[uint32(titleID)]; ok {
			_, _ = w.Write(mangaplusTestErrorResponse(code))
			return
		}
		language, chapters := s.language, s.chapters
		if edition, ok := s.editions[uint32(titleID)]; ok {
			language, chapters = edition.language, edition.chapters
		}
		_, _ = w.Write(pbTestBytes(mangaplusSuccessField, pbTestBytes(
			mangaplusTitleDetailViewField,
			mangaplusTestTitleView(s.title, language, s.languages, chapters...),
		)))
	case "/manga_viewer_v3":
		if s.errorCode != "" {
			_, _ = w.Write(mangaplusTestErrorResponse(s.errorCode))
			return
		}
		s.viewerCalls++
		chapterID, _ := strconv.ParseUint(r.URL.Query().Get("chapter_id"), 10, 32)

		// a token bound to this response's own image URLs, as the real API
		// mints one per viewer response
		if !s.omitToken {
			http.SetCookie(w, &http.Cookie{
				Name:  mangaplusViewTokenCookie,
				Value: fmt.Sprintf("%032d", s.viewerCalls),
			})
		}
		_, _ = w.Write(pbTestBytes(mangaplusSuccessField, pbTestBytes(
			mangaplusViewerField,
			s.viewer(uint32(chapterID)),
		)))
	default:
		http.NotFound(w, r)
	}
}

// viewer returns the MangaViewer body of a chapter: its image pages, plus the
// banner and advertisement entries the API nests among them
func (s *mangaplusTestServer) viewer(chapterID uint32) []byte {
	fields := [][]byte{pbTestVarint(2, uint64(chapterID))}
	for _, imageURL := range s.pages[chapterID] {
		fields = append(fields, mangaplusTestMangaPage(imageURL))
	}
	fields = append(fields,
		mangaplusTestPageVariant(4, pbTestString(1, "ca-app-pub-0000000000000000/0000000000")),
		mangaplusTestPageVariant(2, pbTestString(1, "banner")),
		pbTestVarint(9, mangaplusTestTitleID),
	)

	return pbTestConcat(fields...)
}

// calls returns the requests served for a path, in order
func (s *mangaplusTestServer) calls(path string) []mangaplusTestRequest {
	var calls []mangaplusTestRequest

	for _, request := range s.requests {
		if request.path == path {
			calls = append(calls, request)
		}
	}

	return calls
}

// mangaplusTestAPI points the grabber at a local API for the duration of a
// test, and shrinks the interval between calls so tests don't spend real
// seconds on the rate limit
func mangaplusTestAPI(t *testing.T, apiURL string, interval time.Duration) {
	t.Helper()

	base, rate := mangaplusAPIBase, mangaplusRequestInterval
	mangaplusAPIBase, mangaplusRequestInterval = apiURL, interval
	t.Cleanup(func() {
		mangaplusAPIBase, mangaplusRequestInterval = base, rate
	})
}

// mangaplusTestGrabber returns a grabber pointed at the fake API, with a
// language set and a device secret that needs no registration (unless the test
// is about exactly that)
func mangaplusTestGrabber(t *testing.T, srv *mangaplusTestServer, url, language string) *Mangaplus {
	t.Helper()

	return mangaplusTestGrabberAt(t, srv.URL, url, language)
}

// mangaplusTestGrabberAt points a grabber at any local API URL, for the tests
// that need a server of their own (one answering status codes, say)
func mangaplusTestGrabberAt(t *testing.T, apiURL, url, language string) *Mangaplus {
	t.Helper()

	mangaplusTestAPI(t, apiURL, time.Millisecond)
	t.Setenv(mangaplusSecretEnv, strings.Repeat("f", 32))

	return NewMangaplus(&Grabber{URL: url, Settings: &Settings{Language: language}})
}

// mangaplusTestCaptureOutput captures what the grabber prints while fn runs.
// The lines it shows are part of its contract — one claiming a series is
// published in a single language is what sent a user looking for a language the
// site doesn't have — so they're asserted like any other output.
func mangaplusTestCaptureOutput(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	output, noColor := color.Output, color.NoColor
	color.Output, color.NoColor = &buf, true
	t.Cleanup(func() { color.Output, color.NoColor = output, noColor })

	fn()

	return buf.String()
}

func TestMangaplusTest(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://mangaplus.shueisha.co.jp/titles/100274", true},
		{"https://mangaplus.shueisha.co.jp/titles/100274/", true},
		{"http://mangaplus.shueisha.co.jp/viewer/1023486", true},
		{"https://mangaplus.shueisha.co.jp/viewer/1023486/", true},
		{"https://mangaplus.shueisha.co.jp/viewer/1023486#page=3", true},
		{"https://mangaplus.shueisha.co.jp/", false},
		{"https://mangaplus.shueisha.co.jp/titles/", false},
		{"https://mangaplus.shueisha.co.jp/titles/100274x", false},
		{"https://mangaplus.shueisha.co.jp/manga/100274", false},
		{"https://example.com/titles/100274", false},
		// a domain that only ends with the site's one isn't the site
		{"https://mangaplus.shueisha.co.jp.example.com/titles/100274", false},
		{"https://jumpg-api.tokyo-cdn.com/api/title_detailV3?title_id=100274", false},
		{"", false},
	}

	for _, c := range cases {
		m := Mangaplus{Grabber: &Grabber{URL: c.url}}
		got, err := m.Test()
		if err != nil {
			t.Errorf("Test(%q) error = %v, want none", c.url, err)
		}
		if got != c.want {
			t.Errorf("Test(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

// The site has to be identified from its URL, without a request: it's
// registered before the grabbers that fetch the page to tell, so a looser
// earlier match would shadow it (and cost an API call)
func TestMangaplusIdentifySite(t *testing.T) {
	urls := []string{
		mangaplusTestTitleURL,
		"https://mangaplus.shueisha.co.jp/viewer/1023486",
	}

	for _, url := range urls {
		site, errs := (&Grabber{URL: url, Settings: &Settings{}}).IdentifySite()
		if len(errs) > 0 {
			t.Fatalf("IdentifySite(%q) errors: %v", url, errs)
		}
		if _, ok := site.(*Mangaplus); !ok {
			t.Errorf("IdentifySite(%q) = %T, want *Mangaplus", url, site)
		}
	}
}

func TestMangaplusChapterNumber(t *testing.T) {
	cases := []struct {
		name     string
		previous float64
		want     float64
	}{
		{"#001", -0.1, 1},
		{"#1193", 1192, 1193},
		{"#012.5", 12, 12.5},
		{"001", -0.1, 1},
		{"#001-1", 1, 1.01},
		{"#001-2", 1.01, 1.02},
		{"#1193-2", 1193, 1193.02},
		// a part's number can't collide with an integer chapter number,
		// whatever the part is called: #001-10 is 1.1, never 2
		{"#001-10", 1, 1.1},
		// a chapter with no number at all goes right after the previous one
		{"ex", 57, 57.1},
		{"One-shot", 10, 10.1},
		// ...and at 0 when it's the first chapter of the series
		{"ex", -0.1, 0},
		{"One-shot", -0.1, 0},
		// "0" is a valid chapter number (prologues), not an unparseable name
		{"#000", -0.1, 0},
		{"", -0.1, 0},
		// the number must stay strictly above the previous chapter's, so the
		// API's own order survives Filterables.SortByNumber
		{"#010", 20, 20.1},
		{"#001", 1, 1.1},
		{"#001-1", 2, 2.1},
	}

	for _, c := range cases {
		if got := mangaplusChapterNumber(c.name, c.previous); got != c.want {
			t.Errorf("mangaplusChapterNumber(%q, %v) = %v, want %v", c.name, c.previous, got, c.want)
		}
	}
}

// The list the API sends is ordered, so the numbers derived from it must be
// strictly increasing even though some of its names carry no number
func TestMangaplusChapterNumberKeepsOrder(t *testing.T) {
	names := []string{"#001", "#001-1", "#001-2", "#002", "ex", "#003", "#003-1", "#004"}

	previous := -0.1
	for i, name := range names {
		number := mangaplusChapterNumber(name, previous)
		if i > 0 && number <= previous {
			t.Errorf("%q: number %v doesn't follow %v", name, number, previous)
		}
		previous = number
	}
}

// A chapter read once for free switches from FREE_FOR_FIRST_TIME to STANDARD
// while staying readable, so a non-free type alone can't tell a locked chapter
// from an already read one (which is what makes the count shown to the user
// useful; a locked chapter is only refused by the API itself)
func TestMangaplusChapterLocked(t *testing.T) {
	cases := []struct {
		name    string
		chapter mangaplusChapterInfo
		want    bool
	}{
		{"free", mangaplusChapterInfo{chapterType: 0}, false},
		{"free for the first time", mangaplusChapterInfo{chapterType: 1}, false},
		{"standard", mangaplusChapterInfo{chapterType: 2}, true},
		{"deluxe", mangaplusChapterInfo{chapterType: 3}, true},
		{"standard, already read", mangaplusChapterInfo{chapterType: 2, alreadyViewed: true}, false},
		{"standard, read for free", mangaplusChapterInfo{chapterType: 2, viewedForFree: true}, false},
		{"deluxe, already read", mangaplusChapterInfo{chapterType: 3, alreadyViewed: true}, false},
		{"deluxe, read for free", mangaplusChapterInfo{chapterType: 3, viewedForFree: true}, false},
	}

	for _, c := range cases {
		if got := c.chapter.locked(); got != c.want {
			t.Errorf("%s: locked = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMangaplusChapterTitle(t *testing.T) {
	cases := []struct {
		subTitle string
		name     string
		number   float64
		want     string
	}{
		{"Chapter 1: Romance Dawn", "#001", 1, "Romance Dawn"},
		{"Chapter 1 - Romance Dawn", "#001", 1, "Romance Dawn"},
		// a split part is written "Chapter 1.1" in its subtitle, while its number
		// keeps the part in the hundredths
		{"Chapter 1.1: Extra", "#001-1", 1.01, "Extra"},
		{"Chapter 1.1: Extra", "#001-1", 1.1, "Extra"},
		// ...but only that chapter's own number is dropped
		{"Chapter 2.1: Not this one", "#001-1", 1.01, "Chapter 2.1: Not this one"},
		// nothing to strip
		{"Bonus Chapter: Bathhouse Quest", "ex", 1.2, "Bonus Chapter: Bathhouse Quest"},
		// the prefix is only dropped when it carries the chapter's own number
		{"Chapter 9: Not this one", "#001", 1, "Chapter 9: Not this one"},
		// ...and never at the cost of an empty title
		{"Chapter 1", "#001", 1, "Chapter 1"},
		{"", "#001", 1, "#001"},
		{"Chapter 1:", "#001", 1, "Chapter 1:"},
	}

	for _, c := range cases {
		if got := mangaplusChapterTitle(c.subTitle, c.name, c.number); got != c.want {
			t.Errorf("mangaplusChapterTitle(%q, %q, %v) = %q, want %q", c.subTitle, c.name, c.number, got, c.want)
		}
	}
}

func TestMangaplusDecodeTitleDetail(t *testing.T) {
	chapters := []mangaplusTestChapter{
		{id: 11, name: "#001", subTitle: "Chapter 1: Mission"},
		{id: 12, name: "#001-1", subTitle: "Chapter 1.1: Mission, part 2"},
		{id: 13, name: "ex", subTitle: "Bonus Chapter: Bathhouse Quest", chapterType: 1, viewedForFree: true},
		{id: 14, name: "#002", subTitle: "Chapter 2: Heaps", chapterType: 2, alreadyViewed: true},
	}

	// a localized title: language 4 is Portuguese
	detail, err := decodeMangaplusTitleDetail(mangaplusTestTitleView("Kagurabachi", 4, nil, chapters...))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if detail.Title.Name != "Kagurabachi" {
		t.Errorf("title = %q, want %q", detail.Title.Name, "Kagurabachi")
	}
	if detail.Title.Language != "ptb" {
		t.Errorf("language = %q, want %q", detail.Title.Language, "ptb")
	}
	if len(detail.Chapters) != len(chapters) {
		t.Fatalf("got %d chapters, want %d", len(detail.Chapters), len(chapters))
	}

	for i, want := range chapters {
		got := detail.Chapters[i]
		if got.chapterID != want.id || got.name != want.name || got.subTitle != want.subTitle {
			t.Errorf("chapter %d = %+v, want id %d, name %q, subtitle %q", i, got, want.id, want.name, want.subTitle)
		}
		if got.chapterType != want.chapterType || got.alreadyViewed != want.alreadyViewed || got.viewedForFree != want.viewedForFree {
			t.Errorf(
				"chapter %d flags = {type:%d viewed:%v free:%v}, want {type:%d viewed:%v free:%v}",
				i, got.chapterType, got.alreadyViewed, got.viewedForFree,
				want.chapterType, want.alreadyViewed, want.viewedForFree,
			)
		}
	}

	// an English title reports no language at all: the enum's first value is
	// left out of the wire, like any other default
	english, err := decodeMangaplusTitleDetail(mangaplusTestTitleView("Kagurabachi", 0, nil, chapters[0]))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if english.Title.Language != "eng" {
		t.Errorf("language = %q, want %q", english.Title.Language, "eng")
	}
}

// chapter_list_v2 is the complete list, and chapter_list_group is the older
// truncated one: it's only decoded when the complete list is missing, not in
// addition to it
func TestMangaplusDecodeTitleDetailChapterListGroup(t *testing.T) {
	first := mangaplusTestChapter{id: 1, name: "#001", subTitle: "Chapter 1"}
	mid := mangaplusTestChapter{id: 2, name: "#002", subTitle: "Chapter 2"}
	last := mangaplusTestChapter{id: 3, name: "#003", subTitle: "Chapter 3"}

	group := pbTestBytes(mangaplusChapterListGroupField, pbTestConcat(
		pbTestBytes(mangaplusChapterGroupFirstField, first.encode()),
		pbTestBytes(mangaplusChapterGroupMidField, mid.encode()),
		pbTestBytes(mangaplusChapterGroupLastField, last.encode()),
	))
	title := pbTestBytes(1, pbTestConcat(pbTestVarint(1, mangaplusTestTitleID), pbTestString(2, "Kagurabachi")))

	detail, err := decodeMangaplusTitleDetail(pbTestConcat(title, group))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if len(detail.Chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(detail.Chapters))
	}
	for i, want := range []mangaplusTestChapter{first, mid, last} {
		if detail.Chapters[i].chapterID != want.id {
			t.Errorf("chapter %d has id %d, want %d", i, detail.Chapters[i].chapterID, want.id)
		}
	}

	// with the complete list present, the grouped one is ignored even when it
	// comes first on the wire
	complete := mangaplusTestChapter{id: 9, name: "#009", subTitle: "Chapter 9"}
	both, err := decodeMangaplusTitleDetail(pbTestConcat(
		title,
		group,
		pbTestBytes(mangaplusChapterListV2Field, complete.encode()),
	))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if len(both.Chapters) != 1 || both.Chapters[0].chapterID != complete.id {
		t.Errorf("got %+v, want only chapter %d", both.Chapters, complete.id)
	}
}

// The windows of the older list can overlap (a long series lists the same
// chapter in two of them): a chapter listed twice would be packed twice, which
// shows up as a spurious v2 file
func TestMangaplusDecodeTitleDetailChapterListGroupDedupes(t *testing.T) {
	first := mangaplusTestChapter{id: 1, name: "#001", subTitle: "Chapter 1"}
	second := mangaplusTestChapter{id: 2, name: "#002", subTitle: "Chapter 2"}
	third := mangaplusTestChapter{id: 3, name: "#003", subTitle: "Chapter 3"}

	// the mid window repeats the chapter the first one already listed
	group := pbTestBytes(28, pbTestConcat(
		pbTestBytes(2, first.encode()),
		pbTestBytes(2, second.encode()),
		pbTestBytes(3, second.encode()),
		pbTestBytes(3, third.encode()),
	))
	title := pbTestBytes(1, pbTestConcat(pbTestVarint(1, mangaplusTestTitleID), pbTestString(2, "Kagurabachi")))

	detail, err := decodeMangaplusTitleDetail(pbTestConcat(title, group))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}

	want := []uint32{first.id, second.id, third.id}
	if len(detail.Chapters) != len(want) {
		t.Fatalf("got %d chapters, want %d (the repeated one deduped)", len(detail.Chapters), len(want))
	}
	for i, id := range want {
		if detail.Chapters[i].chapterID != id {
			t.Errorf("chapter %d has id %d, want %d", i, detail.Chapters[i].chapterID, id)
		}
	}

	// a chapter the API sent without an id can't be downloaded at all, but it
	// must not be deduped either: collapsing every id-less one into the first
	// would lose most of the older list
	idless := pbTestBytes(28, pbTestConcat(
		pbTestBytes(2, mangaplusTestChapter{name: "#001", subTitle: "Chapter 1"}.encode()),
		pbTestBytes(2, mangaplusTestChapter{name: "#002", subTitle: "Chapter 2"}.encode()),
	))
	detail, err = decodeMangaplusTitleDetail(pbTestConcat(title, idless))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if len(detail.Chapters) != 2 {
		t.Fatalf("got %d chapters, want both id-less ones kept", len(detail.Chapters))
	}
	if detail.Chapters[0].name != "#001" || detail.Chapters[1].name != "#002" {
		t.Errorf("chapters = %+v, want both of the id-less fixture's", detail.Chapters)
	}
}

// The field numbers are the API's schema, not an implementation detail of this
// grabber: the fixtures elsewhere here are encoded with the same constants the
// decoder reads, so a renumbered schema would keep every one of them green.
// This one writes the numbers as literals, which pins the wire contract.
func TestMangaplusWireFieldNumbers(t *testing.T) {
	// Chapter.chapter_id(2), name(3), sub_title(4), chapter_type(16)
	chapter := pbTestConcat(
		pbTestVarint(1, mangaplusTestTitleID),
		pbTestVarint(2, 1023486),
		pbTestString(3, "#067"),
		pbTestString(4, "Chapter 67: Kyoto Bloodshed Hotel"),
		pbTestVarint(16, 2),
	)
	// Title.title_id(1), name(2), language(7)
	title := pbTestConcat(
		pbTestVarint(1, mangaplusTestTitleID),
		pbTestString(2, "Kagurabachi"),
		pbTestVarint(7, 4),
	)

	// TitleDetailView.title(1), title_languages(27), chapter_list_v2(38),
	// with 27's own title_id(1) and language(2)
	detail, err := decodeMangaplusTitleDetail(pbTestConcat(
		pbTestBytes(1, title),
		pbTestBytes(27, pbTestConcat(pbTestVarint(1, 100140), pbTestVarint(2, 3))),
		pbTestBytes(38, chapter),
	))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if detail.Title.Name != "Kagurabachi" || detail.Title.Language != "ptb" {
		t.Errorf("title = %+v, want Kagurabachi in ptb", detail.Title)
	}
	if len(detail.Languages) != 1 || detail.Languages[0].titleID != 100140 || detail.Languages[0].language != "ind" {
		t.Errorf("languages = %+v, want the one edition 100140 in ind", detail.Languages)
	}
	if len(detail.Chapters) != 1 {
		t.Fatalf("got %d chapters, want 1", len(detail.Chapters))
	}
	if got := detail.Chapters[0]; got.chapterID != 1023486 || got.name != "#067" ||
		got.subTitle != "Chapter 67: Kyoto Bloodshed Hotel" || got.chapterType != 2 {
		t.Errorf("chapter = %+v, want the values of the fixture", got)
	}

	// chapter_list_group(28), whose first(2), mid(3) and last(4) windows hold
	// chapters: the older list, which is a fallback for a response without 38
	grouped := pbTestConcat(
		pbTestBytes(1, title),
		pbTestBytes(28, pbTestConcat(
			pbTestBytes(2, chapter),
			pbTestBytes(4, pbTestConcat(
				pbTestVarint(2, 1023487),
				pbTestString(3, "#068"),
				pbTestString(4, "Chapter 68: Homecoming"),
			)),
		)),
	)
	detail, err = decodeMangaplusTitleDetail(grouped)
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if len(detail.Chapters) != 2 {
		t.Fatalf("got %d chapters, want 2", len(detail.Chapters))
	}
	if got := detail.Chapters[1]; got.chapterID != 1023487 || got.name != "#068" {
		t.Errorf("second chapter = %+v, want the last window's", got)
	}

	// Page.manga_page(1) -> MangaPage.image_url(1) inside MangaViewer.pages(1),
	// whose title_id is 9, inside SuccessResult.manga_viewer(10) inside
	// Response.success(1)
	viewerBody := pbTestConcat(
		pbTestVarint(2, 1023486),
		pbTestBytes(1, pbTestBytes(1, pbTestString(1, "https://assets.test/67/1.webp"))),
		pbTestVarint(9, mangaplusTestTitleID),
	)
	response := mangaplusTestDecodeResponse(t, pbTestBytes(1, pbTestBytes(10, viewerBody)))

	viewer, err := decodeMangaplusViewer(response.success, "token")
	if err != nil {
		t.Fatalf("decodeMangaplusViewer: %v", err)
	}
	if viewer.titleID != mangaplusTestTitleID {
		t.Errorf("titleID = %d, want %d", viewer.titleID, mangaplusTestTitleID)
	}
	if len(viewer.pages) != 1 || viewer.pages[0] != "https://assets.test/67/1.webp" {
		t.Errorf("pages = %v, want the fixture's single image", viewer.pages)
	}

	// ErrorResult.english_popup(2), whose subject(1) and body(2) carry the
	// message the error code is read out of
	popup := pbTestConcat(pbTestString(1, "Invalid Parameter"), pbTestString(2, "Please try again later.(10522)"))
	envelope := &mangaplusAPIResponse{}
	var apiErr *mangaplusAPIError
	if !errors.As(envelope.decode(pbTestBytes(2, pbTestBytes(2, popup))), &apiErr) {
		t.Fatal("decoding the error envelope returned no *mangaplusAPIError")
	}
	if !apiErr.hasCode("10522") {
		t.Errorf("hasCode(10522) = false for %q", apiErr.Error())
	}

	// SuccessResult.registeration_data(2) -> RegistrationData.device_secret(1),
	// written as literals here and read back with the numbers the grabber
	// registers a device with: the registration fixture elsewhere encodes both
	// sides with those same constants, so a renumbered schema would keep it —
	// and with it the whole grabber — green while never yielding a secret
	registered := strings.Repeat("e", 32)
	registration := mangaplusTestDecodeResponse(t, pbTestBytes(1, pbTestBytes(2, pbTestString(1, registered))))
	registerations := pbRepeated(registration.success, mangaplusRegistrationDataField)
	if len(registerations) != 1 {
		t.Fatalf("got %d registration data messages, want 1", len(registerations))
	}
	registerFields, err := pbMessage(registerations[0])
	if err != nil {
		t.Fatalf("pbMessage on the registration data: %v", err)
	}
	if got := pbString(registerFields, mangaplusRegistrationSecretField); got != registered {
		t.Errorf("device_secret = %q, want %q", got, registered)
	}
}

func TestMangaplusDecodeTitleDetailErrors(t *testing.T) {
	cases := []struct {
		name string
		view []byte
	}{
		{"no title name", pbTestBytes(1, pbTestVarint(1, mangaplusTestTitleID))},
		{"no chapters", pbTestConcat(pbTestBytes(1, pbTestString(2, "Kagurabachi")), pbTestBytes(mangaplusChapterListV2Field, []byte{0x16}))},
		{"a chapter list truncated mid-message", pbTestConcat(pbTestBytes(1, pbTestString(2, "Kagurabachi")), pbTestBytes(mangaplusChapterListV2Field, []byte{0x12}))},
	}

	for _, c := range cases {
		if _, err := decodeMangaplusTitleDetail(c.view); err == nil {
			t.Errorf("%s: decodeMangaplusTitleDetail returned no error", c.name)
		}
	}
}

// Only manga_page entries are images: the advertisement and banner variants of
// the Page oneof must not end up as pages of the chapter
func TestMangaplusDecodeViewer(t *testing.T) {
	images := []string{
		"https://jumpg-assets3.tokyo-cdn.com/secure/title/100274/chapter/1023486/manga_page/super_high/1.webp?hash=a",
		"https://jumpg-assets3.tokyo-cdn.com/secure/title/100274/chapter/1023486/manga_page/super_high/2.webp?hash=b",
	}

	view := pbTestConcat(
		pbTestVarint(2, 1023486),
		mangaplusTestMangaPage(images[0]),
		mangaplusTestPageVariant(4, pbTestString(1, "ca-app-pub-0000000000000000/0000000000")),
		mangaplusTestMangaPage(images[1]),
		mangaplusTestPageVariant(3, pbTestString(1, "last page")),
		mangaplusTestPageVariant(2, pbTestString(1, "banner")),
		mangaplusTestPageVariant(5, pbTestString(1, "insert banner")),
		pbTestVarint(9, mangaplusTestTitleID),
	)
	response := mangaplusTestDecodeResponse(t, pbTestBytes(mangaplusSuccessField, pbTestBytes(mangaplusViewerField, view)))

	viewer, err := decodeMangaplusViewer(response.success, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("decodeMangaplusViewer: %v", err)
	}
	if viewer.titleID != mangaplusTestTitleID {
		t.Errorf("titleID = %d, want %d", viewer.titleID, mangaplusTestTitleID)
	}
	if viewer.token != "0123456789abcdef0123456789abcdef" {
		t.Errorf("token = %q, want the response's own", viewer.token)
	}
	if len(viewer.pages) != len(images) {
		t.Fatalf("got %d pages, want %d: %v", len(viewer.pages), len(images), viewer.pages)
	}
	for i, want := range images {
		if viewer.pages[i] != want {
			t.Errorf("page %d = %q, want %q", i+1, viewer.pages[i], want)
		}
	}
}

func TestMangaplusDecodeViewerErrors(t *testing.T) {
	// no viewer data at all
	response := mangaplusTestDecodeResponse(t, pbTestBytes(mangaplusSuccessField, pbTestBytes(3, pbTestString(1, "something else"))))
	if _, err := decodeMangaplusViewer(response.success, "token"); err == nil {
		t.Error("expected an error for a response with no viewer data")
	}

	// an image page with no URL would silently shorten the chapter
	view := pbTestConcat(mangaplusTestMangaPage(""), pbTestVarint(9, mangaplusTestTitleID))
	response = mangaplusTestDecodeResponse(t, pbTestBytes(mangaplusSuccessField, pbTestBytes(mangaplusViewerField, view)))
	if _, err := decodeMangaplusViewer(response.success, "token"); err == nil {
		t.Error("expected an error for an image page without an image")
	}
}

func TestMangaplusDecodeSuccessEnvelope(t *testing.T) {
	response := mangaplusTestDecodeResponse(t, pbTestBytes(mangaplusSuccessField, pbTestBytes(mangaplusTitleDetailViewField, mangaplusTestTitleView("Kagurabachi", 0, nil))))

	if len(pbRepeated(response.success, mangaplusTitleDetailViewField)) != 1 {
		t.Fatalf("decoded %d title details, want 1", len(pbRepeated(response.success, mangaplusTitleDetailViewField)))
	}
}

func TestMangaplusDecodeResponseErrors(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{"truncated", []byte{0x0a, 0x08, 0x12}},
		{"neither success nor error", pbTestString(3, "unexpected")},
		{"empty", nil},
	}

	for _, c := range cases {
		response := &mangaplusAPIResponse{}
		if err := response.decode(c.body); err == nil {
			t.Errorf("%s: decode returned no error", c.name)
		}
	}
}

// The API reports every failure with the same generic popup and appends a
// numeric code to it, so the code is what has to be classified
func TestMangaplusAPIErrorClassification(t *testing.T) {
	m := NewMangaplus(&Grabber{URL: mangaplusTestTitleURL, Settings: &Settings{}})

	cases := []struct {
		name string
		code string
		want error
	}{
		// the codes are literals, not the constants they're classified by:
		// an envelope built out of the same constant it's matched against
		// would agree with itself whatever the value
		{"a chapter behind a subscription", "11301", errMangaplusSubscription},
		{"a subscription's alternate code", "11302", errMangaplusSubscription},
		{"a device secret the API doesn't know", "11012", errMangaplusDeviceSecret},
		{"a missing device secret", "10521", errMangaplusDeviceSecret},
		{"a rate limited device", "10522", errMangaplusRateLimited},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			response := &mangaplusAPIResponse{}
			err := response.decode(mangaplusTestErrorResponse(c.code))
			if err == nil {
				t.Fatal("decode returned no error for an error envelope")
			}

			var apiErr *mangaplusAPIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error %v is not a *mangaplusAPIError", err)
			}
			if !apiErr.hasCode(c.code) {
				t.Errorf("hasCode(%q) = false", c.code)
			}

			explained := m.explainAPIError(err)
			if !errors.Is(explained, c.want) {
				t.Errorf("explainAPIError = %v, want it to wrap %v", explained, c.want)
			}
			// the API's own message is kept: it's the only thing that says
			// which chapter or which account is at fault
			if !strings.Contains(explained.Error(), c.code) {
				t.Errorf("explainAPIError = %q, want it to mention the API's own message", explained)
			}
		})
	}
}

// The secret is rejected when the API doesn't know it (11012), which only
// happens for a stale cached secret or a MANGAPLUS_SECRET from somewhere else;
// a missing one (10521) is the same problem for the user. Either way the message
// has to name both escape hatches: the environment variable wins over the
// cached file, so deleting the file alone wouldn't help while one is set.
func TestMangaplusRejectedSecretIsActionable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path, pathErr := mangaplusSecretPath()
	if pathErr != nil {
		t.Fatalf("mangaplusSecretPath: %v", pathErr)
	}

	for _, code := range []string{mangaplusErrUnknownSecret, mangaplusErrMissingSecret} {
		t.Run(code, func(t *testing.T) {
			response := &mangaplusAPIResponse{}
			err := response.decode(mangaplusTestErrorResponse(code))
			if err == nil {
				t.Fatal("decode returned no error for an error envelope")
			}

			m := NewMangaplus(&Grabber{URL: mangaplusTestTitleURL, Settings: &Settings{}})
			explained := m.explainAPIError(err)
			if !errors.Is(explained, errMangaplusDeviceSecret) {
				t.Errorf("explainAPIError = %v, want it to wrap %v", explained, errMangaplusDeviceSecret)
			}

			for _, want := range []string{mangaplusSecretEnv, path, code} {
				if !strings.Contains(explained.Error(), want) {
					t.Errorf("explainAPIError = %q, want it to mention %q", explained, want)
				}
			}

			// the env var is named as the first escape hatch, which is the one
			// that wins over the file
			if strings.Index(explained.Error(), mangaplusSecretEnv) > strings.Index(explained.Error(), path) {
				t.Errorf("explainAPIError = %q, want %s named before the cached file", explained, mangaplusSecretEnv)
			}
		})
	}
}

// A rate limited device (10522) is locked out for ten or more minutes, so every
// call after it must be refused locally: on a 1193-chapter title that would
// otherwise be one refused request per chapter, all of them feeding the limiter.
//
// Where the refusal sits matters as much as its stickiness: the short-circuit
// has to come *before* the rate limiter, or each of those chapters would also
// sleep out a whole interval (ten minutes of it on that same title). The
// interval below is what makes that ordering observable, and nothing in the
// passing case waits for it: the call that does reach the limiter is the first
// one, and the refused ones never reach it.
func TestMangaplusRateLimitIsSticky(t *testing.T) {
	const interval = 2 * time.Second

	srv := newMangaplusTestServer(t, nil, nil)
	srv.errorCode = mangaplusErrRateLimited
	// built with the tests' millisecond interval, so the one call that has to
	// wait on the limiter doesn't spend the big interval below
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "")

	_, err := m.apiGet("title_detailV3", nil)
	if !errors.Is(err, errMangaplusRateLimited) {
		t.Fatalf("first call error = %v, want %v", err, errMangaplusRateLimited)
	}
	if !strings.Contains(err.Error(), "ten minutes") {
		t.Errorf("first call error = %q, want an actionable message", err)
	}
	if len(srv.requests) != 1 {
		t.Fatalf("the first call made %d requests, want 1", len(srv.requests))
	}

	// a real limiter at a real interval: every call below must return without
	// touching it
	limiter := time.NewTicker(interval)
	defer limiter.Stop()
	m.rateLimiter = limiter.C

	for i := 2; i <= 4; i++ {
		start := time.Now()
		if _, err = m.apiGet("title_detailV3", nil); !errors.Is(err, errMangaplusRateLimited) {
			t.Errorf("call %d error = %v, want %v", i, err, errMangaplusRateLimited)
		}
		if waited := time.Since(start); waited > interval/4 {
			t.Fatalf("refused call %d waited %s, want well under %s: the refusal has to short-circuit before the rate limiter", i, waited, interval/4)
		}
	}
	if len(srv.requests) != 1 {
		t.Errorf("got %d requests, want 1: a rate limited device must not be called again", len(srv.requests))
	}
}

func TestMangaplusAPIErrorUnknownCode(t *testing.T) {
	response := &mangaplusAPIResponse{}
	err := response.decode(mangaplusTestErrorResponse("99999"))
	if err == nil {
		t.Fatal("decode returned no error for an error envelope")
	}

	m := NewMangaplus(&Grabber{URL: mangaplusTestTitleURL, Settings: &Settings{}})
	explained := m.explainAPIError(err)
	if explained.Error() != err.Error() {
		t.Errorf("explainAPIError = %q, want the API's message untouched (%q)", explained, err)
	}
	for _, sentinel := range []error{errMangaplusSubscription, errMangaplusDeviceSecret, errMangaplusRateLimited} {
		if errors.Is(explained, sentinel) {
			t.Errorf("an unknown code was classified as %v", sentinel)
		}
	}
}

// The full fetch flow against the fake API: the chapter numbers derived from
// the API's names, the titles stripped of their redundant number, and no
// chapter filtered out for looking paywalled
func TestMangaplusFetchChapters(t *testing.T) {
	chapters := []mangaplusTestChapter{
		{id: 101, name: "#001", subTitle: "Chapter 1: Romance Dawn"},
		{id: 102, name: "#001-1", subTitle: "Chapter 1.1: Romance Dawn, part 2"},
		{id: 103, name: "ex", subTitle: "Bonus Chapter: Bathhouse Quest", chapterType: 1},
		{id: 104, name: "#002", subTitle: "Chapter 2: Heaps", chapterType: 2},
		{id: 105, name: "#003", subTitle: "Chapter 3: Witness", chapterType: 2, alreadyViewed: true},
		{id: 106, name: "#004", subTitle: "Chapter 4: Rod", chapterType: 2, viewedForFree: true},
	}
	srv := newMangaplusTestServer(t, chapters, nil)
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "")

	if got, err := m.FetchTitle(); err != nil || got != "Test Series" {
		t.Fatalf("FetchTitle = %q, %v, want %q", got, err, "Test Series")
	}

	list, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}

	wantNumbers := []float64{1, 1.01, 1.11, 2, 3, 4}
	wantTitles := []string{"Romance Dawn", "Romance Dawn, part 2", "Bonus Chapter: Bathhouse Quest", "Heaps", "Witness", "Rod"}

	if len(list) != len(chapters) {
		t.Fatalf("got %d chapters, want all %d (a chapter that looks paywalled must not be filtered out)", len(list), len(chapters))
	}
	for i, chapter := range list {
		mangaplus, ok := chapter.(*MangaplusChapter)
		if !ok {
			t.Fatalf("chapter %d is a %T, want a *MangaplusChapter", i, chapter)
		}
		if mangaplus.GetNumber() != wantNumbers[i] {
			t.Errorf("chapter %d number = %v, want %v", i, mangaplus.GetNumber(), wantNumbers[i])
		}
		if mangaplus.GetTitle() != wantTitles[i] {
			t.Errorf("chapter %d title = %q, want %q", i, mangaplus.GetTitle(), wantTitles[i])
		}
		if mangaplus.Language != "eng" {
			t.Errorf("chapter %d language = %q, want %q", i, mangaplus.Language, "eng")
		}
		if mangaplus.Id != chapters[i].id {
			t.Errorf("chapter %d id = %d, want %d", i, mangaplus.Id, chapters[i].id)
		}
	}

	// the list is cached: a second call spends no other API request *and*
	// returns the very slice the first one built. The request count alone
	// wouldn't tell whether it also rebuilt the chapters, since the decoded
	// title detail is cached too, so the two calls have to share a backing
	// array.
	before := len(srv.calls("/title_detailV3"))
	reused, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}
	if after := len(srv.calls("/title_detailV3")); after != before {
		t.Errorf("FetchChapters asked for the title %d times, want %d", after, before)
	}
	if len(reused) != len(list) {
		t.Fatalf("the second call returned %d chapters, want the %d cached ones", len(reused), len(list))
	}
	if &reused[0] != &list[0] {
		t.Error("the second call rebuilt the chapter list, want the cached one")
	}
}

// MangaPlus keeps one edition per language, each with its own title_id, and
// neither lang nor clang can change which one a request is answered with: the
// title_id pins it. With no --language the edition the URL points at is the one
// to download, as it is for every other grabber, so the request must not carry
// those params (asking for English would be asking for an edition the URL isn't,
// and would only be ignored) and nothing is printed about languages.
func TestMangaplusNoLanguageDownloadsTheURLEdition(t *testing.T) {
	// more chapters than the free window lists, so the notice about a
	// restricted edition (covered below) has nothing to say here
	chapters := mangaplusTestWindowChapters()
	languages := []mangaplusTestLanguage{
		{titleID: mangaplusTestTitleID, language: 1}, // the URL's edition: Spanish
		{titleID: 100020, language: 0},               // English
	}
	srv := newMangaplusTestServer(t, chapters, nil)
	srv.language, srv.languages = 1, languages
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "")

	var list Filterables
	out := mangaplusTestCaptureOutput(t, func() {
		var errs []error
		if list, errs = m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})
	if out != "" {
		t.Errorf("no --language printed %q, want nothing", out)
	}

	calls := srv.calls("/title_detailV3")
	if len(calls) != 1 {
		t.Fatalf("got %d title requests, want 1: the URL's edition is the one to download", len(calls))
	}
	if got := calls[0].query.Get("title_id"); got != strconv.Itoa(mangaplusTestTitleID) {
		t.Errorf("title_id = %q, want the URL's %d", got, mangaplusTestTitleID)
	}
	for _, name := range []string{"lang", "clang"} {
		if values, ok := calls[0].query[name]; ok {
			t.Errorf("the title request carries %s=%v, want no such param", name, values)
		}
	}

	if len(list) != len(chapters) {
		t.Fatalf("got %d chapters, want the URL edition's %d", len(list), len(chapters))
	}
	for i, chapter := range list {
		if got := chapter.(*MangaplusChapter); got.Id != chapters[i].id || got.Language != "esp" {
			t.Errorf("chapter %d = {id:%d language:%q}, want {id:%d language:%q}", i, got.Id, got.Language, chapters[i].id, "esp")
		}
	}
}

// --language matching the edition the URL already points at changes nothing:
// no refetch, and no line claiming a switch happened
func TestMangaplusLanguageOfTheURLEditionDoesNotSwitch(t *testing.T) {
	// more chapters than the free window lists, so the notice about landing on a
	// restricted edition (covered below) has nothing to say here
	chapters := mangaplusTestWindowChapters()
	languages := []mangaplusTestLanguage{
		{titleID: mangaplusTestTitleID, language: 1}, // Spanish: the URL's edition
		{titleID: 100020, language: 0},               // English
	}
	pages := map[uint32][]string{201: {"https://assets.test/201/1.webp?hash=a"}}
	srv := newMangaplusTestServer(t, chapters, pages)
	srv.language, srv.languages = 1, languages
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "es")

	var list Filterables
	out := mangaplusTestCaptureOutput(t, func() {
		var errs []error
		if list, errs = m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})
	if out != "" {
		t.Errorf("--language es on the Spanish edition printed %q, want nothing", out)
	}
	if calls := srv.calls("/title_detailV3"); len(calls) != 1 {
		t.Errorf("got %d title requests, want 1 (no switch needed)", len(calls))
	}
	if got := list[0].(*MangaplusChapter).Language; got != "esp" {
		t.Errorf("chapter language = %q, want %q", got, "esp")
	}

	if _, err := m.FetchChapter(list[0]); err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	if got := srv.calls("/manga_viewer_v3")[0].query.Get("clang"); got != "esp" {
		t.Errorf("viewer clang = %q, want %q (the edition being downloaded)", got, "esp")
	}
}

// --language naming another edition is what the edition switch is for: the
// wanted edition's title_id comes out of titleLanguages, its own detail is
// fetched in place of the URL's, and the line says exactly that
func TestMangaplusLanguageSwitchesEdition(t *testing.T) {
	const englishTitleID = 100020

	spanish := []mangaplusTestChapter{{id: 201, name: "#001", subTitle: "Chapter 1: Misión"}}
	english := []mangaplusTestChapter{
		{id: 101, name: "#001", subTitle: "Chapter 1: Mission"},
		{id: 102, name: "#002", subTitle: "Chapter 2: Heaps"},
	}
	languages := []mangaplusTestLanguage{
		{titleID: englishTitleID, language: 0},
		{titleID: mangaplusTestTitleID, language: 1},
	}
	pages := map[uint32][]string{101: {"https://assets.test/101/1.webp?hash=a"}}

	srv := newMangaplusTestServer(t, spanish, pages)
	srv.language, srv.languages = 1, languages
	srv.editions = map[uint32]mangaplusTestEdition{englishTitleID: {chapters: english}}
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "en")

	var list Filterables
	out := mangaplusTestCaptureOutput(t, func() {
		var errs []error
		if list, errs = m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})

	want := "The provided MangaPlus link is for the Spanish version of Test Series; --language selected English, so that's the version being downloaded"
	if !strings.Contains(out, want) {
		t.Errorf("printed %q, want it to contain %q", out, want)
	}

	calls := srv.calls("/title_detailV3")
	if len(calls) != 2 {
		t.Fatalf("got %d title requests, want 2 (the URL's edition, then the wanted one)", len(calls))
	}
	if got := calls[1].query.Get("title_id"); got != strconv.Itoa(englishTitleID) {
		t.Errorf("the refetch asked for title_id %q, want %d", got, englishTitleID)
	}

	if len(list) != len(english) {
		t.Fatalf("got %d chapters, want the English edition's %d", len(list), len(english))
	}
	for i, chapter := range list {
		got := chapter.(*MangaplusChapter)
		if got.Id != english[i].id || got.Language != "eng" {
			t.Errorf("chapter %d = {id:%d language:%q}, want {id:%d language:%q}", i, got.Id, got.Language, english[i].id, "eng")
		}
	}

	if _, err := m.FetchChapter(list[0]); err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	if got := srv.calls("/manga_viewer_v3")[0].query.Get("clang"); got != "eng" {
		t.Errorf("viewer clang = %q, want %q (the edition the switch resolved to)", got, "eng")
	}
}

// The line about a switch is claimed only once the refetched edition reports the
// language that was asked for. The languages list it came out of isn't always
// consistent — it can name an edition the API then answers with another
// language, or the URL's own title_id under a language its edition doesn't
// report — and a line reading "downloading the fra edition" over an edition that
// reports esp is a lie about what was downloaded (the chapters carry the
// language they actually came in).
func TestMangaplusSwitchLineMatchesWhatWasDownloaded(t *testing.T) {
	// the API lists the URL's own title_id as its French edition, and that
	// edition answers reporting the Spanish language
	srv := newMangaplusTestServer(t, mangaplusTestWindowChapters(), nil)
	srv.language = 1
	srv.languages = []mangaplusTestLanguage{
		{titleID: mangaplusTestTitleID, language: 2},
		{titleID: 100020, language: 0},
	}
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "fr")

	var list Filterables
	out := mangaplusTestCaptureOutput(t, func() {
		var errs []error
		if list, errs = m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})

	if strings.Contains(out, "The provided MangaPlus link is for the") {
		t.Errorf("printed %q, which claims a switch to a version that was never answered", out)
	}
	want := "MangaPlus's listing for the French version of Test Series doesn't match the title it returns: downloading the Spanish version"
	if !strings.Contains(out, want) {
		t.Errorf("printed %q, want it to contain %q", out, want)
	}

	// both title calls are for the same edition, and it's the Spanish one the
	// chapters come from
	calls := srv.calls("/title_detailV3")
	if len(calls) != 2 {
		t.Fatalf("got %d title requests, want 2 (the URL's edition and the one switch)", len(calls))
	}
	for i, call := range calls {
		if got := call.query.Get("title_id"); got != strconv.Itoa(mangaplusTestTitleID) {
			t.Errorf("title request %d asked for title_id %q, want %d", i, got, mangaplusTestTitleID)
		}
	}
	for i, chapter := range list {
		if got := chapter.(*MangaplusChapter).Language; got != "esp" {
			t.Errorf("chapter %d language = %q, want the %q the edition reports", i, got, "esp")
		}
	}
}

// Every code the --language flag takes is resolved to the edition MangaPlus
// publishes the title as in that language, and the API names its editions with
// 3-letter codes — which the flag takes as-is, that being the API's own format.
// A code MangaPlus doesn't publish a title under names no edition at all, so
// it's reported as such — the URL's edition is kept rather than an edition
// nobody asked for being downloaded (a mistyped code used to be resolved to
// English, which with the edition pinned by title_id only ever changed what the
// warning said).
//
// The flag's codes, the language enum and the API's code are written out below
// rather than read off the grabber's own tables: a swapped or mistyped entry
// there is exactly what this is here to catch, and deriving the expectation
// from the same table would make a swap agree with itself.
func TestMangaplusLanguageCodeForms(t *testing.T) {
	cases := []struct {
		flag string
		enum uint64
		code string
		name string
	}{
		{"es", 1, "esp", "Spanish"},
		{"fr", 2, "fra", "French"},
		{"id", 3, "ind", "Indonesian"},
		{"pt", 4, "ptb", "Portuguese (BR)"},
		{"ru", 5, "rus", "Russian"},
		{"th", 6, "tha", "Thai"},
		{"de", 7, "deu", "German"},
		{"it", 8, "ita", "Italian"},
		{"vi", 9, "vie", "Vietnamese"},
	}

	for i, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			titleID := uint32(700000 + i)
			srv := newMangaplusTestServer(t, []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}, nil)
			srv.languages = []mangaplusTestLanguage{
				{titleID: titleID, language: c.enum},
				{titleID: mangaplusTestTitleID, language: 0}, // the URL's edition: English
			}
			srv.editions = map[uint32]mangaplusTestEdition{
				titleID: {language: c.enum, chapters: []mangaplusTestChapter{{id: 201, name: "#001", subTitle: "Capítulo 1: Missão"}}},
			}

			m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, c.flag)

			var list Filterables
			out := mangaplusTestCaptureOutput(t, func() {
				var errs []error
				if list, errs = m.FetchChapters(); len(errs) > 0 {
					t.Fatalf("FetchChapters errors: %v", errs)
				}
			})
			if want := fmt.Sprintf("The provided MangaPlus link is for the English version of Test Series; --language selected %s, so that's the version being downloaded", c.name); !strings.Contains(out, want) {
				t.Errorf("-l %s printed %q, want it to contain %q", c.flag, out, want)
			}
			if got := list[0].(*MangaplusChapter).Language; got != c.code {
				t.Errorf("-l %s chapter language = %q, want %q", c.flag, got, c.code)
			}
			calls := srv.calls("/title_detailV3")
			if len(calls) != 2 || calls[1].query.Get("title_id") != strconv.Itoa(int(titleID)) {
				t.Errorf("title requests = %v, want the URL's edition and then %d", calls, titleID)
			}
		})
	}

	// the API's own 3-letter code is taken as-is (it's what the API names its
	// editions with), which is the same edition as the flag's 2-letter one
	srv := newMangaplusTestServer(t, []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}, nil)
	srv.languages = []mangaplusTestLanguage{
		{titleID: 100149, language: 4},
		{titleID: mangaplusTestTitleID, language: 0}, // the URL's edition: English
	}
	srv.editions = map[uint32]mangaplusTestEdition{
		100149: {language: 4, chapters: []mangaplusTestChapter{{id: 201, name: "#001", subTitle: "Capítulo 1: Missão"}}},
	}
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "ptb")
	var list Filterables
	out := mangaplusTestCaptureOutput(t, func() {
		var errs []error
		if list, errs = m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})
	if want := "The provided MangaPlus link is for the English version of Test Series; --language selected Portuguese (BR), so that's the version being downloaded"; !strings.Contains(out, want) {
		t.Errorf("-l ptb printed %q, want it to contain %q", out, want)
	}
	if got := list[0].(*MangaplusChapter).Language; got != "ptb" {
		t.Errorf("-l ptb chapter language = %q, want %q", got, "ptb")
	}
	if calls := srv.calls("/title_detailV3"); len(calls) != 2 || calls[1].query.Get("title_id") != "100149" {
		t.Errorf("title requests = %v, want the URL's edition and then 100149", calls)
	}

	// a code that isn't one of the site's languages can't name an edition: the
	// URL's is kept, and the line names the code itself, that having no name to
	// show
	before := len(srv.calls("/title_detailV3"))
	unknown := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "zz")
	out = mangaplusTestCaptureOutput(t, func() {
		if _, errs := unknown.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})
	if want := "MangaPlus has no zz version of Test Series (available: ptb, eng); downloading the English version the link points at"; !strings.Contains(out, want) {
		t.Errorf("printed %q, want it to contain %q", out, want)
	}
	if after := len(srv.calls("/title_detailV3")); after != before+1 {
		t.Errorf("got %d title requests, want 1 more (no edition to switch to)", after-before)
	}
}

// Every code the language enum can report has a name to show the user: the
// messages above name a language, and a code that reached them unnamed would
// read as the API's own "eng" rather than the "English" it means. A code with
// no name is returned as it is — an enum value this grabber doesn't know
// ("lang11"), or one the user typed that the site doesn't publish ("zz") —
// instead of being guessed at a language it might have been.
//
// The names are written out below rather than read off the grabber's own table,
// the same way the code forms are: a mistyped or swapped entry there is what
// this is here to catch.
func TestMangaplusLanguageNames(t *testing.T) {
	want := map[string]string{
		"eng": "English",
		"esp": "Spanish",
		"fra": "French",
		"ind": "Indonesian",
		"ptb": "Portuguese (BR)",
		"rus": "Russian",
		"tha": "Thai",
		"deu": "German",
		"ita": "Italian",
		"vie": "Vietnamese",
	}

	// the enum is where the codes shown to the user come from, so every one of
	// them has to have a name
	for code, language := range mangaplusLanguageCodes {
		name, ok := want[language]
		if !ok {
			t.Fatalf("the enum's %d is the code %q, which this test has no name for: the enum and the name table have drifted", code, language)
		}
		if got := mangaplusLanguageName(language); got != name {
			t.Errorf("mangaplusLanguageName(%q) = %q, want %q", language, got, name)
		}
	}

	// and the table names nothing the enum doesn't declare
	if len(mangaplusLanguageNames) != len(want) {
		t.Errorf("the name table has %d entries, want the enum's %d", len(mangaplusLanguageNames), len(want))
	}

	for _, code := range []string{"lang11", "zz", ""} {
		if got := mangaplusLanguageName(code); got != code {
			t.Errorf("mangaplusLanguageName(%q) = %q, want the code itself", code, got)
		}
	}
}

// The language enum is a closed list, and a value outside it is reported as the
// number it is. An edition carrying one must not read as English: with the
// edition pinned by title_id alone, a guessed code makes such an edition stand
// in for the English one — --language en would accept it, print nothing and
// label its chapters eng, while the English edition the API does list is
// ignored. Reported honestly, the lookup finds the real English edition, and
// the codes named to the user are the ones the API sent.
func TestMangaplusUnknownLanguageEnumIsNotEnglish(t *testing.T) {
	const (
		englishTitleID = 100020
		unknownEnum    = 11
	)

	// the URL's edition declares an enum this grabber doesn't know, and the
	// title has an English edition of its own
	srv := newMangaplusTestServer(t, []mangaplusTestChapter{{id: 201, name: "#001", subTitle: "Chapter 1: Misión"}}, nil)
	srv.language = unknownEnum
	srv.languages = []mangaplusTestLanguage{
		{titleID: englishTitleID, language: 0},
		{titleID: mangaplusTestTitleID, language: unknownEnum},
	}
	srv.editions = map[uint32]mangaplusTestEdition{
		englishTitleID: {language: 0, chapters: []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}},
	}
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "en")

	var list Filterables
	out := mangaplusTestCaptureOutput(t, func() {
		var errs []error
		if list, errs = m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})

	if want := "The provided MangaPlus link is for the lang11 version of Test Series; --language selected English, so that's the version being downloaded"; !strings.Contains(out, want) {
		t.Errorf("--language en on a lang11 version printed %q, want it to contain %q", out, want)
	}
	if len(list) != 1 {
		t.Fatalf("got %d chapters, want the English edition's one", len(list))
	}
	if got := list[0].(*MangaplusChapter); got.Id != 101 || got.Language != "eng" {
		t.Errorf("chapter = {id:%d language:%q}, want {id:101 language:%q}", got.Id, got.Language, "eng")
	}
	if calls := srv.calls("/title_detailV3"); len(calls) != 2 || calls[1].query.Get("title_id") != strconv.Itoa(englishTitleID) {
		t.Errorf("title requests = %v, want the URL's edition and then %d", calls, englishTitleID)
	}

	// the code of an edition whose enum isn't known is the one the API sent:
	// what's offered as available is that, never a language it might have been
	// taken for
	other := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "fr")
	out = mangaplusTestCaptureOutput(t, func() {
		if _, errs := other.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})
	if want := "MangaPlus has no French version of Test Series (available: eng, lang11); downloading the lang11 version the link points at"; !strings.Contains(out, want) {
		t.Errorf("--language fr printed %q, want it to contain %q", out, want)
	}
}

// A reader URL pins an edition the same way: the series comes from the viewer
// response, and the switch then happens against that edition's title_id
func TestMangaplusLanguageSwitchesEditionFromAViewerURL(t *testing.T) {
	const (
		seriesTitleID  = mangaplusTestTitleID
		englishTitleID = 100020
	)

	srv := newMangaplusTestServer(t, []mangaplusTestChapter{
		{id: 1023486, name: "#067", subTitle: "Chapter 67: Kyoto Bloodshed Hotel"},
	}, map[uint32][]string{
		1023486: {"https://assets.test/es/67/1.webp?hash=a"},
		101:     {"https://assets.test/en/1/1.webp?hash=b"},
	})
	srv.language = 1
	srv.languages = []mangaplusTestLanguage{
		{titleID: englishTitleID, language: 0},
		{titleID: seriesTitleID, language: 1},
	}
	srv.editions = map[uint32]mangaplusTestEdition{
		englishTitleID: {chapters: []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}},
	}

	viewerURL := "https://mangaplus.shueisha.co.jp/viewer/1023486"
	m := mangaplusTestGrabber(t, srv, viewerURL, "en")

	var list Filterables
	out := mangaplusTestCaptureOutput(t, func() {
		var errs []error
		if list, errs = m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})
	if want := "The provided MangaPlus link is for the Spanish version of Test Series; --language selected English, so that's the version being downloaded"; !strings.Contains(out, want) {
		t.Errorf("printed %q, want it to contain %q", out, want)
	}

	calls := srv.calls("/title_detailV3")
	if len(calls) != 2 {
		t.Fatalf("got %d title requests, want 2", len(calls))
	}
	if got := calls[0].query.Get("title_id"); got != strconv.Itoa(seriesTitleID) {
		t.Errorf("the first request asked for title_id %q, want the viewer's %d", got, seriesTitleID)
	}
	if got := calls[1].query.Get("title_id"); got != strconv.Itoa(englishTitleID) {
		t.Errorf("the refetch asked for title_id %q, want %d", got, englishTitleID)
	}

	if len(list) != 1 || list[0].GetNumber() != 1 {
		t.Fatalf("got %v, want the English edition's single chapter 1", list)
	}

	// the viewer call that resolved the series had no language to send yet, and
	// the chapter of the switched edition is fetched with the resolved one. The
	// param's *presence* is what's asserted: an empty clang is also what a
	// request carrying clang= reads as, and sending one before the edition is
	// known is a guess at an edition the title_id already pins
	viewerCalls := srv.calls("/manga_viewer_v3")
	if values, ok := viewerCalls[0].query["clang"]; ok {
		t.Errorf("the resolving viewer call carries clang=%v, want no such param", values)
	}
	if _, err := m.FetchChapter(list[0]); err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	if got := srv.calls("/manga_viewer_v3")[1].query.Get("clang"); got != "eng" {
		t.Errorf("viewer clang = %q, want %q", got, "eng")
	}
}

// A language the title isn't published in keeps the URL's edition. The line has
// to stay factual: it names the languages the API lists, and never claims the
// title is published in a single one (the URL's own edition is proof it isn't).
// A response whose languages list never arrived says that instead of reading
// non-publication out of a list that isn't there.
func TestMangaplusUnpublishedLanguageKeepsTheURLEdition(t *testing.T) {
	// more chapters than the free window lists, so the notice about landing on
	// a restricted edition (covered below) has nothing to say here
	chapters := mangaplusTestWindowChapters()

	cases := []struct {
		name      string
		languages []mangaplusTestLanguage
		want      []string
		notWant   []string
	}{
		{
			"the API lists other editions, just not that one",
			[]mangaplusTestLanguage{{titleID: 100020, language: 0}, {titleID: mangaplusTestTitleID, language: 1}},
			[]string{"MangaPlus has no French version of Test Series", "available: eng, esp", "downloading the Spanish version the link points at"},
			[]string{"only", "listed no other versions"},
		},
		{
			"the API listed no language list at all",
			nil,
			[]string{"MangaPlus listed no other versions of Test Series, so French isn't available", "downloading the Spanish version the link points at"},
			[]string{"has no", "available:", "only"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newMangaplusTestServer(t, chapters, nil)
			srv.language, srv.languages = 1, c.languages
			m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "fr")

			var list Filterables
			out := mangaplusTestCaptureOutput(t, func() {
				var errs []error
				if list, errs = m.FetchChapters(); len(errs) > 0 {
					t.Fatalf("FetchChapters errors: %v", errs)
				}
			})

			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("printed %q, want it to contain %q", out, want)
				}
			}
			for _, notWant := range c.notWant {
				if strings.Contains(out, notWant) {
					t.Errorf("printed %q, want none of %q", out, notWant)
				}
			}

			if calls := srv.calls("/title_detailV3"); len(calls) != 1 {
				t.Errorf("got %d title requests, want 1: an unpublished language must not refetch", len(calls))
			}
			if got := list[0].(*MangaplusChapter).Language; got != "esp" {
				t.Errorf("chapter language = %q, want the URL's %q", got, "esp")
			}
		})
	}
}

// The switch resolves one edition and no more: an answer that doesn't even
// report the language that was asked for must not send the grabber looking for
// a third edition (each lookup is a rate limited call, and the API locks a
// device out after a handful of them)
func TestMangaplusEditionSwitchHappensOnce(t *testing.T) {
	const englishTitleID = 100020

	srv := newMangaplusTestServer(t, []mangaplusTestChapter{{id: 201, name: "#001", subTitle: "Chapter 1: Misión"}}, nil)
	srv.language = 1
	srv.languages = []mangaplusTestLanguage{
		{titleID: englishTitleID, language: 0},
		{titleID: mangaplusTestTitleID, language: 1},
		{titleID: 300001, language: 2}, // French, which is not what was asked for
	}
	srv.editions = map[uint32]mangaplusTestEdition{
		// an edition that reports a language of its own rather than the one it
		// was asked for
		englishTitleID: {language: 2, chapters: []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}},
	}
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "en")

	mangaplusTestCaptureOutput(t, func() {
		if _, err := m.FetchTitle(); err != nil {
			t.Fatalf("FetchTitle: %v", err)
		}
		for i := 2; i <= 3; i++ {
			if _, errs := m.FetchChapters(); len(errs) > 0 {
				t.Fatalf("FetchChapters: %v", errs)
			}
		}
	})

	if calls := srv.calls("/title_detailV3"); len(calls) != 2 {
		t.Errorf("got %d title requests, want 2 (the URL's edition and one switch)", len(calls))
	}
}

// The switch is attempted once and no more. Its result is what makes a second
// attempt pointless in a normal run (the detail gets cached, and a failed fetch
// ends the run), so the flag that bounds it is only observable through a
// grabber whose refetch failed: the cached detail is then still nil, and the
// next call has to find the edition without asking the API for it again (each
// lookup spends a rate limited call, and the device is locked out after a
// handful of them).
func TestMangaplusEditionSwitchIsAttemptedOnce(t *testing.T) {
	const englishTitleID = 100020

	srv := newMangaplusTestServer(t, []mangaplusTestChapter{{id: 201, name: "#001", subTitle: "Chapter 1: Misión"}}, nil)
	srv.language = 1
	srv.languages = []mangaplusTestLanguage{
		{titleID: englishTitleID, language: 0},
		{titleID: mangaplusTestTitleID, language: 1},
	}
	// the wanted edition can't be fetched: the switch is reached (the URL's own
	// detail is served), and its refetch fails
	srv.errorTitleIDs = map[uint32]string{englishTitleID: mangaplusErrMissingSecret}
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "en")

	mangaplusTestCaptureOutput(t, func() {
		if _, err := m.FetchTitle(); err == nil {
			t.Fatal("FetchTitle succeeded, want the failed refetch to surface")
		}
	})
	if calls := srv.calls("/title_detailV3"); len(calls) != 2 {
		t.Fatalf("got %d title requests, want 2 (the URL's edition and the failed switch)", len(calls))
	}

	// the failed refetch left no detail behind, so this call goes through the
	// whole switch path again — except for the switch itself
	mangaplusTestCaptureOutput(t, func() {
		if _, errs := m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})

	calls := srv.calls("/title_detailV3")
	if len(calls) != 3 {
		t.Fatalf("got %d title requests, want 3: the URL's edition, one switch, and the edition fetched again", len(calls))
	}
	if got := calls[2].query.Get("title_id"); got != strconv.Itoa(mangaplusTestTitleID) {
		t.Errorf("the last request asked for title_id %q, want the URL's %d", got, mangaplusTestTitleID)
	}
}

// The title_languages list is what turns a language into the edition to ask
// for: MangaPlus has one edition per language, each with its own title_id, so
// nothing else can (the title_id of the URL pins the edition it belongs to).
// Every enum the API declares is pinned here, plus the two ways an entry can
// name no edition: a language the list doesn't carry, and an entry with no
// title_id (which can't be asked for at all, so it's no edition either).
func TestMangaplusDecodeTitleLanguages(t *testing.T) {
	languages := []mangaplusTestLanguage{
		{titleID: 100020, language: 0},               // eng
		{titleID: mangaplusTestTitleID, language: 1}, // esp
		{titleID: 700005, language: 2},               // fra
		{titleID: 100140, language: 3},               // ind
		{titleID: 100149, language: 4},               // ptb
		{titleID: 0, language: 5},                    // rus: listed, but with no title_id
		{titleID: 100079, language: 6},               // tha
		{titleID: 800001, language: 7},               // deu
		{titleID: 800002, language: 8},               // ita
		{titleID: 1000001, language: 9},              // vie
	}
	chapters := []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}

	detail, err := decodeMangaplusTitleDetail(mangaplusTestTitleView("Kagurabachi", 1, languages, chapters...))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if len(detail.Languages) != len(languages) {
		t.Fatalf("got %d editions, want %d", len(detail.Languages), len(languages))
	}

	// the enum mapping is the one Title.language uses, so the codes here are
	// what --language resolves to
	want := map[string]uint32{
		"eng": 100020, "esp": mangaplusTestTitleID, "fra": 700005, "ind": 100140,
		"ptb": 100149, "tha": 100079, "deu": 800001, "ita": 800002, "vie": 1000001,
	}
	for language, titleID := range want {
		got, ok := detail.editionTitleID(language)
		if !ok || got != titleID {
			t.Errorf("editionTitleID(%q) = %d, %v, want %d, true", language, got, ok, titleID)
		}
	}

	// a language the list doesn't carry has no edition, and neither has the
	// one it does carry without a title_id: asking for it would ask the API for
	// title_id 0, which is some other thing entirely
	for _, language := range []string{"zzz", "rus"} {
		if id, ok := detail.editionTitleID(language); ok {
			t.Errorf("editionTitleID(%q) = %d, true, want no edition", language, id)
		}
	}

	// the languages named to the user are the ones an edition can be asked
	// for: the id-less entry isn't one, so "rus" is left out (naming it would
	// offer a language that can't be downloaded)
	codes := detail.languageCodes()
	wantCodes := []string{"eng", "esp", "fra", "ind", "ptb", "tha", "deu", "ita", "vie"}
	if strings.Join(codes, ",") != strings.Join(wantCodes, ",") {
		t.Errorf("languageCodes() = %v, want the askable editions in the API's order: %v", codes, wantCodes)
	}

	// a response without title_languages names no edition at all: all there is
	// to download is its own edition, whose language is Title.Language (the
	// caller says so itself, see TestMangaplusUnpublishedLanguageKeepsTheURLEdition)
	single, err := decodeMangaplusTitleDetail(mangaplusTestTitleView("Kagurabachi", 2, nil, chapters...))
	if err != nil {
		t.Fatalf("decodeMangaplusTitleDetail: %v", err)
	}
	if got := single.languageCodes(); len(got) != 0 {
		t.Errorf("languageCodes() = %v, want none: the response listed no editions", got)
	}
	if got, ok := single.editionTitleID("fra"); ok {
		t.Errorf("editionTitleID(\"fra\") = %d, true, want no edition", got)
	}
}

// A titleLanguages entry with no title_id names an edition nobody can ask for
// (the detail call is pinned by title_id, and 0 isn't one). It must not be
// treated as an edition — the user would be told the title isn't published in
// a language the very same line then offers as available, and a request for
// title_id 0 would follow if it were believed.
func TestMangaplusLanguageListedWithoutAnEditionIsNotAvailable(t *testing.T) {
	srv := newMangaplusTestServer(t, mangaplusTestWindowChapters(), nil)
	srv.language = 1
	srv.languages = []mangaplusTestLanguage{
		{titleID: 0, language: 0},                    // English, with no title_id to ask for
		{titleID: mangaplusTestTitleID, language: 1}, // Spanish: the URL's edition
	}
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "en")

	var list Filterables
	out := mangaplusTestCaptureOutput(t, func() {
		var errs []error
		if list, errs = m.FetchChapters(); len(errs) > 0 {
			t.Fatalf("FetchChapters errors: %v", errs)
		}
	})

	if want := "MangaPlus has no English version of Test Series (available: esp); downloading the Spanish version the link points at"; !strings.Contains(out, want) {
		t.Errorf("printed %q, want it to contain %q", out, want)
	}
	if strings.Contains(out, "available: eng") {
		t.Errorf("printed %q, which offers a language no edition of the title can be asked for as available", out)
	}
	if calls := srv.calls("/title_detailV3"); len(calls) != 1 {
		t.Errorf("got %d title requests, want 1 (an entry with no title_id is not an edition to switch to)", len(calls))
	}
	if got := list[0].(*MangaplusChapter).Language; got != "esp" {
		t.Errorf("chapter language = %q, want the URL's %q", got, "esp")
	}
}

// A viewer URL is what needs a viewer call before the edition is known: that
// call carries no clang at all (the API answers without it, while a default
// would guess at an edition the title_id already pins), and every later one
// carries the language the resolved edition reports
func TestMangaplusViewerClangFollowsTheResolvedEdition(t *testing.T) {
	chapters := []mangaplusTestChapter{
		{id: 1023486, name: "#067", subTitle: "Chapter 67: Kyoto Bloodshed Hotel"},
		{id: 1023495, name: "#066", subTitle: "Chapter 66: Truth"},
	}
	pages := map[uint32][]string{
		1023486: {"https://assets.test/67/1.webp?hash=a"},
		1023495: {"https://assets.test/66/1.webp?hash=b"},
	}
	srv := newMangaplusTestServer(t, chapters, pages)
	srv.language = 1 // Spanish, reported by the edition's detail
	m := mangaplusTestGrabber(t, srv, "https://mangaplus.shueisha.co.jp/viewer/1023486", "")

	// nothing is known about the edition until its detail is fetched
	if got := m.clangLocked(); got != "" {
		t.Errorf("clangLocked() = %q before any title detail, want none", got)
	}

	// resolving the series takes a viewer call, made before any title detail
	// exists: it can't carry a language yet, and the param is absent rather than
	// empty (a clang= of any value would be a guess at an edition the title_id
	// already pins)
	if _, err := m.FetchTitle(); err != nil {
		t.Fatalf("FetchTitle: %v", err)
	}
	if values, ok := srv.calls("/manga_viewer_v3")[0].query["clang"]; ok {
		t.Errorf("the resolving viewer call carries clang=%v, want no such param", values)
	}
	if got := m.clangLocked(); got != "esp" {
		t.Errorf("clangLocked() = %q, want the resolved edition's %q", got, "esp")
	}

	list, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}
	for _, chapter := range list {
		if _, err := m.FetchChapter(chapter); err != nil {
			t.Fatalf("FetchChapter: %v", err)
		}
	}
	for i, call := range srv.calls("/manga_viewer_v3")[1:] {
		if got := call.query.Get("clang"); got != "esp" {
			t.Errorf("viewer call %d sent clang = %q, want %q", i+1, got, "esp")
		}
	}
}

// An edition that lists no more than the free window's worth of chapters, on a
// series with more than one edition, is what landing on a restricted edition
// looks like: say so, without claiming what the other editions list (finding
// that out would take one rate limited call each). It's said with --language
// too — picking a language is exactly how one lands on such an edition (a
// title's whole run can be free in one edition and the free window in another),
// and the chapter count is the only hint a run gives before downloading — after
// whatever the switch said, since it describes the edition that was downloaded.
func TestMangaplusRestrictedEditionNotice(t *testing.T) {
	window := make([]mangaplusTestChapter, 0, mangaplusFreeWindowChapters)
	for i := 1; i <= mangaplusFreeWindowChapters; i++ {
		window = append(window, mangaplusTestChapter{id: uint32(200 + i), name: fmt.Sprintf("#%03d", i), subTitle: fmt.Sprintf("Chapter %d", i)})
	}
	languages := []mangaplusTestLanguage{
		{titleID: 100020, language: 0},
		{titleID: mangaplusTestTitleID, language: 1},
	}
	// the English edition, which --language en switches to, lists the window too
	editions := map[uint32]mangaplusTestEdition{
		100020: {language: 0, chapters: window},
	}

	cases := []struct {
		name      string
		language  string
		languages []mangaplusTestLanguage
		chapters  []mangaplusTestChapter
		switchTo  bool
		want      bool
	}{
		{"a restricted edition, no --language", "", languages, window, false, true},
		{"a restricted edition picked with --language", "es", languages, window, false, true},
		{"a switch to a restricted edition", "en", languages, window, true, true},
		{"an edition with more than the window", "", languages, append(append([]mangaplusTestChapter{}, window...), mangaplusTestChapter{id: 300, name: "#007", subTitle: "Chapter 7"}), false, false},
		{"a series published in one language", "", languages[:1], window, false, false},
		{"no editions listed at all", "", nil, window, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newMangaplusTestServer(t, c.chapters, nil)
			srv.language, srv.languages = 1, c.languages
			if c.switchTo {
				srv.editions = editions
			}
			m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, c.language)

			out := mangaplusTestCaptureOutput(t, func() {
				if _, errs := m.FetchChapters(); len(errs) > 0 {
					t.Fatalf("FetchChapters errors: %v", errs)
				}
			})

			// a switch line is what the notice has to follow, never replace: both
			// say something true of the run, and neither contradicts the other
			lines := 1
			if c.switchTo {
				lines = 2
				if want := "The provided MangaPlus link is for the Spanish version of Test Series; --language selected English, so that's the version being downloaded"; !strings.Contains(out, want) {
					t.Errorf("printed %q, want it to contain %q", out, want)
				}
			}

			if !c.want {
				if out != "" {
					t.Errorf("printed %q, want nothing", out)
				}
				return
			}

			for _, want := range []string{
				fmt.Sprintf("lists only %d chapters", len(window)),
				"each language is a separate version: eng, esp",
				"use --language to pick another",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("printed %q, want it to contain %q", out, want)
				}
			}
			// one line (two after a switch), not a paragraph
			if got := strings.Count(strings.TrimSpace(out), "\n"); got != lines-1 {
				t.Errorf("printed %d lines (%q), want %d", got+1, out, lines)
			}
			if c.switchTo {
				switchAt, noticeAt := strings.Index(out, "The provided MangaPlus link is for the"), strings.Index(out, "lists only ")
				if switchAt < 0 || noticeAt < 0 || switchAt > noticeAt {
					t.Errorf("printed %q, want the switch line before the notice", out)
				}
			}
		})
	}
}

// Every page carries the token minted by the viewer response its URLs belong
// to: they're neither shared between chapters nor refetched, and --bundle
// (which fetches every chapter twice) can't make one chapter send another's
func TestMangaplusFetchChapter(t *testing.T) {
	chapters := []mangaplusTestChapter{
		{id: 101, name: "#001", subTitle: "Chapter 1: Romance Dawn"},
		{id: 102, name: "#002", subTitle: "Chapter 2: Heaps"},
	}
	pages := map[uint32][]string{
		101: {"https://assets.test/1/1.webp?hash=a", "https://assets.test/1/2.webp?hash=b"},
		102: {"https://assets.test/2/1.webp?hash=c"},
	}
	srv := newMangaplusTestServer(t, chapters, pages)
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "")

	list, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}

	tokens := map[string]bool{}
	for i, chapter := range list {
		got, err := m.FetchChapter(chapter)
		if err != nil {
			t.Fatalf("FetchChapter(%d): %v", i, err)
		}
		if got.PagesCount != int64(len(pages[chapter.(*MangaplusChapter).Id])) {
			t.Errorf("chapter %d has %d pages, want %d", i, got.PagesCount, len(pages[chapter.(*MangaplusChapter).Id]))
		}
		if len(got.Pages) != int(got.PagesCount) {
			t.Fatalf("chapter %d has %d pages but a count of %d", i, len(got.Pages), got.PagesCount)
		}

		for n, page := range got.Pages {
			if page.Number != int64(n+1) {
				t.Errorf("chapter %d page %d is numbered %d", i, n+1, page.Number)
			}
			if want := pages[chapter.(*MangaplusChapter).Id][n]; page.URL != want {
				t.Errorf("chapter %d page %d url = %q, want %q", i, n+1, page.URL, want)
			}
			if page.Headers["Plus-Vw-Token"] == "" {
				t.Fatalf("chapter %d page %d has no token", i, n+1)
			}
			// the cookie has to be the same token: the shared http session
			// may hold another response's (rejected) one
			// the cookie's name is the API's own contract: it's written out
			// instead of being read off the constant that produced it, so a
			// wrong one fails here rather than at every chapter download
			if want := "plus_vw_token=" + page.Headers["Plus-Vw-Token"]; page.Headers["Cookie"] != want {
				t.Errorf("chapter %d page %d cookie = %q, want %q", i, n+1, page.Headers["Cookie"], want)
			}
		}

		token := got.Pages[0].Headers["Plus-Vw-Token"]
		if tokens[token] {
			t.Errorf("chapter %d reuses another chapter's token %q", i, token)
		}
		tokens[token] = true

		// --bundle fetches every chapter twice: the second call must reuse the
		// cached viewer response instead of spending another rate limited call
		viewerCalls := srv.viewerCalls
		again, err := m.FetchChapter(chapter)
		if err != nil {
			t.Fatalf("second FetchChapter(%d): %v", i, err)
		}
		if srv.viewerCalls != viewerCalls {
			t.Errorf("chapter %d was fetched from the API %d times, want %d", i, srv.viewerCalls-viewerCalls, 0)
		}
		if again.Title != got.Title || again.PagesCount != got.PagesCount {
			t.Errorf("chapter %d differs between calls: %+v vs %+v", i, again, got)
		}
	}
}

// A reader URL doesn't carry the series it belongs to: that comes out of a
// viewer response, which is then reused for that chapter instead of being
// fetched again
func TestMangaplusViewerURLResolvesTitle(t *testing.T) {
	chapters := []mangaplusTestChapter{
		{id: 1023486, name: "#067", subTitle: "Chapter 67: Kyoto Bloodshed Hotel"},
		{id: 1023495, name: "#066", subTitle: "Chapter 66: Truth"},
	}
	pages := map[uint32][]string{
		1023486: {"https://assets.test/67/1.webp?hash=a"},
		1023495: {"https://assets.test/66/1.webp?hash=b"},
	}
	srv := newMangaplusTestServer(t, chapters, pages)
	m := mangaplusTestGrabber(t, srv, "https://mangaplus.shueisha.co.jp/viewer/1023486", "")

	if got, err := m.FetchTitle(); err != nil || got != "Test Series" {
		t.Fatalf("FetchTitle = %q, %v, want %q", got, err, "Test Series")
	}
	if srv.viewerCalls != 1 {
		t.Fatalf("resolving the title took %d viewer calls, want 1", srv.viewerCalls)
	}
	if got := srv.calls("/title_detailV3")[0].query.Get("title_id"); got != strconv.Itoa(mangaplusTestTitleID) {
		t.Errorf("title_id param = %q, want %d", got, mangaplusTestTitleID)
	}

	list, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}
	if len(list) != len(chapters) {
		t.Fatalf("got %d chapters, want %d", len(list), len(chapters))
	}

	// the chapter the URL pointed at is already cached
	if _, err := m.FetchChapter(list[0]); err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	if srv.viewerCalls != 1 {
		t.Errorf("the viewer response from the title lookup was fetched again (%d calls)", srv.viewerCalls)
	}

	// the other chapter still needs its own
	if _, err := m.FetchChapter(list[1]); err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}
	if srv.viewerCalls != 2 {
		t.Errorf("got %d viewer calls, want 2", srv.viewerCalls)
	}
}

// A viewer response without its plus_vw_token cookie can't be used: the token
// exists nowhere else (the app path's MangaViewer has no field of its own for
// it), and every page would fail as a bare 400 with nothing pointing here
func TestMangaplusViewerWithoutToken(t *testing.T) {
	chapters := []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}
	srv := newMangaplusTestServer(t, chapters, map[uint32][]string{101: {"https://assets.test/1/1.webp"}})
	srv.omitToken = true
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "")

	list, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}

	_, err := m.FetchChapter(list[0])
	if err == nil {
		t.Fatal("FetchChapter returned no error for a response without a token")
	}
	if !strings.Contains(err.Error(), mangaplusViewTokenCookie) {
		t.Errorf("FetchChapter error = %q, want it to name the missing cookie", err)
	}
}

// Every call carries the base params and the device secret; the one call that
// must not carry a secret is the registration itself
func TestMangaplusRequestParams(t *testing.T) {
	chapters := []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}
	pages := map[uint32][]string{101: {"https://assets.test/1/1.webp?hash=a"}}
	srv := newMangaplusTestServer(t, chapters, pages)
	m := mangaplusTestGrabber(t, srv, mangaplusTestTitleURL, "")

	list, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatalf("FetchChapters errors: %v", errs)
	}
	chapter, err := m.FetchChapter(list[0])
	if err != nil {
		t.Fatalf("FetchChapter: %v", err)
	}

	if chapter.PagesCount != 1 {
		t.Fatalf("got %d pages, want 1", chapter.PagesCount)
	}

	for _, request := range srv.requests {
		if request.agent != mangaplusUserAgent {
			t.Errorf("%s %s: user agent = %q, want %q", request.method, request.path, request.agent, mangaplusUserAgent)
		}
		for name, want := range map[string]string{"os": "android", "os_ver": "36", "app_ver": "250"} {
			if got := request.query.Get(name); got != want {
				t.Errorf("%s %s: %s = %q, want %q", request.method, request.path, name, got, want)
			}
		}
		if got := request.query.Get("secret"); got != strings.Repeat("f", 32) {
			t.Errorf("%s %s: secret = %q, want the configured one", request.method, request.path, got)
		}
	}

	if got := srv.calls("/manga_viewer_v3")[0].query.Get("viewer_mode"); got != "vertical" {
		t.Errorf("viewer_mode = %q, want %q", got, "vertical")
	}
}

// The logical failure envelope is expected over HTTP 200, but a front end in
// front of the API (or the API one day) may answer 4xx/429 with the same body:
// the API's own actionable message is what the user needs, not a bare status
// code, and anything that isn't an envelope still falls back to that code
func TestMangaplusRequestNon200(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         []byte
		wantMessage  string
		wantSentinel error
	}{
		{
			"a rate limited device, answered with 429",
			http.StatusTooManyRequests,
			mangaplusTestErrorResponse(mangaplusErrRateLimited),
			"10522",
			errMangaplusRateLimited,
		},
		{
			"a rejected secret, answered with 403",
			http.StatusForbidden,
			mangaplusTestErrorResponse(mangaplusErrUnknownSecret),
			"11012",
			errMangaplusDeviceSecret,
		},
		{
			"a 500 with something that isn't an envelope",
			http.StatusInternalServerError,
			[]byte("<html>502 Bad Gateway</html>"),
			"received 500 response code",
			nil,
		},
		{
			"a 503 with an empty body",
			http.StatusServiceUnavailable,
			nil,
			"received 503 response code",
			nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := c.status, c.body
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write(body)
			}))
			defer srv.Close()

			m := mangaplusTestGrabberAt(t, srv.URL, mangaplusTestTitleURL, "")

			_, err := m.apiGet("title_detailV3", nil)
			if err == nil {
				t.Fatal("apiGet returned no error")
			}
			if !strings.Contains(err.Error(), c.wantMessage) {
				t.Errorf("apiGet error = %q, want it to mention %q", err, c.wantMessage)
			}
			if c.wantSentinel != nil && !errors.Is(err, c.wantSentinel) {
				t.Errorf("apiGet error = %v, want it to wrap %v", err, c.wantSentinel)
			}
		})
	}
}

// A response past the cap is refused instead of silently truncated: half a
// protobuf message can still decode into a shorter, plausible one
func TestMangaplusRequestBodyCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte{0x0a}, mangaplusMaxResponseSize+1))
	}))
	defer srv.Close()

	// the payload above is built from the constant, so the value itself has to
	// be pinned somewhere: a few MB is what a real response needs (a
	// 1193-chapter title is a few hundred KB), not just whatever the constant
	// happens to say
	if mangaplusMaxResponseSize != 16<<20 {
		t.Errorf("mangaplusMaxResponseSize = %d, want %d", mangaplusMaxResponseSize, 16<<20)
	}

	m := mangaplusTestGrabberAt(t, srv.URL, mangaplusTestTitleURL, "")

	_, err := m.apiGet("title_detailV3", nil)
	if err == nil {
		t.Fatal("apiGet returned no error for an oversized response")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(mangaplusMaxResponseSize)) {
		t.Errorf("apiGet error = %q, want it to name the %d byte cap", err, mangaplusMaxResponseSize)
	}
}

// A stalled response must not hang the whole run
func TestMangaplusRequestTimeout(t *testing.T) {
	// the constructor's own client is where the timeout lives, and this test
	// overrides it below, so it's only observable on one that was left alone
	untouched := NewMangaplus(&Grabber{URL: mangaplusTestTitleURL, Settings: &Settings{}})
	if got := untouched.client.Timeout; got != mangaplusRequestTimeout {
		t.Errorf("client timeout = %s, want %s", got, mangaplusRequestTimeout)
	}
	// and it has to be a timeout worth having: long enough for a slow response
	// to arrive, short enough to give up on one that never will
	if mangaplusRequestTimeout < 10*time.Second {
		t.Errorf("mangaplusRequestTimeout = %s, want a whole call to fit in it", mangaplusRequestTimeout)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	m := mangaplusTestGrabberAt(t, srv.URL, mangaplusTestTitleURL, "")
	m.client.Timeout = 30 * time.Millisecond

	start := time.Now()
	if _, err := m.apiGet("title_detailV3", nil); err == nil {
		t.Fatal("apiGet returned no error for a stalled response")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("apiGet waited %s, want it to give up on the client's timeout", elapsed)
	}
}

// Without a user config dir the secret must not be cached somewhere shared
// (os.TempDir): it would hand the device to whoever else can read that.
//
// Every variable os.UserConfigDir reads is cleared, on every platform: it's
// $HOME or $XDG_CONFIG_HOME on Unix and %AppData% on Windows, and clearing
// only the Unix ones would leave the test failing for a Windows contributor.
func TestMangaplusSecretPathWithoutConfigDir(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	t.Setenv(mangaplusSecretEnv, "")

	path, err := mangaplusSecretPath()
	if err == nil {
		t.Errorf("mangaplusSecretPath = %q with no error, want an error without a user config dir", path)
	} else {
		// the user is told what a config dir is wanted for, and that the
		// secret can be handed over instead
		if !strings.Contains(err.Error(), mangaplusSecretEnv) {
			t.Errorf("mangaplusSecretPath error = %q, want it to name %s", err, mangaplusSecretEnv)
		}
		if !strings.Contains(err.Error(), "device secret") {
			t.Errorf("mangaplusSecretPath error = %q, want it to say what the config dir is for", err)
		}
		// the underlying error is kept, not replaced
		if errors.Unwrap(err) == nil {
			t.Errorf("mangaplusSecretPath error = %q, want it to wrap the config dir error", err)
		}
	}

	m := NewMangaplus(&Grabber{URL: mangaplusTestTitleURL, Settings: &Settings{}})
	if _, err := m.deviceSecret(); err == nil {
		t.Error("deviceSecret returned no error without a user config dir to register into")
	} else if !strings.Contains(err.Error(), mangaplusSecretEnv) {
		t.Errorf("deviceSecret error = %q, want it to name %s", err, mangaplusSecretEnv)
	}
}

// The anonymous device secret comes from MANGAPLUS_SECRET, then from the file a
// previous run cached, and otherwise from registering a new device with the API
func TestMangaplusDeviceSecret(t *testing.T) {
	const (
		envSecret        = "11111111111111111111111111111111"
		cachedSecret     = "22222222222222222222222222222222"
		registeredSecret = "33333333333333333333333333333333"
	)

	cases := []struct {
		name          string
		env           string
		cached        string
		wantSecret    string
		wantRegisters int
	}{
		{"MANGAPLUS_SECRET wins", envSecret, cachedSecret, envSecret, 0},
		{"a cached secret is reused", "", cachedSecret, cachedSecret, 0},
		{"an implausible cached secret is ignored", "", "not-a-secret", registeredSecret, 1},
		{"no secret at all registers a device", "", "", registeredSecret, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("XDG_CONFIG_HOME", dir)
			t.Setenv(mangaplusSecretEnv, c.env)
			if c.cached != "" {
				path, err := mangaplusSecretPath()
				if err != nil {
					t.Fatalf("mangaplusSecretPath: %v", err)
				}
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatalf("creating the config dir: %v", err)
				}
				if err := os.WriteFile(path, []byte(c.cached+"\n"), 0600); err != nil {
					t.Fatalf("caching a secret: %v", err)
				}
			}

			srv := newMangaplusTestServer(t, []mangaplusTestChapter{{id: 101, name: "#001", subTitle: "Chapter 1: Mission"}}, nil)
			srv.secret = registeredSecret
			mangaplusTestAPI(t, srv.URL, time.Millisecond)
			m := NewMangaplus(&Grabber{URL: mangaplusTestTitleURL, Settings: &Settings{}})

			if got, err := m.FetchTitle(); err != nil || got != "Test Series" {
				t.Fatalf("FetchTitle = %q, %v, want %q", got, err, "Test Series")
			}

			registers := srv.calls("/register")
			if len(registers) != c.wantRegisters {
				t.Fatalf("got %d registrations, want %d", len(registers), c.wantRegisters)
			}

			if got := srv.calls("/title_detailV3")[0].query.Get("secret"); got != c.wantSecret {
				t.Errorf("secret sent = %q, want %q", got, c.wantSecret)
			}

			if c.wantRegisters == 1 {
				// registration carries no secret, only the device it wants
				// registered
				register := registers[0]
				if register.method != "PUT" {
					t.Errorf("registration used %s, want PUT", register.method)
				}
				if got := register.query.Get("secret"); got != "" {
					t.Errorf("registration sent a secret (%q), it must not", got)
				}
				if got := register.query.Get("device_token"); !mangaplusSecretRe.MatchString(got) {
					t.Errorf("device_token = %q, want 32 hex characters", got)
				}
				// the salt is part of the app itself, so it's written out here:
				// a wrong one would still be accepted by the fake API, and only
				// the real one could tell
				wantKey := md5Hex(register.query.Get("device_token") + "4Kin9vGg")
				if got := register.query.Get("security_key"); got != wantKey {
					t.Errorf("security_key = %q, want md5(device_token + salt) = %q", got, wantKey)
				}
				if got := register.query.Get("security_key"); got == register.query.Get("device_token") {
					t.Error("the security key is the device token itself")
				}

				// the registered secret is cached for the next run
				path, err := mangaplusSecretPath()
				if err != nil {
					t.Fatalf("mangaplusSecretPath: %v", err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("reading the cached secret: %v", err)
				}
				if got := strings.TrimSpace(string(data)); got != registeredSecret {
					t.Errorf("cached secret = %q, want %q", got, registeredSecret)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatalf("statting the cached secret: %v", err)
				}
				if perm := info.Mode().Perm(); perm != 0600 {
					t.Errorf("cached secret permissions = %v, want %v", perm, os.FileMode(0600))
				}
			}
		})
	}
}

// The API locks a device out when it's called too often, so calls are spaced
// out instead of fired in a burst
func TestMangaplusRateLimiter(t *testing.T) {
	const (
		interval = 30 * time.Millisecond
		calls    = 5
	)

	srv := newMangaplusTestServer(t, nil, nil)
	srv.secret = strings.Repeat("a", 32)
	mangaplusTestAPI(t, srv.URL, interval)
	t.Setenv(mangaplusSecretEnv, strings.Repeat("f", 32))
	m := NewMangaplus(&Grabber{URL: mangaplusTestTitleURL, Settings: &Settings{}})

	times := make([]time.Time, 0, calls)
	for i := 0; i < calls; i++ {
		if _, err := m.apiGet("title_detailV3", nil); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		times = append(times, time.Now())
	}

	// the first call may take a tick the ticker had already buffered, so it's
	// the gaps between the calls after it that are guaranteed
	for i := 2; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap < interval*3/4 {
			t.Errorf("calls %d and %d were %s apart, want at least %s", i, i+1, gap, interval*3/4)
		}
	}
}

// mangaplusTestDecodeResponse decodes a response body the way the grabber's
// request does, so the tests go through the same envelope field numbers
func mangaplusTestDecodeResponse(t *testing.T, body []byte) *mangaplusAPIResponse {
	t.Helper()

	response := &mangaplusAPIResponse{}
	if err := response.decode(body); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}

	return response
}
