// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// The madaradex reader script sets a random `mdx_fp` cookie and has
// admin-ajax.php mint an `mdx_auth` for it; the image CDN 403s without both.
var (
	madaradexAjaxRe   = regexp.MustCompile(`const ajaxUrl = "([^"]+)"`)
	madaradexDomainRe = regexp.MustCompile(`const configuredDomain = "\.?([^"]+)"`)

	madaradexMu sync.Mutex
	// madaradexDone holds when each site's handshake last ran
	madaradexDone = map[string]time.Time{}
)

// madaradexAuthTTL renews the handshake before the cookie's six hours run out,
// since the desktop app keeps the session for as long as it stays open.
const madaradexAuthTTL = 5 * time.Hour

// authorizeMadaradex does the reader's cookie handshake once per TTL, so the
// session sends both cookies to the image CDN. It's a no-op on other pages.
func authorizeMadaradex(doc *goquery.Document) {
	html := ""
	doc.Find("script").EachWithBreak(func(_ int, sc *goquery.Selection) bool {
		if text := sc.Text(); strings.Contains(text, "mdx_auth_refresh") {
			html = text
			return false
		}
		return true
	})
	if html == "" {
		return
	}
	ajax := madaradexAjaxRe.FindStringSubmatch(html)
	domain := madaradexDomainRe.FindStringSubmatch(html)
	if ajax == nil || domain == nil {
		return
	}
	endpoint := strings.ReplaceAll(ajax[1], `\/`, `/`)

	madaradexMu.Lock()
	defer madaradexMu.Unlock()
	if at, ok := madaradexDone[endpoint]; ok && time.Since(at) < madaradexAuthTTL {
		return
	}

	fp := make([]byte, 16)
	if _, err := rand.Read(fp); err != nil {
		return
	}
	http.SetCookie(domain[1], "mdx_fp", hex.EncodeToString(fp))
	body, err := http.Post(http.RequestParams{
		URL:  endpoint,
		Form: url.Values{"action": {"mdx_auth_refresh"}},
	})
	if err != nil {
		return
	}
	body.Close()
	madaradexDone[endpoint] = time.Now()
}
