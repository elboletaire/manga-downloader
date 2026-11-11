// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/elboletaire/manga-downloader/http"
)

// MangaHub is a grabber for the MangaHub platform (mangahub.io and the sites running its app).
// Its reader shows only ~6 pages, so the title, chapters and pages come from the GraphQL API.
type MangaHub struct {
	*Grabber
	// manga is the series from the API, fetched once for FetchTitle and FetchChapters
	manga *mangahubManga
	// keys are the page keys the session issued, by key id; the chapters of a run
	// are fetched concurrently, so mu guards them
	mu   sync.Mutex
	keys map[string][]byte
}

// NewMangaHub returns a MangaHub grabber, matched by the markup of its pages
func NewMangaHub(g *Grabber) *MangaHub {
	return &MangaHub{Grabber: g}
}

// mangahubCDN serves the page images, relative to it
const mangahubCDN = "https://imgx.mghcdn.com/"

// mangahubMarker is the API config every page of the platform carries; its apiUrl is empty now,
// so the API is the page's own origin (the old fixed host answers 403)
const mangahubMarker = `window.App={"apiUrl":"`

// mangahubPathRe is the path shape of a series or reader page, the only shape the markup check fetches
var mangahubPathRe = regexp.MustCompile(`(?i)^/(manga/[^/]+|chapter/[^/]+/chapter-[^/]+)/?$`)

// mangahubChapterPathRe captures the origin and slug of a reader URL (/chapter/{slug}/chapter-{n})
var mangahubChapterPathRe = regexp.MustCompile(`(?i)^(https?://[^/?#]+)/chapter/([^/?#]+)/chapter-[^/?#]*`)

// mangahubSlugRe captures the series slug of a series URL
var mangahubSlugRe = regexp.MustCompile(`/manga/([^/?#]+)`)

// mangahubClientRe captures the client bundle a page loads, the one with every site's config
var mangahubClientRe = regexp.MustCompile(`src="(/assets/client[^"]*)"`)

// mangahubNumberTitleRe matches a title that only names the chapter's number, which ChapterLabel names instead
var mangahubNumberTitleRe = regexp.MustCompile(`(?i)^chapter\s*[\d.]+$`)

// mangahubManga is the series as the API returns it. source and slug are the
// keys its chapters' pages are fetched by
type mangahubManga struct {
	Title    string `json:"title"`
	Chapters []struct {
		Number float64 `json:"number"`
		Title  string  `json:"title"`
	} `json:"chapters"`
	source string
	slug   string
	key    string
}

// MangaHubChapter is a chapter of the list, with what its pages are fetched by
type MangaHubChapter struct {
	Chapter
	source string
	slug   string
	key    string
}

// Test matches a series or reader page by the API config its markup carries,
// with no domain list: a clone of the platform needs no code of its own
func (m *MangaHub) Test() (bool, error) {
	u, err := url.Parse(m.URL)
	if err != nil || !mangahubPathRe.MatchString(u.Path) {
		return false, nil
	}

	return pageHasMarkup(m.Grabber, m.URL, mangahubMarker), nil
}

// resolveSeriesURL rewrites a reader URL to its series URL. Series URLs are unchanged.
func (m *MangaHub) resolveSeriesURL() {
	if match := mangahubChapterPathRe.FindStringSubmatch(m.URL); match != nil {
		m.URL = match[1] + "/manga/" + match[2]
	}
}

// FetchTitle returns the series title
func (m *MangaHub) FetchTitle() (string, error) {
	manga, err := m.fetchManga()
	if err != nil {
		return "", err
	}

	return sanitizeTitle(manga.Title), nil
}

// FetchChapters returns the chapters of the series, from the API
func (m *MangaHub) FetchChapters() (Filterables, []error) {
	manga, err := m.fetchManga()
	if err != nil {
		return nil, []error{err}
	}

	chapters := make(Filterables, 0, len(manga.Chapters))
	for _, c := range manga.Chapters {
		title := sanitizeTitle(c.Title)
		if mangahubNumberTitleRe.MatchString(title) {
			title = ""
		}
		chapters = append(chapters, &MangaHubChapter{
			Chapter: Chapter{
				Title:  title,
				Number: c.Number,
				Label:  Label{Style: LabelNumbered},
			},
			source: manga.source,
			slug:   manga.slug,
			key:    manga.key,
		})
	}

	return chapters, nil
}

// FetchChapter fetches the pages of a chapter from the API
func (m *MangaHub) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*MangaHubChapter)
	warm := fmt.Sprintf("{ manga(x: %s, slug: %s) { title } }", mchap.source, strconv.Quote(mchap.slug))

	// the chapter only answers once the session holds a page key
	if err := m.ensureKeys(mchap.key, warm); err != nil {
		return nil, err
	}

	var data struct {
		Chapter *struct {
			Pages string `json:"pages"`
		} `json:"chapter"`
	}
	err := m.query(mchap.key, fmt.Sprintf(
		"{ chapter(x: %s, slug: %s, number: %s) { pages } }",
		mchap.source, strconv.Quote(mchap.slug), strconv.FormatFloat(mchap.Number, 'f', -1, 64),
	), &data)
	if err != nil {
		return nil, err
	}
	if data.Chapter == nil {
		return nil, errors.New("the API has no such chapter")
	}

	pages, err := m.decryptPages(data.Chapter.Pages, mchap.key, warm)
	if err != nil {
		return nil, err
	}
	images, err := mangahubPages(pages)
	if err != nil {
		return nil, err
	}

	chapter := &Chapter{
		Title:      mchap.Title,
		Label:      mchap.Label,
		Number:     mchap.Number,
		PagesCount: int64(len(images)),
		Language:   "en",
	}
	for i, img := range images {
		chapter.Pages = append(chapter.Pages, Page{Number: int64(i + 1), URL: img})
	}

	return chapter, nil
}

// fetchManga returns the series from the API, once per grabber
func (m *MangaHub) fetchManga() (*mangahubManga, error) {
	if m.manga != nil {
		return m.manga, nil
	}

	m.resolveSeriesURL()
	slug, err := m.slug()
	if err != nil {
		return nil, err
	}
	source, key, err := m.siteCode()
	if err != nil {
		return nil, err
	}

	var data struct {
		Manga *mangahubManga `json:"manga"`
	}
	err = m.query(key, fmt.Sprintf("{ manga(x: %s, slug: %s) { title, chapters { number, title } } }", source, strconv.Quote(slug)), &data)
	if err != nil {
		return nil, err
	}
	if data.Manga == nil {
		return nil, fmt.Errorf("the API has no series %q", slug)
	}
	data.Manga.source = source
	data.Manga.slug = slug
	data.Manga.key = key
	m.manga = data.Manga

	return m.manga, nil
}

// siteCode returns the code the API keys this site's data by, the dataSourceKey
// of its entry in the client bundle the series page loads, and the access key
// that same page issued as the mhub_access cookie the API expects in a header
func (m *MangaHub) siteCode() (code, key string, err error) {
	page, err := http.GetText(http.RequestParams{URL: m.URL})
	if err != nil {
		return "", "", err
	}
	base, _ := url.Parse(m.URL)

	key = http.SessionCookie(base.Hostname(), "mhub_access")
	if key == "" {
		return "", "", errors.New("the series page issued no mhub_access cookie")
	}

	match := mangahubClientRe.FindStringSubmatch(page)
	if match == nil {
		return "", "", errors.New("could not find the client bundle in the series page")
	}
	ref, err := url.Parse(match[1])
	if err != nil {
		return "", "", err
	}
	bundle, err := http.GetText(http.RequestParams{URL: base.ResolveReference(ref).String(), Referer: m.URL})
	if err != nil {
		return "", "", err
	}

	host := strings.TrimPrefix(base.Hostname(), "www.")
	entry := regexp.MustCompile(`domain:"` + regexp.QuoteMeta(host) + `",[^}]*dataSourceKey:"([a-z0-9]+)"`)
	site := entry.FindStringSubmatch(bundle)
	if site == nil {
		return "", "", fmt.Errorf("no site code for %s in its client bundle", host)
	}

	return site[1], key, nil
}

// endpoint returns the site's own URL at path, the origin its client calls
func (m *MangaHub) endpoint(path, rawQuery string) (string, error) {
	base, err := url.Parse(m.URL)
	if err != nil {
		return "", err
	}

	return (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: path, RawQuery: rawQuery}).String(), nil
}

// ensureKeys issues the session's page key on the run's first chapter
func (m *MangaHub) ensureKeys(access, warm string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.keys) > 0 {
		return nil
	}

	return m.issueKey(access, warm)
}

// pageKey returns the key of an envelope's key id, issuing the session's key
// again when the run doesn't hold it yet
func (m *MangaHub) pageKey(keyID, access, warm string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key, ok := m.keys[keyID]; ok {
		return key, nil
	}
	if err := m.issueKey(access, warm); err != nil {
		return nil, err
	}
	if key, ok := m.keys[keyID]; ok {
		return key, nil
	}

	return nil, fmt.Errorf("the site issued no key for %s", keyID)
}

// issueKey fetches the session's page key into m.keys. The endpoint answers 409
// until a GraphQL call has run on the session, so warm runs one and the key is
// asked for once more. The caller holds m.mu.
func (m *MangaHub) issueKey(access, warm string) error {
	id, key, err := m.fetchKey(access)
	var status *http.StatusError
	if errors.As(err, &status) && status.Code == 409 {
		var discard json.RawMessage
		if err := m.query(access, warm, &discard); err != nil {
			return err
		}
		id, key, err = m.fetchKey(access)
	}
	if err != nil {
		return err
	}

	if m.keys == nil {
		m.keys = map[string][]byte{}
	}
	m.keys[id] = key

	return nil
}

// fetchKey asks the site for its page key, the 32-byte AES-256 key base64url encoded
func (m *MangaHub) fetchKey(access string) (string, []byte, error) {
	endpoint, err := m.endpoint("/api/chapter-crypto", "")
	if err != nil {
		return "", nil, err
	}
	body, err := http.GetText(http.RequestParams{
		URL:     endpoint,
		Accept:  "application/json",
		Referer: m.URL,
		Headers: map[string]string{"x-mhub-access": access},
	})
	if err != nil {
		return "", nil, err
	}

	var resp struct {
		KeyID string `json:"keyId"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return "", nil, err
	}
	key, err := base64.RawURLEncoding.DecodeString(resp.Key)
	if err != nil || resp.KeyID == "" || len(key) != 32 {
		return "", nil, errors.New("the site issued no usable page key")
	}

	return resp.KeyID, key, nil
}

// decryptPages returns the page list, decrypted when the site encrypted it
func (m *MangaHub) decryptPages(raw, access, warm string) (string, error) {
	if !strings.HasPrefix(raw, "enc:v1:") {
		return raw, nil
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 6 {
		return "", errors.New("the page list envelope is malformed")
	}
	key, err := m.pageKey(parts[2], access, warm)
	if err != nil {
		return "", err
	}

	return mangahubOpen(parts, key)
}

// mangahubOpen decrypts an envelope's parts (key id, IV, auth tag, ciphertext, each
// base64url) with AES-256-GCM. The tag is appended to the ciphertext, as GCM expects
func mangahubOpen(parts []string, key []byte) (string, error) {
	iv, errIV := base64.RawURLEncoding.DecodeString(parts[3])
	tag, errTag := base64.RawURLEncoding.DecodeString(parts[4])
	data, errData := base64.RawURLEncoding.DecodeString(parts[5])
	if errIV != nil || errTag != nil || errData != nil || len(iv) != 12 || len(tag) != 16 {
		return "", errors.New("the page list envelope is malformed")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, iv, append(data, tag...), nil)
	if err != nil {
		return "", errors.New("could not decrypt the page list")
	}

	return string(plain), nil
}

// query runs one GraphQL query on the site's API with the access key and decodes its data into out
func (m *MangaHub) query(key, q string, out any) error {
	endpoint, err := m.endpoint("/graphql", url.Values{"query": {q}}.Encode())
	if err != nil {
		return err
	}

	body, err := http.GetText(http.RequestParams{
		URL:     endpoint,
		Accept:  "application/json",
		Headers: map[string]string{"x-mhub-access": key},
	})
	if err != nil {
		return err
	}

	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return fmt.Errorf("mangahub API: %s", resp.Errors[0].Message)
	}

	return json.Unmarshal(resp.Data, out)
}

// slug returns the series slug of the series URL
func (m *MangaHub) slug() (string, error) {
	match := mangahubSlugRe.FindStringSubmatch(m.URL)
	if match == nil {
		return "", fmt.Errorf("could not find the series slug in url %s", m.URL)
	}

	return match[1], nil
}

// mangahubPages returns the image URLs of a chapter from its page list, a JSON
// object holding a path prefix ("p") and the file names under it ("i")
func mangahubPages(raw string) ([]string, error) {
	if strings.HasPrefix(raw, "enc:") {
		return nil, errors.New("the site encrypts its pages in a format that isn't supported")
	}

	var list struct {
		Prefix string   `json:"p"`
		Images []string `json:"i"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("could not read the page list: %w", err)
	}
	if len(list.Images) == 0 {
		return nil, errors.New("the chapter has no pages")
	}

	pages := make([]string, 0, len(list.Images))
	for _, img := range list.Images {
		pages = append(pages, mangahubCDN+strings.TrimPrefix(list.Prefix, "/")+img)
	}

	return pages, nil
}
