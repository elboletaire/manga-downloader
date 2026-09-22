// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
)

// The app API's endpoint base. Every call is a GET (or a PUT, for the one
// registration call) with all of its arguments in the query string.
//
// It's a package-level var, unexported and never set outside of tests, only so
// the tests can point it at a local server.
var mangaplusAPIBase = "https://jumpg-api.tokyo-cdn.com/api"

// mangaplusRequestInterval is the minimum time between two calls to the app
// API. It starts refusing calls (and locking the whole device out for ten or
// more minutes) after roughly ten requests in a few seconds, so they're spaced
// out instead of retried. A package-level var for the same reason as the base
// URL: tests shrink it instead of sleeping for seconds.
var mangaplusRequestInterval = 500 * time.Millisecond

// The fixed query params every call carries: the API refuses to answer without
// them, and they're what tells it which client it's talking to (which is also
// why the Android client's platform is the one that gets the complete chapter
// list).
const (
	mangaplusPlatform   = "android"
	mangaplusOSVersion  = "36"
	mangaplusAppVersion = "250"
	mangaplusUserAgent  = "okhttp/4.12.0"
)

// mangaplusSecurityKeySalt is the constant the app API derives a new device's
// security key with (security_key = md5(device_token + salt)); the salt is part
// of the app itself, not of its configuration.
const mangaplusSecurityKeySalt = "4Kin9vGg"

// mangaplusSecretEnv is the environment variable that overrides the device
// secret, so one registered elsewhere (another install, another machine) can be
// reused instead of registering a new device on every run
const mangaplusSecretEnv = "MANGAPLUS_SECRET"

// mangaplusViewTokenCookie is the cookie a viewer response sets, and the header
// its image URLs are signed for
const mangaplusViewTokenCookie = "plus_vw_token"

// mangaplusRequestTimeout is the timeout of every call to the app API: without
// one a stalled response would hang the whole run
const mangaplusRequestTimeout = 60 * time.Second

// mangaplusMaxResponseSize caps how much of a response body is read. Real ones
// are a few hundred KB (a 1193-chapter title), so this is only here to keep a
// broken or hostile answer from eating all the memory — and reaching it is an
// error, never a silent truncation (half a protobuf message can still decode
// into a shorter, plausible one).
const mangaplusMaxResponseSize = 16 << 20

// mangaplusMaxErrorBodySize caps the body read to explain a non-200 response:
// the error envelope it may carry is small, and anything past this isn't one
const mangaplusMaxErrorBodySize = 64 << 10

// mangaplusChapterTypeStandard is the chapter_type value at which a chapter
// stops being free to read (FREE is 0, FREE_FOR_FIRST_TIME 1, STANDARD 2 and
// DELUXE 3)
const mangaplusChapterTypeStandard = 2

// mangaplusFreeWindowChapters is how many chapters an edition that carries only
// the site's free window lists: an edition of a serializing title that isn't
// the one its whole run is free in reads as this many chapters, which is what
// detailLocked notices and tells the user about (the count is never a filter)
const mangaplusFreeWindowChapters = 6

// Field numbers of the app API's protobuf schema, as used below. Only the ones
// this grabber reads are listed.
const (
	// Response.success -> SuccessResult
	mangaplusSuccessField = 1
	// Response.error -> ErrorResult
	mangaplusErrorField = 2
	// SuccessResult.registeration_data -> RegistrationData (the API's own
	// spelling); its device_secret(1) is the secret every later call needs
	mangaplusRegistrationDataField = 2
	// SuccessResult.title_detail_view -> TitleDetailView
	mangaplusTitleDetailViewField = 8
	// SuccessResult.manga_viewer -> MangaViewer
	mangaplusViewerField = 10
	// MangaViewer.pages -> Page, whose manga_page(1) is the only variant of its
	// oneof that holds an image
	mangaplusViewerPagesField   = 1
	mangaplusPageMangaPageField = 1
	// TitleDetailView.title_languages, one entry per language the title is
	// published in, each with the title_id of that language's own edition (see
	// mangaplusTitleLanguage)
	mangaplusTitleLanguagesField = 27
	// TitleDetailView.chapter_list_v2, the complete and ascending chapter list
	mangaplusChapterListV2Field = 38
	// TitleDetailView.chapter_list_group, the older partial chapter list
	mangaplusChapterListGroupField = 28
	// ChapterGroup.first_chapter_list / mid_chapter_list / last_chapter_list
	mangaplusChapterGroupFirstField = 2
	mangaplusChapterGroupMidField   = 3
	mangaplusChapterGroupLastField  = 4
	// RegistrationData.device_secret
	mangaplusRegistrationSecretField = 1
	// ErrorResult.english_popup -> ErrorPopup, whose subject(1) and body(2)
	// carry the message; the same popup is repeated once per language in
	// ErrorResult.localized_popup (5)
	mangaplusErrorPopupField          = 2
	mangaplusErrorLocalizedPopupField = 5
	mangaplusErrorPopupSubjectField   = 1
	mangaplusErrorPopupBodyField      = 2
)

// The numeric codes the app API appends to its error popups, e.g.
// "...Please try again later.(10521)". They're the only way to tell its
// failures apart: the message itself is generic.
const (
	// mangaplusErrUnknownSecret is answered when the secret sent isn't one the
	// API ever issued, which is what a stale cached secret — or a
	// MANGAPLUS_SECRET belonging to another edition's API — hits
	mangaplusErrUnknownSecret = "11012"
	// mangaplusErrMissingSecret is answered when the secret parameter is missing
	// altogether, which this grabber never does: it's classified together with
	// the one above because the fix is the same one for the user
	mangaplusErrMissingSecret = "10521"
	// mangaplusErrRateLimited is answered when too many calls were made: the
	// whole device is locked out for ten or more minutes
	mangaplusErrRateLimited = "10522"
	// mangaplusErrSubscription and its alternate are answered for a chapter
	// that can't be read without a Manga Plus MAX subscription
	mangaplusErrSubscription    = "11301"
	mangaplusErrSubscriptionAlt = "11302"
)

// errMangaplusSubscription is returned for a chapter the API refuses to serve
// without a Manga Plus MAX subscription. Reading a chapter once for free flips
// it from FREE_FOR_FIRST_TIME to STANDARD while leaving it readable, so the
// chapter type is deliberately not used to predict this: only the API's own
// refusal is.
var errMangaplusSubscription = errors.New("this chapter requires a Manga Plus MAX subscription")

// errMangaplusDeviceSecret is returned when the API rejects the device secret,
// which is a local problem (a stale cached secret, or a MANGAPLUS_SECRET that
// belongs to another region's API) rather than a chapter one
var errMangaplusDeviceSecret = errors.New("the MangaPlus API rejected the device secret")

// errMangaplusRateLimited is returned when the API's rate limiter has locked
// the device out: too many calls in a short time cost it a ten-or-more-minute
// lockout, which no retry can shorten or work around
var errMangaplusRateLimited = errors.New("the MangaPlus API is rate limiting this device, which locks it out for about ten minutes: retry after that, not sooner")

// mangaplusURLTitle and mangaplusURLViewer are the two kinds of page the
// grabber accepts: a series page, and a reader page of one chapter
const (
	mangaplusURLTitle  = "titles"
	mangaplusURLViewer = "viewer"
)

// mangaplusSecretRe matches the device secrets the API issues, which are always
// 32 lowercase hex characters: anything else (an empty MANGAPLUS_SECRET, a
// truncated cached file) is treated as absent instead of being sent
var mangaplusSecretRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// mangaplusDefaultLanguage is the edition a title's own language enum reads as
// when it carries a value the API's list doesn't know (the enum's first value
// is English)
const mangaplusDefaultLanguage = "eng"

// mangaplusLanguages maps the 2-letter codes the --language flag takes to the
// ISO 639-2/T codes MangaPlus names its editions with
var mangaplusLanguages = map[string]string{
	"en": "eng",
	"es": "esp",
	"fr": "fra",
	"id": "ind",
	"pt": "ptb",
	"ru": "rus",
	"th": "tha",
	"de": "deu",
	"it": "ita",
	"vi": "vie",
}

// mangaplusLanguageCodes maps the Title.language enum to its code, ordered as
// the API's schema declares it
var mangaplusLanguageCodes = map[uint64]string{
	0: "eng",
	1: "esp",
	2: "fra",
	3: "ind",
	4: "ptb",
	5: "rus",
	6: "tha",
	7: "deu",
	8: "ita",
	9: "vie",
}

// MangaPlus (mangaplus.shueisha.co.jp) is Shueisha's official reader, so every
// chapter it publishes is the official release, in one edition per language.
//
// It needs its own grabber because of how it serves chapters: the API the
// website itself uses (jumpg-webapi) only lists a window of the chapter list
// and paywalls the middle of a running series, while the *app* API
// (jumpg-api) returns the complete list — and for a still-serializing title
// every chapter of it is free to read. That API is protobuf-only (see
// protobuf.go) and authentically anonymous: it wants a device secret that any
// client can register for itself on first launch, which is what
// deviceSecret() does.
//
// Two of its properties shape the code below:
//
//   - It rate limits hard. Roughly ten requests in a few seconds earn error
//     10522 and a ten-minute lockout for the whole device, so each call waits
//     on m.rateLimiter, spaced mangaplusRequestInterval apart.
//   - A viewer response mints a fresh plus_vw_token bound to the signed image
//     URLs *of that response* (no token: HTTP 400; another response's token:
//     HTTP 403). The token therefore travels with every page (Page.Headers)
//     instead of in the shared http session, which concurrent chapters would
//     clobber, and each chapter's viewer response is cached because --bundle
//     fetches every chapter twice.
type Mangaplus struct {
	*Grabber
	// rateLimiter spaces out every call to the app API
	rateLimiter <-chan time.Time
	// client is the HTTP client every call is made with, whose timeout is what
	// keeps a stalled response from hanging the whole run. It's a client of our
	// own rather than the shared http helper because a viewer response's
	// plus_vw_token only exists in a Set-Cookie header: the helper returns the
	// body alone, and the token would end up in a session shared by the
	// chapters downloading in parallel
	client *http.Client
	// rateLimited records that the API answered 10522. The device is then
	// locked out for ten or more minutes, so no further call is made: each one
	// would be refused, and would keep feeding the limiter. It's read from the
	// request path, which already holds mu, hence an atomic instead of a field
	// guarded by it
	rateLimited atomic.Bool
	// mu guards the lazily built state below. Chapters are fetched
	// concurrently, and a viewer call must not be interleaved with another
	// one (each would otherwise pay a rate limit tick for nothing, and the
	// chapter language it reports would race)
	mu sync.Mutex
	// title is the series title, as reported by the API
	title string
	// titleId is the title_id every call is made for. A title URL carries it
	// directly; a viewer URL only reveals it through one viewer response
	titleId uint32
	// detail is the decoded title_detailV3 response
	detail *mangaplusTitleDetail
	// language is the 3-letter code of the edition being downloaded, as the
	// edition's own title detail reports it — which --language may have resolved
	// to another edition's title_id — and what the viewer calls ask for. It's
	// empty until a title detail was fetched
	language string
	// editionResolved records that the edition --language asks for was already
	// resolved, so the refetch that may take happens at most once per grabber,
	// however many callers fetch the title detail
	editionResolved bool
	// chapters is the chapter list built by FetchChapters
	chapters Filterables
	// viewers caches each chapter's decoded viewer response by chapter_id
	viewers map[uint32]*mangaplusViewer
	// secretMu guards secret alone: it's resolved from inside the request
	// path, which the other mutex may already be held by
	secretMu sync.Mutex
	// secret is the anonymous device secret the app API authenticates with
	secret string
}

// NewMangaplus returns a new Mangaplus site grabber
func NewMangaplus(g *Grabber) *Mangaplus {
	return &Mangaplus{
		Grabber:     g,
		rateLimiter: time.Tick(mangaplusRequestInterval),
		client:      &http.Client{Timeout: mangaplusRequestTimeout},
		viewers:     map[uint32]*mangaplusViewer{},
	}
}

// MangaplusChapter represents a Mangaplus chapter
type MangaplusChapter struct {
	Chapter
	// Id is the app API's chapter_id
	Id uint32
	// SubTitle is the API's long chapter name, e.g. "Chapter 1: Romance Dawn"
	SubTitle string
	// Name is the API's short chapter name, e.g. "#001"
	Name string
}

// Test returns true for the site's series and reader URLs. It matches the URL
// alone: starting a request here would cost a rate limit tick (and the app API
// lockout is easy to hit) just to tell whether the URL is ours.
func (m *Mangaplus) Test() (bool, error) {
	_, _, ok := mangaplusURL(m.URL)

	return ok, nil
}

// FetchTitle returns the series title
func (m *Mangaplus) FetchTitle() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.title != "" {
		return m.title, nil
	}

	detail, err := m.detailLocked()
	if err != nil {
		return "", err
	}

	m.title = sanitizeTitle(detail.Title.Name)

	return m.title, nil
}

// FetchChapters returns the chapters of the series
func (m *Mangaplus) FetchChapters() (chapters Filterables, errs []error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.chapters != nil {
		return m.chapters, nil
	}

	detail, err := m.detailLocked()
	if err != nil {
		return nil, []error{err}
	}

	// The API's chapter list is complete and in ascending order, but its short
	// name isn't always a number ("ex", "One-shot") and the whole list is
	// renumbered by Filterables.SortByNumber, so the numbers are derived in
	// order (mangaplusChapterNumber) instead of parsed independently.
	previous := -0.1
	locked := 0

	for _, chapter := range detail.Chapters {
		number := mangaplusChapterNumber(chapter.name, previous)
		previous = number

		// This is a count for the user, not a filter: a locked chapter is only
		// refused when the API itself refuses to serve it (see FetchChapter).
		if chapter.locked() {
			locked++
		}

		chapters = append(chapters, &MangaplusChapter{
			Chapter: Chapter{
				Number:   number,
				Title:    mangaplusChapterTitle(chapter.subTitle, chapter.name, number),
				Language: m.language,
			},
			Id:       chapter.chapterID,
			SubTitle: chapter.subTitle,
			Name:     chapter.name,
		})
	}

	if locked > 0 {
		color.Yellow("%d of %d chapters need a Manga Plus MAX subscription and can't be downloaded", locked, len(chapters))
	}

	m.chapters = chapters

	return chapters, nil
}

// FetchChapter fetches a chapter and its pages
func (m *Mangaplus) FetchChapter(f Filterable) (*Chapter, error) {
	chap := f.(*MangaplusChapter)

	m.mu.Lock()
	defer m.mu.Unlock()

	viewer, err := m.viewerLocked(chap.Id)
	if err != nil {
		return nil, err
	}

	// The image URLs are signed and the token they need is minted for them,
	// so both travel together with each page. Sending it as a header *and* as
	// a cookie keeps them consistent: the http package writes the shared
	// session cookie before applying per-request headers, so an explicit
	// Cookie overrides a token harvested from some other response.
	headers := map[string]string{
		"Plus-Vw-Token": viewer.token,
		"Cookie":        mangaplusViewTokenCookie + "=" + viewer.token,
	}

	chapter := &Chapter{
		Title:      mangaplusChapterTitle(chap.SubTitle, chap.Name, chap.Number),
		Number:     chap.Number,
		PagesCount: int64(len(viewer.pages)),
		Language:   m.language,
	}

	for i, pageURL := range viewer.pages {
		chapter.Pages = append(chapter.Pages, Page{
			Number:  int64(i + 1),
			URL:     pageURL,
			Headers: headers,
		})
	}

	return chapter, nil
}

// titleIDLocked returns the title_id to query the API with, resolving it from
// a reader URL when needed (a viewer response carries the title it belongs to).
// It must be called with m.mu held.
func (m *Mangaplus) titleIDLocked() (uint32, error) {
	if m.titleId != 0 {
		return m.titleId, nil
	}

	kind, id, ok := mangaplusURL(m.URL)
	if !ok {
		return 0, fmt.Errorf("invalid MangaPlus url %q", m.URL)
	}

	if kind == mangaplusURLViewer {
		viewer, err := m.viewerLocked(id)
		if err != nil {
			return 0, err
		}
		if viewer.titleID == 0 {
			return 0, fmt.Errorf("could not tell which series chapter %d belongs to", id)
		}
		m.titleId = viewer.titleID
		return m.titleId, nil
	}

	m.titleId = id

	return m.titleId, nil
}

// detailLocked returns the decoded title_detailV3 response of the edition being
// downloaded, fetching it once per grabber. It must be called with m.mu held.
func (m *Mangaplus) detailLocked() (*mangaplusTitleDetail, error) {
	if m.detail != nil {
		return m.detail, nil
	}

	// The edition the URL points at — or, for a reader URL, the one the viewer
	// response's title_id pins. Neither lang nor clang is sent for it: they
	// can't switch editions (only title_id does that), so asking for a language
	// the edition isn't would be ignored anyway, and with no --language the
	// edition the URL points at is the one to download.
	titleID, err := m.titleIDLocked()
	if err != nil {
		return nil, err
	}

	detail, err := m.fetchDetail(titleID)
	if err != nil {
		return nil, err
	}

	requested := mangaplusLanguage(m.GetPreferredLanguage())
	if requested != "" && !m.editionResolved {
		// The flag is what keeps the switch below from happening more than
		// once, whichever caller fetches the detail first
		m.editionResolved = true

		if detail, err = m.resolveEditionLocked(detail, requested); err != nil {
			return nil, err
		}
	}

	m.detail = detail
	// The edition's own answer is authoritative: it's the edition every later
	// call is made for, and its language is what the viewer calls ask for
	m.language = detail.Title.Language

	if requested == "" {
		// More than one edition exists and the one being downloaded carries no
		// more than the free window's worth of chapters: that's what a URL
		// pointing at one of a series' restricted editions looks like. What the
		// other editions list isn't claimed — finding that out would cost one
		// rate limited call each — only that they're separate downloads and
		// --language is the way to one.
		if len(detail.Languages) > 1 && len(detail.Chapters) <= mangaplusFreeWindowChapters {
			color.Yellow(
				"This MangaPlus edition lists only %d chapters (each language is a separate edition: %s): use --language to pick another",
				len(detail.Chapters), strings.Join(detail.languageCodes(), ", "),
			)
		}
	}

	return m.detail, nil
}

// fetchDetail requests the title detail of one edition and decodes it. That
// edition is pinned by title_id alone: the lang and clang params the API also
// takes can't change which one is answered with (see detailLocked).
func (m *Mangaplus) fetchDetail(titleID uint32) (*mangaplusTitleDetail, error) {
	response, err := m.apiGet("title_detailV3", url.Values{
		"title_id": {strconv.FormatUint(uint64(titleID), 10)},
	})
	if err != nil {
		return nil, err
	}

	views := pbRepeated(response.success, mangaplusTitleDetailViewField)
	if len(views) == 0 {
		return nil, errors.New("the MangaPlus API returned no title data")
	}

	return decodeMangaplusTitleDetail(views[0].b)
}

// resolveEditionLocked returns the title detail of the edition --language asks
// for, refetching it when the edition fetched above isn't that one. It must be
// called with m.mu held, with the detail of the edition the URL points at.
//
// MangaPlus keeps one edition per language, each with its own title_id, and
// their chapter lists aren't the same — the same series can list its whole run
// in one language and only the free window in another. Nothing but the
// title_id selects between them, so the wanted edition's id is looked up in
// the titleLanguages the API reports, and its own detail fetched in place of
// the URL's.
func (m *Mangaplus) resolveEditionLocked(detail *mangaplusTitleDetail, requested string) (*mangaplusTitleDetail, error) {
	if detail.Title.Language == requested {
		// the URL already points at the edition that was asked for
		return detail, nil
	}

	titleID, ok := detail.editionTitleID(requested)
	if !ok {
		// The title isn't published in the language that was asked for, so the
		// URL's edition is kept. What's said stays factual: the languages the
		// API does list, never that the title is published in one language
		// only (the URL's own edition is proof of the opposite)
		color.Yellow(
			"MangaPlus doesn't publish %s in %s (available: %s); keeping the %s edition from the URL",
			detail.Title.Name, requested, strings.Join(detail.languageCodes(), ", "), detail.Title.Language,
		)

		return detail, nil
	}

	color.Yellow(
		"MangaPlus keeps one edition per language: downloading the %s edition of %s instead of the %s one",
		requested, detail.Title.Name, detail.Title.Language,
	)

	switched, err := m.fetchDetail(titleID)
	if err != nil {
		return nil, err
	}

	// every later call is made for the edition that's being downloaded
	m.titleId = titleID

	return switched, nil
}

// viewerLocked returns the decoded manga_viewer_v3 response of a chapter,
// fetching it once. It must be called with m.mu held.
func (m *Mangaplus) viewerLocked(chapterID uint32) (*mangaplusViewer, error) {
	if viewer, ok := m.viewers[chapterID]; ok {
		return viewer, nil
	}

	params := url.Values{
		"chapter_id":           {strconv.FormatUint(uint64(chapterID), 10)},
		"split":                {"no"},
		"img_quality":          {"super_high"},
		"ticket_reading":       {"no"},
		"free_reading":         {"no"},
		"subscription_reading": {"yes"},
		"viewer_mode":          {"vertical"},
	}
	// clang is sent only once the edition's language is known, and left out
	// otherwise: the API answers fine without it, while a default would be a
	// guess at an edition the title_id already pins
	if clang := m.clangLocked(); clang != "" {
		params.Set("clang", clang)
	}

	response, err := m.apiGet("manga_viewer_v3", params)
	if err != nil {
		return nil, err
	}

	viewer, err := decodeMangaplusViewer(response.success, response.token)
	if err != nil {
		return nil, err
	}
	if len(viewer.pages) == 0 {
		// A successful response with no image page means the API handed out
		// banners and an ad only: better to fail the chapter than to write an
		// empty archive as if it had worked.
		return nil, fmt.Errorf("the MangaPlus API returned no pages for chapter %d", chapterID)
	}
	if viewer.token == "" {
		// The image URLs are signed for a token that only exists in the
		// response's Set-Cookie header (the app path has no token field of its
		// own): without it every page would come back a bare 400, with nothing
		// pointing back here.
		return nil, fmt.Errorf("the MangaPlus API returned no %s cookie for chapter %d, which its image URLs are signed for", mangaplusViewTokenCookie, chapterID)
	}

	m.viewers[chapterID] = viewer

	return viewer, nil
}

// clangLocked returns the language code to send with viewer calls: the one the
// edition being downloaded reports, or an empty code while that isn't known yet
// (which is the case for the viewer call a reader URL needs to resolve the
// series from — the API answers without it, and a default would guess at an
// edition the title_id already pins). It must be called with m.mu held.
func (m *Mangaplus) clangLocked() string {
	return m.language
}

// apiGet performs an authenticated, rate-limited GET against the app API
func (m *Mangaplus) apiGet(path string, params url.Values) (*mangaplusAPIResponse, error) {
	secret, err := m.deviceSecret()
	if err != nil {
		return nil, err
	}

	return m.request("GET", path, params, secret)
}

// request performs one call against the app API. Every call carries the base
// params (platform and app version, which is what tells the API which client
// it's talking to) plus the device secret — the one call that mustn't, because
// it's the one that hands the secret out, is registration.
func (m *Mangaplus) request(method, path string, params url.Values, secret string) (*mangaplusAPIResponse, error) {
	// A rate limited device stays rate limited for ten or more minutes, so
	// every call after the first 10522 would be refused (and would keep feeding
	// the limiter): none is made at all
	if m.rateLimited.Load() {
		return nil, errMangaplusRateLimited
	}

	query := url.Values{
		"os":      {mangaplusPlatform},
		"os_ver":  {mangaplusOSVersion},
		"app_ver": {mangaplusAppVersion},
	}
	if secret != "" {
		query.Set("secret", secret)
	}
	for name, values := range params {
		for _, value := range values {
			query.Add(name, value)
		}
	}

	<-m.rateLimiter

	req, err := http.NewRequest(method, mangaplusAPIBase+"/"+path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mangaplusUserAgent)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Every logical failure this API reports is an HTTP 200 carrying its
		// error envelope, but a front end in front of it (or the API itself,
		// one day) may answer 4xx/429 with the same body: decode it, so the
		// user gets the API's own actionable message instead of a bare status
		// code. Anything that isn't an envelope falls back to that status.
		envelope := &mangaplusAPIResponse{}
		var apiErr *mangaplusAPIError
		if body, readErr := io.ReadAll(io.LimitReader(resp.Body, mangaplusMaxErrorBodySize)); readErr == nil {
			if errors.As(envelope.decode(body), &apiErr) {
				return nil, m.explainAPIError(apiErr)
			}
		}

		return nil, fmt.Errorf("received %d response code", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, mangaplusMaxResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > mangaplusMaxResponseSize {
		return nil, fmt.Errorf("the MangaPlus API answered with more than %d bytes", mangaplusMaxResponseSize)
	}

	response := &mangaplusAPIResponse{}
	// A viewer response mints the token its own signed image URLs demand.
	// Reading it off this very response (instead of off a shared cookie jar)
	// is what keeps concurrent chapters from borrowing each other's token,
	// which the API rejects with a 403.
	for _, cookie := range resp.Cookies() {
		if cookie.Name == mangaplusViewTokenCookie {
			response.token = cookie.Value
		}
	}

	if err = response.decode(body); err != nil {
		return nil, m.explainAPIError(err)
	}

	return response, nil
}

// deviceSecret returns the anonymous device secret every API call is
// authenticated with. It's taken from MANGAPLUS_SECRET when set, then from the
// secret registered on a previous run, and otherwise registered now (which is
// what the app itself does on a first launch) so no account is ever needed.
func (m *Mangaplus) deviceSecret() (string, error) {
	m.secretMu.Lock()
	defer m.secretMu.Unlock()

	if m.secret != "" {
		return m.secret, nil
	}

	if secret := strings.TrimSpace(os.Getenv(mangaplusSecretEnv)); mangaplusSecretRe.MatchString(secret) {
		m.secret = secret
		return m.secret, nil
	}

	path, err := mangaplusSecretPath()
	if err != nil {
		return "", err
	}

	if cached, err := os.ReadFile(path); err == nil {
		if secret := strings.TrimSpace(string(cached)); mangaplusSecretRe.MatchString(secret) {
			m.secret = secret
			return m.secret, nil
		}
	}

	secret, err := m.registerDevice()
	if err != nil {
		return "", err
	}
	m.secret = secret

	// Say it out loud: registering a device is a side effect the user can't
	// see otherwise, and it's the only thing that makes the next run reuse it.
	if err = mangaplusStoreSecret(path, secret); err != nil {
		color.Yellow("registered a new MangaPlus device, but could not store its secret in %s (it will be registered again on the next run): %s", path, err)
	} else {
		color.Yellow("registered a new MangaPlus device, whose secret is stored in %s", path)
	}

	return m.secret, nil
}

// registerDevice registers an anonymous device with the app API. The API wants
// nothing but an md5-derived device identifier (the Android client's
// android_id, which is 8 random bytes) and a matching security key derived out
// of it, and answers with the secret every later call has to carry.
func (m *Mangaplus) registerDevice() (string, error) {
	androidID := make([]byte, 8)
	if _, err := rand.Read(androidID); err != nil {
		return "", err
	}

	deviceToken := md5Hex(hex.EncodeToString(androidID))
	response, err := m.request("PUT", "register", url.Values{
		"device_token": {deviceToken},
		"security_key": {md5Hex(deviceToken + mangaplusSecurityKeySalt)},
	}, "")
	if err != nil {
		return "", err
	}

	// Response.success(1) -> SuccessResult.registeration_data(2) ->
	// RegistrationData.device_secret(1)
	registrations := pbRepeated(response.success, mangaplusRegistrationDataField)
	if len(registrations) == 0 {
		return "", errors.New("the MangaPlus API returned no registration data")
	}
	fields, err := pbMessage(registrations[0])
	if err != nil {
		return "", err
	}

	secret := pbString(fields, mangaplusRegistrationSecretField)
	if !mangaplusSecretRe.MatchString(secret) {
		return "", fmt.Errorf("the MangaPlus API returned an unexpected device secret %q", secret)
	}

	return secret, nil
}

// explainAPIError turns an API error envelope into the error the user gets.
// The API reports every failure as a popup, so the numeric code it appends to
// the popup body is what tells the actionable cases apart.
func (m *Mangaplus) explainAPIError(err error) error {
	var apiErr *mangaplusAPIError
	if !errors.As(err, &apiErr) {
		return err
	}

	switch {
	case apiErr.hasCode(mangaplusErrSubscription), apiErr.hasCode(mangaplusErrSubscriptionAlt):
		return fmt.Errorf("%w: %s", errMangaplusSubscription, apiErr.Error())
	case apiErr.hasCode(mangaplusErrUnknownSecret), apiErr.hasCode(mangaplusErrMissingSecret):
		return fmt.Errorf("%w: %s (%s)", errMangaplusDeviceSecret, mangaplusSecretHint(), apiErr.Error())
	case apiErr.hasCode(mangaplusErrRateLimited):
		// Remember the lockout: it doesn't clear for ten or more minutes, so
		// every later call would be answered with the same refusal
		m.rateLimited.Store(true)

		return fmt.Errorf("%w (%s)", errMangaplusRateLimited, apiErr.Error())
	}

	return apiErr
}

// mangaplusSecretHint describes the two ways out of a rejected device secret.
// Either of them can be the stale one and only the user knows which: the
// environment variable wins over the cached file, so a bad MANGAPLUS_SECRET
// hides a good cached secret and the other way around.
func mangaplusSecretHint() string {
	if path, err := mangaplusSecretPath(); err == nil {
		return fmt.Sprintf(
			"%s isn't a secret this API knows, or the secret cached in %s is stale: unset %s or delete %s, then run again to register a new device",
			mangaplusSecretEnv, path, mangaplusSecretEnv, path,
		)
	}

	return fmt.Sprintf(
		"%s isn't a secret this API knows, or the cached secret is stale: unset %s or delete the cached secret, then run again to register a new device",
		mangaplusSecretEnv, mangaplusSecretEnv,
	)
}

// mangaplusURLRe matches the site's series and reader URLs, capturing which of
// the two it is and the numeric id it points at
var mangaplusURLRe = regexp.MustCompile(`^/(titles|viewer)/(\d+)(?:/|$)`)

// mangaplusHostRe matches the site's domain and its subdomains
var mangaplusHostRe = regexp.MustCompile(`(?i)(^|\.)mangaplus\.shueisha\.co\.jp$`)

// mangaplusURL parses a site URL, returning the kind of page it is
// (mangaplusURLTitle or mangaplusURLViewer) and the id it points at
func mangaplusURL(rawURL string) (kind string, id uint32, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || !mangaplusHostRe.MatchString(u.Hostname()) {
		return "", 0, false
	}

	matches := mangaplusURLRe.FindStringSubmatch(u.Path)
	if matches == nil {
		return "", 0, false
	}

	parsed, err := strconv.ParseUint(matches[2], 10, 32)
	if err != nil {
		return "", 0, false
	}

	return matches[1], uint32(parsed), true
}

// mangaplusChapterPrefixRe matches the "Chapter 12:" / "Chapter 12." /
// "Chapter 12 -" prefix every subtitle in the chapter list starts with,
// capturing the number so it can be told apart from a title that just happens
// to start with the word "chapter"
var mangaplusChapterPrefixRe = regexp.MustCompile(`(?i)^\s*chapter\s*(\d+(?:\.\d+)?)\s*[:.\-–]?\s*`)

// mangaplusChapterTitle builds a chapter's title out of the API's subtitle,
// dropping the leading "Chapter <number>[:.-]" prefix (the number is already
// part of the file name template) and falling back to the untouched subtitle —
// and then to the short name — whenever stripping it would leave nothing.
//
// A split part is what the exception needs: the subtitle writes it the way the
// site names it ("Chapter 1.1" for "#001-1"), while its number keeps the part
// in the hundredths (1.01, so a two-digit part can't collide with an integer
// chapter), so both spellings count as the chapter's own number.
func mangaplusChapterTitle(subTitle, name string, number float64) string {
	title := strings.TrimSpace(subTitle)

	if matches := mangaplusChapterPrefixRe.FindStringSubmatch(title); matches != nil {
		if prefix, err := strconv.ParseFloat(matches[1], 64); err == nil &&
			(prefix == number || prefix == mangaplusPartSubtitleNumber(name)) {
			if stripped := strings.TrimSpace(title[len(matches[0]):]); stripped != "" {
				title = stripped
			}
		}
	}

	if title == "" {
		title = strings.TrimSpace(name)
	}

	return title
}

// mangaplusPartSubtitleNumber returns the number a split chapter writes in its
// own subtitle: "Chapter 1.1" for the chapter named "#001-1". A chapter whose
// name isn't a split part reads as -1, which no subtitle number can equal.
func mangaplusPartSubtitleNumber(name string) float64 {
	base, part, ok := mangaplusSplitPart(name)
	if !ok {
		return -1
	}

	return base + part/10
}

// mangaplusSplitPart parses the "<base>-<part>" name of a split chapter,
// "#001-1" being the first part of chapter 1
func mangaplusSplitPart(name string) (base, part float64, ok bool) {
	cleaned := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "#"))

	i := strings.Index(cleaned, "-")
	if i <= 0 {
		return 0, 0, false
	}

	base, errBase := strconv.ParseFloat(strings.TrimSpace(cleaned[:i]), 64)
	part, errPart := strconv.ParseFloat(strings.TrimSpace(cleaned[i+1:]), 64)
	if errBase != nil || errPart != nil {
		return 0, 0, false
	}

	return base, part, true
}

// mangaplusChapterNumber returns the number to sort and filter a chapter by.
// The API lists chapters in ascending order but names some of them without a
// number at all ("ex", "One-shot") and splits others with a "-N" suffix
// ("#001-1" is the first part of chapter 1), so the number is derived from the
// name, falls back to "right after the previous chapter" and is finally clamped
// to stay strictly increasing: Filterables is sorted by number, and the API's
// own order would otherwise be lost.
//
// A part's number goes into the hundredths (base + part/100, so "#001-1" is
// 1.01): with tenths "#001-10" would land exactly on chapter 2 and --range 2
// would then match the first part of chapter 1.
func mangaplusChapterNumber(name string, previous float64) float64 {
	number := -1.0

	if base, part, ok := mangaplusSplitPart(name); ok {
		number = base + part/100
	}

	if number < 0 {
		cleaned := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "#"))
		if parsed, err := strconv.ParseFloat(cleaned, 64); err == nil {
			number = parsed
		}
	}

	number = mangaplusRoundChapterNumber(number)

	if number < 0 || number <= previous {
		number = mangaplusRoundChapterNumber(previous + 0.1)
	}

	return number
}

// mangaplusRoundChapterNumber trims the floating point error the /100 and
// +0.1 steps above leave behind (1.01 + 0.1 is 1.1100000000000001, which reads
// badly in a file name) while keeping a chapter number's own decimals
func mangaplusRoundChapterNumber(number float64) float64 {
	return math.Round(number*1000) / 1000
}

// mangaplusLanguageCode maps the language enum of a title's detail to its
// 3-letter code. The enum is ordered as the API declares it, and a value the
// list doesn't know falls back to English.
func mangaplusLanguageCode(code uint64) string {
	if lang, ok := mangaplusLanguageCodes[code]; ok {
		return lang
	}

	return mangaplusDefaultLanguage
}

// mangaplusLanguage maps the --language flag to the code the app API names its
// editions with. An empty flag reads as an empty code: it asks for no edition
// in particular, and the one the URL points at is downloaded (see detailLocked).
// MangaPlus names its editions with an ISO 639-2/T code, while the flag (and
// most sites) use the shorter ISO 639-1 one; anything else is taken as-is and so
// finds no edition, which resolveEditionLocked reports as a language the title
// isn't published in — the alternative, resolving an unrecognised code to the
// default edition, would download an edition nobody asked for.
func mangaplusLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return ""
	}
	if code, ok := mangaplusLanguages[lang]; ok {
		return code
	}

	return lang
}

// md5Hex returns the lowercase hex md5 of s. It's the API's own (and weak)
// way of deriving a device identifier out of the Android client's android_id,
// not a hash of ours for anything security related.
func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))

	return hex.EncodeToString(sum[:])
}

// mangaplusSecretPath returns where the registered device secret is cached.
// There's deliberately no fallback when the user config dir is unknown: the
// secret would otherwise be written into a shared, world-readable directory
// (os.TempDir) instead of the user's own one.
//
// Failing to find one is not fatal to the grabber — the secret is only ever
// cached, never required — so the error says as much, and names the way to
// hand over a secret registered elsewhere instead.
func mangaplusSecretPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf(
			"could not find a user config dir, which is where the MangaPlus device secret is cached: %w; set %s to use a secret registered elsewhere",
			err, mangaplusSecretEnv,
		)
	}

	return filepath.Join(dir, "manga-downloader", "mangaplus-secret"), nil
}

// mangaplusStoreSecret caches the device secret, so the next run doesn't have
// to register a new device (and spend one of its calls doing so)
func mangaplusStoreSecret(path, secret string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(secret+"\n"), 0600)
}

// mangaplusAPIResponse is a decoded app API response: either the fields of its
// success payload or, for the logical failures the API also answers with HTTP
// 200, its error envelope — plus the token a viewer response minted for its own
// image URLs.
type mangaplusAPIResponse struct {
	// success holds the fields of the SuccessResult message
	success []pbField
	// token is the plus_vw_token the response set, if any
	token string
}

// decode splits a raw app API response body into its success payload or its
// error envelope. The two are mutually exclusive siblings of the top-level
// Response message, and anything else (an empty body, a message with neither
// one) is refused rather than treated as a success.
func (r *mangaplusAPIResponse) decode(body []byte) error {
	fields, err := pbParse(body)
	if err != nil {
		return fmt.Errorf("could not decode the MangaPlus API response: %w", err)
	}

	if success, ok := pbFieldByNumber(fields, mangaplusSuccessField); ok {
		payload, err := pbMessage(success)
		if err != nil {
			return err
		}
		r.success = payload

		return nil
	}

	if apiErr, ok := pbFieldByNumber(fields, mangaplusErrorField); ok {
		decoded, err := decodeMangaplusAPIError(apiErr)
		if err != nil {
			return err
		}

		return decoded
	}

	return errors.New("the MangaPlus API returned an unrecognised response")
}

// mangaplusAPIError is the error envelope the app API answers failures with.
// It repeats the same popup once per supported language: only the first one is
// surfaced to the user, but all of them are searched for the numeric code the
// API appends to every popup body, e.g. "...Please try again later.(10521)".
type mangaplusAPIError struct {
	subject string
	body    string
	// popups is every popup's subject and body concatenated, only used to
	// look codes up (they're not guaranteed to be in the same one)
	popups string
}

// Error returns the popup as the API wrote it
func (e *mangaplusAPIError) Error() string {
	switch {
	case e.subject == "":
		return e.body
	case e.body == "":
		return e.subject
	}

	return e.subject + ": " + e.body
}

// hasCode reports whether any of the envelope's popups carries the given
// numeric error code
func (e *mangaplusAPIError) hasCode(code string) bool {
	return strings.Contains(e.popups, "("+code+")")
}

// decodeMangaplusAPIError decodes an ErrorResult: english_popup(2), whose
// subject(1) and body(2) carry the message, plus the localized_popup(5)
// repetitions of it
func decodeMangaplusAPIError(field pbField) (*mangaplusAPIError, error) {
	fields, err := pbMessage(field)
	if err != nil {
		return nil, err
	}

	apiErr := &mangaplusAPIError{}

	if popup, ok := pbFieldByNumber(fields, mangaplusErrorPopupField); ok {
		popupFields, err := pbMessage(popup)
		if err != nil {
			return nil, err
		}
		apiErr.subject = pbString(popupFields, mangaplusErrorPopupSubjectField)
		apiErr.body = pbString(popupFields, mangaplusErrorPopupBodyField)
	}

	var popups strings.Builder
	for _, popup := range pbRepeated(fields, mangaplusErrorPopupField) {
		popupFields, err := pbMessage(popup)
		if err != nil {
			return nil, err
		}
		popups.WriteString(pbString(popupFields, mangaplusErrorPopupSubjectField))
		popups.WriteString(pbString(popupFields, mangaplusErrorPopupBodyField))
	}
	for _, popup := range pbRepeated(fields, mangaplusErrorLocalizedPopupField) {
		popupFields, err := pbMessage(popup)
		if err != nil {
			return nil, err
		}
		popups.WriteString(pbString(popupFields, mangaplusErrorPopupSubjectField))
		popups.WriteString(pbString(popupFields, mangaplusErrorPopupBodyField))
	}
	apiErr.popups = popups.String()

	return apiErr, nil
}

// mangaplusTitleDetail is a decoded title_detailV3 response
//
// TitleDetailView.title(1) -> Title: title_id(1), name(2), language(7);
// title_languages(27) -> one edition per language, and
// chapter_list_v2(38) (or, as a fallback, chapter_list_group(28)) for the
// chapters
type mangaplusTitleDetail struct {
	Title mangaplusTitle
	// Languages is every edition the title is published as, one entry per
	// language, each with the title_id that edition has to be asked for. It's
	// what lets a language be turned into the edition it belongs to: nothing
	// else can, since every edition is a title_id of its own
	Languages []mangaplusTitleLanguage
	Chapters  []mangaplusChapterInfo
}

// mangaplusTitleLanguage is one TitleDetailView.title_languages entry: the
// title_id the title is published as in one language, plus that language. An
// absent language is the enum's first value, English, exactly as in Title.
type mangaplusTitleLanguage struct {
	titleID  uint32
	language string
}

// editionTitleID returns the title_id of the edition the title is published as
// in the given language, if the API lists one. An entry with no id can't be
// asked for at all, so it doesn't count as one.
func (d *mangaplusTitleDetail) editionTitleID(language string) (uint32, bool) {
	for _, edition := range d.Languages {
		if edition.titleID != 0 && edition.language == language {
			return edition.titleID, true
		}
	}

	return 0, false
}

// languageCodes returns the languages the title is published in, in the order
// the API lists them. A response without title_languages (an older app API)
// still proves one language: the one of its own edition.
func (d *mangaplusTitleDetail) languageCodes() []string {
	var codes []string
	for _, edition := range d.Languages {
		codes = append(codes, edition.language)
	}

	if len(codes) == 0 {
		codes = append(codes, d.Title.Language)
	}

	return codes
}

// mangaplusTitle is the series metadata the API reports, of which only the name
// and the language of the edition matter here
//
// The language is an enum, and it's left at its first value (English) for an
// English title, so an absent field means English rather than unknown.
type mangaplusTitle struct {
	Name     string
	Language string
}

// mangaplusChapterInfo is a chapter as the title detail lists it
type mangaplusChapterInfo struct {
	chapterID uint32
	// name is the short chapter name ("#001", "#001-1", "ex")
	name string
	// subTitle is the long chapter name ("Chapter 1: Romance Dawn")
	subTitle string
	// chapterType is FREE (0), FREE_FOR_FIRST_TIME (1), STANDARD (2) or
	// DELUXE (3)
	chapterType uint64
	// alreadyViewed and viewedForFree are the API's own free-read flags
	alreadyViewed bool
	viewedForFree bool
}

// decodeMangaplusTitleDetail decodes a TitleDetailView. The complete chapter
// list is chapter_list_v2 (field 38); chapter_list_group (field 28) is the
// older shape, whose first/mid/last windows only ever cover part of a long
// series, so it's decoded as a fallback for a response without field 38 rather
// than alongside it.
func decodeMangaplusTitleDetail(view []byte) (*mangaplusTitleDetail, error) {
	fields, err := pbParse(view)
	if err != nil {
		return nil, err
	}

	detail := &mangaplusTitleDetail{}

	if title, ok := pbFieldByNumber(fields, 1); ok {
		titleFields, err := pbMessage(title)
		if err != nil {
			return nil, err
		}
		detail.Title = mangaplusTitle{
			Name:     pbString(titleFields, 2),
			Language: mangaplusLanguageCode(pbUint(titleFields, 7)),
		}
	}
	if detail.Title.Name == "" {
		return nil, errors.New("the MangaPlus API returned no title name")
	}

	// TitleDetailView.title_languages(27) -> TitleLanguages: title_id(1),
	// language(2); one entry per language the title is published as, which is
	// what turns a language into the edition to ask for
	for _, edition := range pbRepeated(fields, mangaplusTitleLanguagesField) {
		editionFields, err := pbMessage(edition)
		if err != nil {
			return nil, err
		}
		detail.Languages = append(detail.Languages, mangaplusTitleLanguage{
			titleID:  uint32(pbUint(editionFields, 1)),
			language: mangaplusLanguageCode(pbUint(editionFields, 2)),
		})
	}

	for _, chapter := range pbRepeated(fields, mangaplusChapterListV2Field) {
		info, err := decodeMangaplusChapterInfo(chapter)
		if err != nil {
			return nil, err
		}
		detail.Chapters = append(detail.Chapters, info)
	}

	if len(detail.Chapters) == 0 {
		// The older list's windows can overlap (a long series lists the same
		// chapter in two of them), and a chapter listed twice would be packed
		// twice — as a spurious v2 file. The first occurrence wins and the
		// server's order is kept. A chapter without an id isn't deduped: it
		// can't be downloaded at all, and collapsing every one of them into
		// the first would lose most of the list.
		seen := map[uint32]bool{}
		for _, group := range pbRepeated(fields, mangaplusChapterListGroupField) {
			groupFields, err := pbMessage(group)
			if err != nil {
				return nil, err
			}
			for _, field := range []int{
				mangaplusChapterGroupFirstField,
				mangaplusChapterGroupMidField,
				mangaplusChapterGroupLastField,
			} {
				for _, chapter := range pbRepeated(groupFields, field) {
					info, err := decodeMangaplusChapterInfo(chapter)
					if err != nil {
						return nil, err
					}
					if info.chapterID != 0 {
						if seen[info.chapterID] {
							continue
						}
						seen[info.chapterID] = true
					}
					detail.Chapters = append(detail.Chapters, info)
				}
			}
		}
	}

	if len(detail.Chapters) == 0 {
		return nil, errors.New("the MangaPlus API returned no chapters")
	}

	return detail, nil
}

// locked reports whether a chapter looks like one that can't be read without a
// Manga Plus MAX subscription: a non-free type (STANDARD or DELUXE) that the
// API has no record of having been read. A chapter read once for free keeps
// working after the API flips it from FREE_FOR_FIRST_TIME to STANDARD, which is
// why the read flags are what makes the type alone inconclusive.
func (c mangaplusChapterInfo) locked() bool {
	return c.chapterType >= mangaplusChapterTypeStandard && !c.alreadyViewed && !c.viewedForFree
}

// decodeMangaplusChapterInfo decodes one Chapter of a title detail
//
// Chapter.chapter_id(2), name(3), sub_title(4), already_viewed(8),
// viewed_for_free(11), chapter_type(16)
func decodeMangaplusChapterInfo(field pbField) (mangaplusChapterInfo, error) {
	fields, err := pbMessage(field)
	if err != nil {
		return mangaplusChapterInfo{}, err
	}

	return mangaplusChapterInfo{
		chapterID:     uint32(pbUint(fields, 2)),
		name:          strings.TrimSpace(pbString(fields, 3)),
		subTitle:      strings.TrimSpace(pbString(fields, 4)),
		alreadyViewed: pbBool(fields, 8),
		viewedForFree: pbBool(fields, 11),
		chapterType:   pbUint(fields, 16),
	}, nil
}

// mangaplusViewer is a decoded manga_viewer_v3 response
//
// MangaViewer.chapter_id(2), title_id(9), title_language(15)
type mangaplusViewer struct {
	titleID uint32
	// pages holds the image URL of every manga_page of the chapter, in order
	pages []string
	// token is the plus_vw_token the response minted for those URLs
	token string
}

// decodeMangaplusViewer decodes a MangaViewer. Its pages are a oneof: only
// manga_page entries are images, while the banner, advertisement and last-page
// variants are the reader's own furniture — an advertisement becoming "page 20"
// of a chapter is exactly what this filter is here to prevent.
func decodeMangaplusViewer(success []pbField, token string) (*mangaplusViewer, error) {
	views := pbRepeated(success, mangaplusViewerField)
	if len(views) == 0 {
		return nil, errors.New("the MangaPlus API returned no viewer data")
	}
	fields, err := pbMessage(views[0])
	if err != nil {
		return nil, err
	}

	viewer := &mangaplusViewer{
		titleID: uint32(pbUint(fields, 9)),
		token:   token,
	}

	for i, page := range pbRepeated(fields, mangaplusViewerPagesField) {
		pageFields, err := pbMessage(page)
		if err != nil {
			return nil, err
		}
		// every other member of the oneof (banner_list, last_page,
		// advertisement, insert_banner_list) is reader furniture, not a page
		mangaPage, ok := pbFieldByNumber(pageFields, mangaplusPageMangaPageField)
		if !ok {
			continue
		}
		imageFields, err := pbMessage(mangaPage)
		if err != nil {
			return nil, err
		}
		// MangaPage.image_url(1); its encryption_key(5) is always empty on
		// this API path: these images are plain, undecypted WebP
		imageURL := pbString(imageFields, 1)
		if imageURL == "" {
			return nil, fmt.Errorf("the MangaPlus API returned page %d of the chapter without an image", i+1)
		}
		viewer.pages = append(viewer.pages, imageURL)
	}

	return viewer, nil
}
