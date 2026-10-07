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

// mangaplusAPIBase is the app API's base URL (a var so tests can point it at a
// local server)
var mangaplusAPIBase = "https://jumpg-api.tokyo-cdn.com/api"

// mangaplusRequestInterval spaces out the calls to the app API: about ten
// requests in a few seconds lock the device out for ten or more minutes
var mangaplusRequestInterval = 500 * time.Millisecond

// The params identifying the client; the Android one is what gets the
// complete chapter list
const (
	mangaplusPlatform   = "android"
	mangaplusOSVersion  = "36"
	mangaplusAppVersion = "250"
	mangaplusUserAgent  = "okhttp/4.12.0"
)

// mangaplusSecurityKeySalt derives a new device's security key:
// md5(device_token + salt)
const mangaplusSecurityKeySalt = "4Kin9vGg"

// mangaplusSecretEnv overrides the cached device secret
const mangaplusSecretEnv = "MANGAPLUS_SECRET"

// mangaplusViewTokenCookie is the cookie a viewer response sets, which its
// image URLs are signed for
const mangaplusViewTokenCookie = "plus_vw_token"

const mangaplusRequestTimeout = 60 * time.Second

// mangaplusMaxResponseSize caps a response body. Reaching it is an error, not a
// truncation: half a protobuf message can still decode into a plausible one.
const mangaplusMaxResponseSize = 16 << 20

// mangaplusMaxErrorBodySize caps the body read to explain a non-200 response
const mangaplusMaxErrorBodySize = 64 << 10

// mangaplusChapterTypeStandard is the first chapter_type that isn't free
// (FREE 0, FREE_FOR_FIRST_TIME 1, STANDARD 2, DELUXE 3)
const mangaplusChapterTypeStandard = 2

// mangaplusFreeWindowChapters is how many chapters an edition that only
// carries the free window lists
const mangaplusFreeWindowChapters = 6

// Field numbers of the app API's protobuf schema, only the ones read here
const (
	// Response.success / Response.error
	mangaplusSuccessField = 1
	mangaplusErrorField   = 2
	// SuccessResult.registeration_data (sic), title_detail_view, manga_viewer
	mangaplusRegistrationDataField = 2
	mangaplusTitleDetailViewField  = 8
	mangaplusViewerField           = 10
	// MangaViewer.pages -> Page, whose manga_page(1) is the only image variant
	mangaplusViewerPagesField   = 1
	mangaplusPageMangaPageField = 1
	// TitleDetailView.title_languages: one edition (title_id) per language
	mangaplusTitleLanguagesField = 27
	// TitleDetailView.chapter_list_v2, the complete and ascending list
	mangaplusChapterListV2Field = 38
	// TitleDetailView.chapter_list_group, the older partial list
	mangaplusChapterListGroupField = 28
	// ChapterGroup.first_chapter_list / mid_chapter_list / last_chapter_list
	mangaplusChapterGroupFirstField = 2
	mangaplusChapterGroupMidField   = 3
	mangaplusChapterGroupLastField  = 4
	// RegistrationData.device_secret
	mangaplusRegistrationSecretField = 1
	// ErrorResult.english_popup and localized_popup -> ErrorPopup
	mangaplusErrorPopupField          = 2
	mangaplusErrorLocalizedPopupField = 5
	mangaplusErrorPopupSubjectField   = 1
	mangaplusErrorPopupBodyField      = 2
)

// The codes the app API appends to its (otherwise generic) error popups, e.g.
// "...Please try again later.(10521)"
const (
	mangaplusErrUnknownSecret   = "11012"
	mangaplusErrMissingSecret   = "10521"
	mangaplusErrRateLimited     = "10522"
	mangaplusErrSubscription    = "11301"
	mangaplusErrSubscriptionAlt = "11302"
)

// errMangaplusSubscription is returned when the API refuses a chapter. The
// chapter type can't predict it: a chapter read once for free flips from
// FREE_FOR_FIRST_TIME to STANDARD and stays readable.
var errMangaplusSubscription = errors.New("this chapter requires a Manga Plus MAX subscription")

var errMangaplusDeviceSecret = errors.New("the MangaPlus API rejected the device secret")

var errMangaplusRateLimited = errors.New("the MangaPlus API is rate limiting this device, which locks it out for about ten minutes: retry after that, not sooner")

// The two kinds of page the grabber accepts: a series and a chapter reader
const (
	mangaplusURLTitle  = "titles"
	mangaplusURLViewer = "viewer"
)

// mangaplusSecretRe matches the secrets the API issues; anything else is
// treated as absent
var mangaplusSecretRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// mangaplusLanguages maps the --language codes to the API's
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

// mangaplusLanguageCodes maps the Title.language enum to its code
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

// mangaplusLanguageNames names the language codes in user-facing messages
var mangaplusLanguageNames = map[string]string{
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

// Mangaplus is Shueisha's official reader (mangaplus.shueisha.co.jp).
//
// It talks to the *app* API (jumpg-api) rather than the website's one, which
// only lists a window of the chapters and paywalls the middle of a running
// series. The app API speaks protobuf (see protobuf.go) and authenticates with
// an anonymous device secret any client can register.
//
//   - It rate limits hard (10522, a lockout of ten or more minutes), so every
//     call waits on rateLimiter.
//   - A viewer response mints a plus_vw_token bound to that response's signed
//     image URLs (none: 400, another response's: 403), so the token travels
//     with each page instead of in the shared http session, and each viewer
//     response is cached because --bundle fetches every chapter twice.
type Mangaplus struct {
	*Grabber
	rateLimiter <-chan time.Time
	// client is our own rather than the shared http helper: the token is only
	// in the viewer response's Set-Cookie, which the helper would put in a
	// session shared by every chapter downloading in parallel
	client *http.Client
	// rateLimited is set on the first 10522: the lockout lasts minutes, so no
	// further call is made (each one would also feed the limiter)
	rateLimited atomic.Bool
	// mu guards the lazily built state below, and serializes the API calls
	mu       sync.Mutex
	title    string
	titleId  uint32
	detail   *mangaplusTitleDetail
	language string // the API code of the edition being downloaded
	chapters Filterables
	viewers  map[uint32]*mangaplusViewer
	// secretMu guards the secret alone, which is resolved while mu may be held
	secretMu sync.Mutex
	secret   string
	// secretFromEnv marks a MANGAPLUS_SECRET, which is never replaced
	secretFromEnv bool
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
	Id       uint32
	SubTitle string // e.g. "Chapter 1: Romance Dawn"
	Name     string // e.g. "#001"
}

// Test matches the URL alone: a request would cost a rate limit tick
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

	// the list is ascending but not every name is a number, so numbers are
	// derived in order
	previous := -0.1
	locked := 0

	for _, chapter := range detail.Chapters {
		number := mangaplusChapterNumber(chapter.name, previous)
		previous = number

		// a count for the user, not a filter (see errMangaplusSubscription)
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

	// The token travels with its own pages. The explicit Cookie overrides any
	// token the shared session harvested from another response.
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

// titleIDLocked returns the title_id to query, resolving a reader URL's
// through its viewer response. It must be called with m.mu held.
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

// detailLocked returns the title detail of the edition being downloaded,
// fetching it once. It must be called with m.mu held.
func (m *Mangaplus) detailLocked() (*mangaplusTitleDetail, error) {
	if m.detail != nil {
		return m.detail, nil
	}

	titleID, err := m.titleIDLocked()
	if err != nil {
		return nil, err
	}

	detail, err := m.fetchDetail(titleID)
	if err != nil {
		return nil, err
	}

	requested := mangaplusLanguage(m.GetPreferredLanguage())
	if requested != "" {
		if detail, err = m.resolveEditionLocked(detail, requested); err != nil {
			return nil, err
		}
	}

	m.detail = detail
	m.language = detail.Title.Language

	// An edition listing only the free window is what landing on a restricted
	// one looks like; the other editions may list the whole run
	if len(detail.languageCodes()) > 1 && len(detail.Chapters) <= mangaplusFreeWindowChapters {
		hint := "use --language to pick another"
		if requested != "" {
			hint = "pick another one with --language"
		}
		color.Yellow(
			"This MangaPlus version lists only %d chapters (each language is a separate version: %s): %s",
			len(detail.Chapters), mangaplusFlagLanguages(detail.languageCodes()), hint,
		)
	}

	return m.detail, nil
}

// fetchDetail requests and decodes the title detail of one edition
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
// for. Each language is a separate edition with its own title_id and chapter
// list, and lang/clang can't switch between them, so the wanted edition's id is
// looked up in title_languages and fetched instead. It must be called with
// m.mu held.
func (m *Mangaplus) resolveEditionLocked(detail *mangaplusTitleDetail, requested string) (*mangaplusTitleDetail, error) {
	if detail.Title.Language == requested {
		return detail, nil
	}

	titleID, ok := detail.editionTitleID(requested)
	if !ok {
		if available := detail.languageCodes(); len(available) > 0 {
			color.Yellow(
				"MangaPlus has no %s version of %s (available: %s); downloading the %s version the link points at",
				mangaplusLanguageName(requested), detail.Title.Name, mangaplusFlagLanguages(available), mangaplusLanguageName(detail.Title.Language),
			)
		} else {
			color.Yellow(
				"MangaPlus listed no other versions of %s, so %s isn't available; downloading the %s version the link points at",
				detail.Title.Name, mangaplusLanguageName(requested), mangaplusLanguageName(detail.Title.Language),
			)
		}

		return detail, nil
	}

	switched, err := m.fetchDetail(titleID)
	if err != nil {
		return nil, err
	}

	// the languages list isn't always consistent with the detail it leads to,
	// so the message follows what the refetched edition reports
	if switched.Title.Language == requested {
		color.Yellow(
			"The provided MangaPlus link is for the %s version of %s; --language selected %s, so that's the version being downloaded",
			mangaplusLanguageName(detail.Title.Language), detail.Title.Name, mangaplusLanguageName(requested),
		)
	} else {
		color.Yellow(
			"MangaPlus's listing for the %s version of %s doesn't match the title it returns: downloading the %s version",
			mangaplusLanguageName(requested), detail.Title.Name, mangaplusLanguageName(switched.Title.Language),
		)
	}

	m.titleId = titleID

	return switched, nil
}

// viewerLocked returns the decoded viewer response of a chapter, fetching it
// once. It must be called with m.mu held.
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
	// unknown before the edition's detail (a reader URL's first call), and the
	// API answers fine without it
	if m.language != "" {
		params.Set("clang", m.language)
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
		return nil, fmt.Errorf("the MangaPlus API returned no pages for chapter %d", chapterID)
	}
	if viewer.token == "" {
		return nil, fmt.Errorf("the MangaPlus API returned no %s cookie for chapter %d, which its image URLs are signed for", mangaplusViewTokenCookie, chapterID)
	}

	m.viewers[chapterID] = viewer

	return viewer, nil
}

// apiGet performs an authenticated, rate-limited GET against the app API
func (m *Mangaplus) apiGet(path string, params url.Values) (*mangaplusAPIResponse, error) {
	secret, err := m.deviceSecret()
	if err != nil {
		return nil, err
	}

	response, err := m.request("GET", path, params, secret)
	if !errors.Is(err, errMangaplusDeviceSecret) {
		return response, err
	}

	// a cached secret the API no longer knows is replaced by a new device, once
	renewed, renewErr := m.renewDeviceSecret(secret)
	if renewErr != nil {
		return nil, renewErr
	}
	if renewed == "" {
		return nil, err
	}

	return m.request("GET", path, params, renewed)
}

// renewDeviceSecret registers a new device in place of a rejected secret. It
// returns an empty secret for a MANGAPLUS_SECRET, which the user set on purpose.
func (m *Mangaplus) renewDeviceSecret(rejected string) (string, error) {
	m.secretMu.Lock()
	defer m.secretMu.Unlock()

	if m.secretFromEnv {
		return "", nil
	}
	if m.secret != rejected {
		// someone else already renewed it
		return m.secret, nil
	}

	path, err := mangaplusSecretPath()
	if err != nil {
		return "", err
	}

	color.Yellow("the cached MangaPlus device secret was rejected, registering a new device")

	return m.registerDeviceLocked(path)
}

// request performs one call against the app API. Only registration is made
// without a secret.
func (m *Mangaplus) request(method, path string, params url.Values, secret string) (*mangaplusAPIResponse, error) {
	// checked before the limiter, so refused calls don't wait out an interval
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
		// logical failures come as HTTP 200, but a non-200 may still carry the
		// error envelope, whose message is more useful than the status
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
	// read off this very response, so concurrent chapters can't borrow each
	// other's token
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

// deviceSecret returns the device secret: MANGAPLUS_SECRET, then the cached
// one, and otherwise a newly registered device
func (m *Mangaplus) deviceSecret() (string, error) {
	m.secretMu.Lock()
	defer m.secretMu.Unlock()

	if m.secret != "" {
		return m.secret, nil
	}

	if secret := strings.TrimSpace(os.Getenv(mangaplusSecretEnv)); mangaplusSecretRe.MatchString(secret) {
		m.secret, m.secretFromEnv = secret, true
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

	return m.registerDeviceLocked(path)
}

// registerDeviceLocked registers a new device and caches its secret in path.
// It must be called with m.secretMu held.
func (m *Mangaplus) registerDeviceLocked(path string) (string, error) {
	secret, err := m.registerDevice()
	if err != nil {
		return "", err
	}
	m.secret = secret

	if err = mangaplusStoreSecret(path, secret); err != nil {
		color.Yellow("registered a new MangaPlus device, but could not store its secret in %s (it will be registered again on the next run): %s", path, err)
	} else {
		color.Yellow("registered a new MangaPlus device, whose secret is stored in %s", path)
	}

	return m.secret, nil
}

// registerDevice registers an anonymous device, identified by an md5 of 8
// random bytes (the Android client's android_id), and returns its secret
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

// explainAPIError turns an API error envelope into the error the user gets,
// telling the actionable cases apart by their code
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
		m.rateLimited.Store(true)

		return fmt.Errorf("%w (%s)", errMangaplusRateLimited, apiErr.Error())
	}

	return apiErr
}

// mangaplusSecretHint names both ways out of a rejected secret: the variable
// wins over the cached file, so either may be the stale one
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

var mangaplusURLRe = regexp.MustCompile(`^/(titles|viewer)/(\d+)(?:/|$)`)

var mangaplusHostRe = regexp.MustCompile(`(?i)(^|\.)mangaplus\.shueisha\.co\.jp$`)

// mangaplusURL parses a site URL into its kind (mangaplusURLTitle or
// mangaplusURLViewer) and id
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

// mangaplusChapterPrefixRe matches the "Chapter 12:" prefix of a subtitle, in
// every edition's words for it ("Capítulo 12:", "Chapitre 12:", "ตอนที่ 12")
var mangaplusChapterPrefixRe = regexp.MustCompile(`(?i)^\s*(?:chapter|cap[ií]tulo|chapitre|ตอนที่)\s*(\d+(?:\.\d+)?)\s*[:.\-–]?\s*`)

// mangaplusChapterTitle builds a chapter's title out of the API's subtitle,
// dropping its "Chapter <number>:" prefix when the number is the chapter's own,
// and never leaving it empty
func mangaplusChapterTitle(subTitle, name string, number float64) string {
	title := strings.TrimSpace(subTitle)

	if matches := mangaplusChapterPrefixRe.FindStringSubmatch(title); matches != nil {
		if prefix, err := strconv.ParseFloat(matches[1], 64); err == nil &&
			(prefix == number || prefix == mangaplusPartSubtitleNumber(name)) {
			stripped := strings.TrimSpace(title[len(matches[0]):])
			// the Spanish edition ends every title with a full stop, which
			// would sit right before the file extension (an ellipsis stays)
			if strings.HasSuffix(stripped, ".") && !strings.HasSuffix(stripped, "..") {
				stripped = strings.TrimSpace(strings.TrimSuffix(stripped, "."))
			}
			if stripped != "" {
				title = stripped
			}
		}
	}

	if title == "" {
		title = strings.TrimSpace(name)
	}

	return title
}

// mangaplusPartSubtitleNumber returns the number a split chapter's subtitle
// uses ("Chapter 1.1" for "#001-1"), or -1 for any other chapter
func mangaplusPartSubtitleNumber(name string) float64 {
	base, part, ok := mangaplusSplitPart(name)
	if !ok {
		return -1
	}

	return base + part/10
}

// mangaplusSplitPart parses a split chapter's "#<base>-<part>" name
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

// mangaplusChapterNumber derives a chapter's number from its name, falling back
// to right after the previous one ("ex", "One-shot") and kept strictly
// increasing so the API's order survives sorting. A split part goes into the
// hundredths ("#001-1" is 1.01): with tenths, "#001-10" would be chapter 2.
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

// mangaplusRoundChapterNumber trims floating point error (1.01 + 0.1 is
// 1.1100000000000001)
func mangaplusRoundChapterNumber(number float64) float64 {
	return math.Round(number*1000) / 1000
}

// mangaplusLanguageCode maps the language enum to its code. An unknown value
// reads as "lang<N>" rather than a guess, which could pass for another edition.
func mangaplusLanguageCode(code uint64) string {
	if lang, ok := mangaplusLanguageCodes[code]; ok {
		return lang
	}

	return fmt.Sprintf("lang%d", code)
}

// mangaplusLanguageName returns a language code's name, or the code itself
func mangaplusLanguageName(code string) string {
	if name, ok := mangaplusLanguageNames[code]; ok {
		return name
	}

	return code
}

// mangaplusFlagLanguages lists language codes the way --language takes them
// ("en" rather than the API's "eng"), keeping any code without a short form
func mangaplusFlagLanguages(codes []string) string {
	flags := make([]string, 0, len(codes))
	for _, code := range codes {
		flag := code
		for short, long := range mangaplusLanguages {
			if long == code {
				flag = short
				break
			}
		}
		flags = append(flags, flag)
	}

	return strings.Join(flags, ", ")
}

// mangaplusLanguage maps the --language flag to the API's code. An unknown code
// is kept as-is, so it matches no edition instead of a default one.
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

// md5Hex returns the hex md5 of s, as the API derives device identifiers (not
// used for anything security related)
func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))

	return hex.EncodeToString(sum[:])
}

// mangaplusSecretPath returns where the device secret is cached. There's no
// fallback to a shared directory such as os.TempDir.
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

// mangaplusStoreSecret caches the device secret for the next runs
func mangaplusStoreSecret(path, secret string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(secret+"\n"), 0600)
}

// mangaplusAPIResponse is a decoded app API response, plus the token a viewer
// response set
type mangaplusAPIResponse struct {
	success []pbField
	token   string
}

// decode splits a response body into its success payload or its error
// envelope; anything else is refused
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

// mangaplusAPIError is the API's error envelope. The first popup is what the
// user sees, while every popup is searched for the error code.
type mangaplusAPIError struct {
	subject string
	body    string
	popups  string
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

// hasCode reports whether any of the popups carries the given error code
func (e *mangaplusAPIError) hasCode(code string) bool {
	return strings.Contains(e.popups, "("+code+")")
}

// decodeMangaplusAPIError decodes an ErrorResult
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
type mangaplusTitleDetail struct {
	Title     mangaplusTitle
	Languages []mangaplusTitleLanguage
	Chapters  []mangaplusChapterInfo
}

// mangaplusTitleLanguage is one title_languages entry: an edition's title_id
// and its language
type mangaplusTitleLanguage struct {
	titleID  uint32
	language string
}

// editionTitleID returns the title_id of the edition in the given language
func (d *mangaplusTitleDetail) editionTitleID(language string) (uint32, bool) {
	for _, edition := range d.Languages {
		if edition.titleID != 0 && edition.language == language {
			return edition.titleID, true
		}
	}

	return 0, false
}

// languageCodes returns the languages of the editions that can be asked for
// (those with a title_id), in the API's order
func (d *mangaplusTitleDetail) languageCodes() []string {
	var codes []string
	for _, edition := range d.Languages {
		if edition.titleID == 0 {
			continue
		}
		codes = append(codes, edition.language)
	}

	return codes
}

// mangaplusTitle is the series metadata used here. An absent language is the
// enum's first value: English.
type mangaplusTitle struct {
	Name     string
	Language string
}

// mangaplusChapterInfo is a chapter as the title detail lists it
type mangaplusChapterInfo struct {
	chapterID     uint32
	name          string // "#001", "#001-1", "ex"
	subTitle      string // "Chapter 1: Romance Dawn"
	chapterType   uint64
	alreadyViewed bool
	viewedForFree bool
}

// decodeMangaplusTitleDetail decodes a TitleDetailView. chapter_list_group, the
// older list whose windows only cover part of a long series, is only a
// fallback for a response without chapter_list_v2.
func decodeMangaplusTitleDetail(view []byte) (*mangaplusTitleDetail, error) {
	fields, err := pbParse(view)
	if err != nil {
		return nil, err
	}

	detail := &mangaplusTitleDetail{}

	// title(1) -> Title: name(2), language(7)
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

	// TitleLanguages: title_id(1), language(2)
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
		// the windows can overlap, and a chapter listed twice would be packed
		// twice; chapters without an id can't be told apart, so they're kept
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

// locked reports whether a chapter looks like it needs a subscription: a
// non-free type the API has no record of having been read
func (c mangaplusChapterInfo) locked() bool {
	return c.chapterType >= mangaplusChapterTypeStandard && !c.alreadyViewed && !c.viewedForFree
}

// decodeMangaplusChapterInfo decodes one Chapter: chapter_id(2), name(3),
// sub_title(4), already_viewed(8), viewed_for_free(11), chapter_type(16)
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
type mangaplusViewer struct {
	titleID uint32
	pages   []string
	token   string
}

// decodeMangaplusViewer decodes a MangaViewer: title_id(9) and its pages. Only
// the manga_page variant of a page is an image; banners and ads are skipped.
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
		mangaPage, ok := pbFieldByNumber(pageFields, mangaplusPageMangaPageField)
		if !ok {
			continue
		}
		imageFields, err := pbMessage(mangaPage)
		if err != nil {
			return nil, err
		}
		// MangaPage.image_url(1): plain WebP, its encryption_key is always
		// empty on this API path
		imageURL := pbString(imageFields, 1)
		if imageURL == "" {
			return nil, fmt.Errorf("the MangaPlus API returned page %d of the chapter without an image", i+1)
		}
		viewer.pages = append(viewer.pages, imageURL)
	}

	return viewer, nil
}
