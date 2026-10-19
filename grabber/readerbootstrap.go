// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
)

// Some readers (bloomscans) serve a chapter's images only after its bootstrap
// script posts a check and gets a per-chapter cookie. This makes the same POST;
// the session store then sends the cookie with the image requests.
var brgVarsRe = regexp.MustCompile(`var t = (\d+);\s*var s = '([0-9a-f]+)';\s*var g = '([0-9a-f]+)';\s*var ch_id = (\d+);`)

// authenticateReader runs the bootstrap handshake of a chapter page that loads
// one (a script with data-brg-bootstrap), and does nothing for other pages
func authenticateReader(doc *goquery.Document, chapterURL string) error {
	src, ok := doc.Find("script[data-brg-bootstrap]").First().Attr("src")
	if !ok {
		return nil
	}
	base, err := url.Parse(chapterURL)
	if err != nil {
		return err
	}
	scriptURL, err := base.Parse(src)
	if err != nil {
		return err
	}
	js, err := http.GetText(http.RequestParams{URL: scriptURL.String(), Referer: chapterURL, DisableRetry: true})
	if err != nil {
		return err
	}
	m := brgVarsRe.FindStringSubmatch(js)
	if m == nil {
		return errors.New("the bootstrap script has no handshake values")
	}
	t, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return err
	}

	origin := base.Scheme + "://" + base.Host
	body, err := json.Marshal(struct {
		T   int64  `json:"t"`
		S   string `json:"s"`
		G   string `json:"g"`
		Sol string `json:"sol"`
	}{t, m[2], m[3], brgSimpleHash(m[4] + "|" + m[1] + "|" + m[2])})
	if err != nil {
		return err
	}
	text, err := http.PostText(http.RequestParams{
		URL:          origin + "/reader-bootstrap/" + m[4],
		Body:         string(body),
		Origin:       origin,
		Referer:      chapterURL,
		Headers:      map[string]string{"Content-Type": "application/json"},
		DisableRetry: true,
	})
	if err != nil {
		return err
	}
	var res struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil || !res.Success {
		return fmt.Errorf("the reader refused the handshake: %s", text)
	}
	return nil
}

// brgSimpleHash is the reader's djb2 variant, in lowercase hex. The reader
// takes Math.abs(h - 2^32) when the sign bit is set, which is 2^33 - h, not
// 2^32 - h, so that case is kept as the page computes it.
func brgSimpleHash(str string) string {
	var h uint32 = 5381
	for i := range len(str) {
		h = h*33 + uint32(str[i])
	}
	v := uint64(h)
	if h&0x80000000 != 0 {
		v = 1<<33 - uint64(h)
	}
	return strconv.FormatUint(v, 16)
}
