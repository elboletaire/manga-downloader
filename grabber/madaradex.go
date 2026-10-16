// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/elboletaire/manga-downloader/http"
)

// The madaradex reader script sets a random `mdx_fp` cookie and has
// admin-ajax.php mint an `mdx_auth` for it; the image CDN 403s without both.
var (
	madaradexAjaxRe   = regexp.MustCompile(`const ajaxUrl = "([^"]+)"`)
	madaradexDomainRe = regexp.MustCompile(`const configuredDomain = "\.?([^"]+)"`)

	madaradexMu   sync.Mutex
	madaradexDone = map[string]bool{}
)

// authorizeMadaradex does the reader's cookie handshake once per site, so the
// session sends both cookies to the image CDN. It's a no-op on other pages.
func authorizeMadaradex(html string) {
	if !strings.Contains(html, "mdx_auth_refresh") {
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
	if madaradexDone[endpoint] {
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
	madaradexDone[endpoint] = true
}
