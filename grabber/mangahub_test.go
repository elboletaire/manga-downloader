// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// mangahubTestKey is the access key the fake site issues in its mhub_access cookie
const mangahubTestKey = "issued-by-the-page"

// mangahubTestKeyID and mangahubTestPageKey are the page key the fake site issues
// and seals its encrypted page lists with
const mangahubTestKeyID = "k1"

var mangahubTestPageKey = []byte("0123456789abcdef0123456789abcdef")

// mangahubSite serves a fake MangaHub mirror: its pages, client bundle and GraphQL
// API, for one series of three chapters, answering only the key the pages issue.
// The page key is issued only after a GraphQL call on the session.
func mangahubSite(t *testing.T) *httptest.Server {
	t.Helper()

	var srv *httptest.Server
	warmed := false
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// the API sits on the pages' origin, as the client has it when apiUrl is empty
		u, _ := url.Parse(srv.URL)
		host := u.Hostname()
		switch {
		case r.URL.Path == "/manga/red-storm_111", r.URL.Path == "/chapter/red-storm_111/chapter-482":
			http.SetCookie(w, &http.Cookie{Name: "mhub_access", Value: mangahubTestKey, Path: "/"})
			fmt.Fprint(w, mangahubPage("/assets/client.abc123.js"))
		case r.URL.Path == "/manga/nocookie":
			// the same page without the visitor cookie the site would issue
			fmt.Fprint(w, mangahubPage("/assets/client.abc123.js"))
		case r.URL.Path == "/manga/other-site":
			fmt.Fprint(w, mangahubPage("/assets/client.other.js"))
		case r.URL.Path == "/assets/client.abc123.js":
			// every site's config sits in the one bundle, the decoy first
			fmt.Fprintf(w, `x={domain:"other.example",sitename:"Other",dataSourceKey:"zz99"},`+
				`y={domain:"%s",sitename:"Fake",siteShortName:"Fake",dataSourceKey:"mn03",primaryColor:"#f05759"}`, host)
		case r.URL.Path == "/assets/client.other.js":
			fmt.Fprint(w, `x={domain:"other.example",sitename:"Other",dataSourceKey:"zz99"}`)
		case r.URL.Path == "/graphql":
			if r.Header.Get("x-mhub-access") != mangahubTestKey {
				http.Error(w, "missing or wrong key", http.StatusForbidden)
				return
			}
			warmed = true
			fmt.Fprint(w, mangahubAPIResponse(r.URL.Query().Get("query")))
		case r.URL.Path == "/api/chapter-crypto":
			if r.Header.Get("x-mhub-access") != mangahubTestKey {
				http.Error(w, "missing or wrong key", http.StatusForbidden)
				return
			}
			if !warmed {
				http.Error(w, `{"success":false,"error":"session_changed"}`, http.StatusConflict)
				return
			}
			fmt.Fprintf(w, `{"keyId":%q,"key":%q,"expiresAt":0}`,
				mangahubTestKeyID, base64.RawURLEncoding.EncodeToString(mangahubTestPageKey))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

// mangahubPage is a series page carrying the API config and the client bundle at src
func mangahubPage(src string) string {
	return `<html><script>window.App={"apiUrl":"","chapterCryptoEnabled":true}</script>` +
		`<script src="` + src + `"></script><h1>Red Storm</h1></html>`
}

// mangahubAPIResponse answers a query the way the API does: the manga with its
// chapters, or one chapter's pages, keyed by the source and the slug it's asked for
func mangahubAPIResponse(query string) string {
	switch {
	case strings.Contains(query, "manga(x: mn03, slug: \"red-storm_111\")"):
		return `{"data":{"manga":{"title":"Red Storm","slug":"red-storm_111","chapters":[` +
			`{"number":482,"title":"Chapter 482"},` +
			`{"number":388.5,"title":"  Epilogue\n"},` +
			`{"number":0,"title":"Chapter 0"}]}}}`
	case strings.Contains(query, "chapter(x: mn03, slug: \"red-storm_111\", number: 388.5)"):
		return `{"data":{"chapter":{"pages":"{\"p\":\"red-storm/388.5/\",\"i\":[\"1.jpg\",\"2.jpg\"]}"}}}`
	case strings.Contains(query, "chapter(x: mn03, slug: \"red-storm_111\", number: 482)"):
		return `{"data":{"chapter":{"pages":"{\"p\":\"/red-storm/482/\",\"i\":[\"1.jpg\"]}"}}}`
	case strings.Contains(query, "chapter(x: mn03, slug: \"red-storm_111\", number: 0)"):
		return `{"data":{"chapter":{"pages":"` + mangahubSeal(`{"p":"red-storm/0/","i":["1.jpg"]}`) + `"}}}`
	default:
		return `{"data":null,"errors":[{"message":"Cannot read properties of undefined"}]}`
	}
}

// mangahubSeal encrypts a page list the way the site does, in its enc:v1 envelope.
// The key is 32 bytes and the IV 12, so the cipher can't fail
func mangahubSeal(plain string) string {
	block, _ := aes.NewCipher(mangahubTestPageKey)
	gcm, _ := cipher.NewGCM(block)
	iv := []byte("123456789012")
	sealed := gcm.Seal(nil, iv, []byte(plain), nil)
	data, tag := sealed[:len(sealed)-gcm.Overhead()], sealed[len(sealed)-gcm.Overhead():]

	enc := base64.RawURLEncoding.EncodeToString
	return strings.Join([]string{"enc", "v1", mangahubTestKeyID, enc(iv), enc(tag), enc(data)}, ":")
}

// TestMangaHubFetchEncryptedChapter decrypts an enc:v1 page list, which the
// session's key is issued for only after a GraphQL call, and fetches it again
// from the cached key
func TestMangaHubFetchEncryptedChapter(t *testing.T) {
	srv := mangahubSite(t)
	m := NewMangaHub(&Grabber{URL: srv.URL + "/manga/red-storm_111"})

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	for range 2 {
		chapter, err := m.FetchChapter(chapters[2])
		if err != nil {
			t.Fatal(err)
		}
		if len(chapter.Pages) != 1 || chapter.Pages[0].URL != mangahubCDN+"red-storm/0/1.jpg" {
			t.Fatalf("pages = %+v, want red-storm/0/1.jpg", chapter.Pages)
		}
	}
}

// TestMangaHubOpen checks the envelope round trip, and that a wrong key or a
// damaged envelope fails instead of returning garbage
func TestMangaHubOpen(t *testing.T) {
	envelope := mangahubSeal(`{"p":"s/1/","i":["1.jpg"]}`)
	parts := strings.Split(envelope, ":")

	plain, err := mangahubOpen(parts, mangahubTestPageKey)
	if err != nil || plain != `{"p":"s/1/","i":["1.jpg"]}` {
		t.Fatalf("mangahubOpen() = %q, %v", plain, err)
	}
	if _, err := mangahubOpen(parts, bytes.Repeat([]byte{'x'}, 32)); err == nil {
		t.Error("expected a wrong key to fail")
	}
	damaged := append([]string(nil), parts...)
	damaged[4] = "AAAA"
	if _, err := mangahubOpen(damaged, mangahubTestPageKey); err == nil {
		t.Error("expected a short auth tag to fail")
	}
}

// TestMangaHubTestMatchesMarkup checks the platform is recognised by the API
// config its pages carry, not by their domain
func TestMangaHubTestMatchesMarkup(t *testing.T) {
	srv := mangahubSite(t)

	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"series page", srv.URL + "/manga/red-storm_111", true},
		{"reader page", srv.URL + "/chapter/red-storm_111/chapter-482", true},
		{"other path never fetched", srv.URL + "/search", false},
		{"page without the config", srv.URL + "/manga/missing", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewMangaHub(&Grabber{URL: c.url})
			ok, err := m.Test()
			if err != nil {
				t.Fatal(err)
			}
			if ok != c.want {
				t.Errorf("Test() = %v, want %v", ok, c.want)
			}
		})
	}
}

// TestMangaHubResolveSeriesURL covers the reader URLs that load their series
func TestMangaHubResolveSeriesURL(t *testing.T) {
	cases := map[string]string{
		"https://mangahub.io/chapter/red-storm_111/chapter-482":           "https://mangahub.io/manga/red-storm_111",
		"https://WWW.1manga.co/chapter/red-storm_111/chapter-388.5?x=1#y": "https://WWW.1manga.co/manga/red-storm_111",
		"https://mangahub.io/manga/red-storm_111":                         "https://mangahub.io/manga/red-storm_111",
	}
	for in, want := range cases {
		m := NewMangaHub(&Grabber{URL: in})
		m.resolveSeriesURL()
		if m.URL != want {
			t.Errorf("resolveSeriesURL(%s) = %s, want %s", in, m.URL, want)
		}
	}
}

// TestMangaHubFetchChapters reads the chapter list from the API, decimal
// numbers included, and leaves a title that only names the number to the label
func TestMangaHubFetchChapters(t *testing.T) {
	srv := mangahubSite(t)
	m := NewMangaHub(&Grabber{URL: srv.URL + "/chapter/red-storm_111/chapter-482"})

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(chapters) != 3 {
		t.Fatalf("got %d chapters, want 3", len(chapters))
	}

	want := []struct {
		number float64
		title  string
	}{{482, ""}, {388.5, "Epilogue"}, {0, ""}}
	for i, w := range want {
		c := chapters[i].(*MangaHubChapter)
		if c.Number != w.number || c.Title != w.title {
			t.Errorf("chapter %d = (%v, %q), want (%v, %q)", i, c.Number, c.Title, w.number, w.title)
		}
	}

	title, err := m.FetchTitle()
	if err != nil || title != "Red Storm" {
		t.Errorf("FetchTitle() = %q, %v; want Red Storm", title, err)
	}
}

// TestMangaHubFetchChapter fetches a decimal chapter's pages under its prefix
func TestMangaHubFetchChapter(t *testing.T) {
	srv := mangahubSite(t)
	m := NewMangaHub(&Grabber{URL: srv.URL + "/manga/red-storm_111"})

	chapters, errs := m.FetchChapters()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	chapter, err := m.FetchChapter(chapters[1])
	if err != nil {
		t.Fatal(err)
	}
	if chapter.PagesCount != 2 || len(chapter.Pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(chapter.Pages))
	}
	if chapter.Pages[0].URL != mangahubCDN+"red-storm/388.5/1.jpg" {
		t.Errorf("first page = %s", chapter.Pages[0].URL)
	}
	if chapter.Pages[1].Number != 2 {
		t.Errorf("second page number = %d, want 2", chapter.Pages[1].Number)
	}
}

// TestMangaHubAPIErrors fails loudly when the API reports an error or the
// chapter isn't there, instead of passing an empty page list on
func TestMangaHubAPIErrors(t *testing.T) {
	srv := mangahubSite(t)
	m := NewMangaHub(&Grabber{URL: srv.URL + "/manga/red-storm_111"})

	if _, err := m.FetchChapter(&MangaHubChapter{Chapter: Chapter{Number: 7}, source: "mn03", slug: "red-storm_111", key: mangahubTestKey}); err == nil {
		t.Error("expected an error for a chapter the API doesn't have")
	}
	if _, err := m.FetchChapter(&MangaHubChapter{Chapter: Chapter{Number: 482}, source: "zz99", slug: "red-storm_111", key: mangahubTestKey}); err == nil {
		t.Error("expected the API's error to surface")
	}
}

// TestMangaHubSiteCode reads the site code and the issued key, and fails when
// the client bundle has no entry for the host or the page issued no cookie
func TestMangaHubSiteCode(t *testing.T) {
	srv := mangahubSite(t)

	m := NewMangaHub(&Grabber{URL: srv.URL + "/manga/red-storm_111"})
	code, key, err := m.siteCode()
	if err != nil || code != "mn03" || key != mangahubTestKey {
		t.Fatalf("siteCode() = %q, %q, %v; want mn03, %s", code, key, err, mangahubTestKey)
	}

	other := NewMangaHub(&Grabber{URL: srv.URL + "/manga/other-site"})
	if _, _, err := other.siteCode(); err == nil || !strings.Contains(err.Error(), "no site code") {
		t.Errorf("expected no site code for a host missing from the bundle, got %v", err)
	}

	// localhost is a host no test page sets a cookie for, unlike 127.0.0.1
	bare := NewMangaHub(&Grabber{URL: strings.Replace(srv.URL, "127.0.0.1", "localhost", 1) + "/manga/nocookie"})
	if _, err := bare.FetchTitle(); err == nil || !strings.Contains(err.Error(), "mhub_access") {
		t.Errorf("expected a missing cookie error, got %v", err)
	}
}

// TestMangaHubPages reads the page list, refusing the encrypted and empty ones
func TestMangaHubPages(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{"prefix and files", `{"p":"red-storm/1/","i":["1.jpg","2.png"]}`,
			[]string{mangahubCDN + "red-storm/1/1.jpg", mangahubCDN + "red-storm/1/2.png"}, false},
		{"leading slash prefix", `{"p":"/s/1/","i":["1.jpg"]}`, []string{mangahubCDN + "s/1/1.jpg"}, false},
		{"encrypted", `enc:v1:k:iv:tag:data`, nil, true},
		{"no pages", `{"p":"s/1/","i":[]}`, nil, true},
		{"not JSON", `<html>`, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := mangahubPages(c.raw)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("pages = %v, want %v", got, c.want)
			}
		})
	}
}

// TestMangaHubTitleNumberRe checks which titles only name the chapter's number
func TestMangaHubTitleNumberRe(t *testing.T) {
	for title, want := range map[string]bool{
		"Chapter 482":   true,
		"chapter 388.5": true,
		"Chapter 0":     true,
		"Chapter 1 - X": false,
		"Epilogue":      false,
	} {
		if got := mangahubNumberTitleRe.MatchString(title); got != want {
			t.Errorf("%q matched %v, want %v", title, got, want)
		}
	}
}
