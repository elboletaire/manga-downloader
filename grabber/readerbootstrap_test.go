// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// bootstrapSite records what a bootstrap server was sent
type bootstrapSite struct {
	mu           sync.Mutex
	posts        []string
	imageCookies []string
}

// the expected values come from the reader's own hash, run in node
func TestBrgSimpleHash(t *testing.T) {
	for in, want := range map[string]string{
		"15693|1792360323|8f790f68f7937667": "43deb01e",
		"15693|1792352447|abc":              "4ee76ad7",
		"x":                                 "2b61d",
		"brg|10":                            "10b3df323",
	} {
		if got := brgSimpleHash(in); got != want {
			t.Errorf("brgSimpleHash(%q) = %s, want %s", in, got, want)
		}
	}
}

// bootstrapServer serves a reader page's bootstrap script, answers its POST
// with a cookie and records the bodies and cookies the requests carried
func bootstrapServer(t *testing.T, refuse bool) (*httptest.Server, *bootstrapSite) {
	t.Helper()
	site := &bootstrapSite{}
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/reader-bootstrap/15693.js", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		fmt.Fprint(w, "(function(){ var t = 1792360323;\n var s = '8f790f68f7937667';\n var g = '0c4f1f02';\n var ch_id = 15693; })();")
	})
	mux.HandleFunc("/reader-bootstrap/15693", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		body, _ := io.ReadAll(r.Body)
		site.mu.Lock()
		site.posts = append(site.posts, string(body))
		site.mu.Unlock()
		if refuse {
			fmt.Fprint(w, `{"success":false,"data":"no"}`)
			return
		}
		stdhttp.SetCookie(w, &stdhttp.Cookie{Name: "brg_ch_15693", Value: "token1", Path: "/"})
		fmt.Fprint(w, `{"success":true,"data":"ok"}`)
	})
	mux.HandleFunc("/reader-image/", func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		site.mu.Lock()
		site.imageCookies = append(site.imageCookies, r.Header.Get("Cookie"))
		site.mu.Unlock()
		fmt.Fprint(w, "page")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, site
}

// bootstrapPage is a reader page that loads the bootstrap script
func bootstrapPage(t *testing.T, base string) *goquery.Document {
	t.Helper()
	return docFromHTML(t, fmt.Sprintf(`<html><head><script src="%s/reader-bootstrap/15693.js?ver=1" data-brg-bootstrap="15693"></script></head><body></body></html>`, base))
}

func TestAuthenticateReaderPostsTheHandshake(t *testing.T) {
	srv, site := bootstrapServer(t, false)
	doc := bootstrapPage(t, srv.URL)

	if err := authenticateReader(doc, srv.URL+"/chapter/1/"); err != nil {
		t.Fatalf("authenticateReader() error = %v", err)
	}
	site.mu.Lock()
	defer site.mu.Unlock()
	if len(site.posts) != 1 {
		t.Fatalf("made %d handshake posts, want 1", len(site.posts))
	}
	var got struct {
		T   int64  `json:"t"`
		S   string `json:"s"`
		G   string `json:"g"`
		Sol string `json:"sol"`
	}
	if err := json.Unmarshal([]byte(site.posts[0]), &got); err != nil {
		t.Fatal(err)
	}
	if got.T != 1792360323 || got.S != "8f790f68f7937667" || got.G != "0c4f1f02" || got.Sol != brgSimpleHash("15693|1792360323|8f790f68f7937667") {
		t.Errorf("handshake body = %+v", got)
	}
}

// the cookie the handshake answers with reaches the chapter's image requests
func TestAuthenticateReaderCookieReachesImages(t *testing.T) {
	srv, site := bootstrapServer(t, false)
	if err := authenticateReader(bootstrapPage(t, srv.URL), srv.URL+"/chapter/1/"); err != nil {
		t.Fatalf("authenticateReader() error = %v", err)
	}
	body, err := http.Get(http.RequestParams{URL: srv.URL + "/reader-image/15693/a/b.webp", DisableRetry: true})
	if err != nil {
		t.Fatal(err)
	}
	body.Close()

	site.mu.Lock()
	defer site.mu.Unlock()
	if n := len(site.imageCookies); n == 0 || !strings.Contains(site.imageCookies[n-1], "brg_ch_15693=token1") {
		t.Errorf("image request carried cookie %q, want brg_ch_15693=token1", site.imageCookies)
	}
}

func TestAuthenticateReaderRefused(t *testing.T) {
	srv, _ := bootstrapServer(t, true)
	if err := authenticateReader(bootstrapPage(t, srv.URL), srv.URL+"/chapter/1/"); err == nil {
		t.Error("authenticateReader() accepted a refused handshake")
	}
}

// a page without the bootstrap script needs no handshake
func TestAuthenticateReaderNoScript(t *testing.T) {
	doc := docFromHTML(t, `<html><body><img src="x.webp"></body></html>`)
	if err := authenticateReader(doc, "https://example.com/chapter/1/"); err != nil {
		t.Errorf("authenticateReader() error = %v on a page without the script", err)
	}
}
