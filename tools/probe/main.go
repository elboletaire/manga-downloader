// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Probe is a throwaway dev tool to inspect what the browser package sees on a
// given URL. Not part of the app.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/http"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: probe <url> [waitSelector] [querySelector...]")
		os.Exit(1)
	}
	url := os.Args[1]
	wait := ""
	if len(os.Args) > 2 {
		wait = os.Args[2]
	}

	if os.Getenv("PROBE_VISIBLE") == "1" {
		browser.SetVisible(true)
	}
	timeout := 45 * time.Second
	if s := os.Getenv("PROBE_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			timeout = d
		}
	}
	if s := os.Getenv("PROBE_SLEEP"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			browser.SetSettle(d)
		}
	}
	if os.Getenv("PROBE_NETLOG") == "1" {
		browser.NetLog = func(u string, status int, mime string) {
			if strings.Contains(mime, "image") || strings.Contains(mime, "css") || strings.Contains(mime, "font") || strings.Contains(mime, "javascript") {
				return
			}
			fmt.Printf("NET %d %s %s\n", status, mime, u)
		}
	}

	start := time.Now()
	var html string
	var err error
	if s := os.Getenv("PROBE_SCROLL"); s != "" {
		iterations := 20
		fmt.Sscanf(s, "%d", &iterations)
		pause := 400 * time.Millisecond
		if p := os.Getenv("PROBE_SCROLL_PAUSE"); p != "" {
			if d, perr := time.ParseDuration(p); perr == nil {
				pause = d
			}
		}
		html, err = browser.GetHTMLWithScroll(url, wait, iterations, pause, timeout)
	} else {
		html, err = browser.GetHTML(url, wait, timeout)
	}
	fmt.Printf("elapsed: %s\n", time.Since(start))
	if err != nil {
		fmt.Println("ERROR:", err)
		browser.Close()
		os.Exit(1)
	}

	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	fmt.Println("page title:", strings.TrimSpace(doc.Find("title").Text()))
	fmt.Println("html size:", len(html))

	for _, sel := range os.Args[3:] {
		matches := doc.Find(sel)
		fmt.Printf("selector %q → %d matches\n", sel, matches.Length())
		matches.Slice(0, min(3, matches.Length())).Each(func(i int, s *goquery.Selection) {
			h, _ := goquery.OuterHtml(s)
			if len(h) > 300 {
				h = h[:300] + "..."
			}
			fmt.Printf("  [%d] %s\n", i, strings.TrimSpace(h))
		})
	}

	if f := os.Getenv("PROBE_DUMP"); f != "" {
		os.WriteFile(f, []byte(html), 0644)
		fmt.Println("dumped to", f)
	}

	// PROBE_FETCH_SEL: grab the first image matching the selector and try to
	// download it via plain HTTP with the harvested browser session
	if sel := os.Getenv("PROBE_FETCH_SEL"); sel != "" {
		img := doc.Find(sel).First()
		src := img.AttrOr("src", img.AttrOr("data-src", ""))
		src = strings.TrimSpace(src)
		fmt.Printf("fetching %q via plain HTTP...\n", src)
		if src != "" {
			body, err := http.Get(http.RequestParams{URL: src, Referer: url})
			if err != nil {
				fmt.Println("  FETCH ERROR:", err)
			} else {
				data, _ := io.ReadAll(body)
				body.Close()
				head := data
				if len(head) > 16 {
					head = head[:16]
				}
				fmt.Printf("  got %d bytes, magic: %q\n", len(data), head)
			}
		}
	}

	// PROBE_API_URL: after the browser session is harvested, try fetching an
	// arbitrary URL (e.g. a JSON API endpoint) via plain HTTP with the
	// harvested cookies/UA, to check whether the session carries over to
	// another host (e.g. an api. subdomain)
	if apiURLs := os.Getenv("PROBE_API_URL"); apiURLs != "" {
		for _, apiURL := range strings.Split(apiURLs, ",") {
			fmt.Printf("fetching %q via plain HTTP with harvested session...\n", apiURL)
			body, err := http.Get(http.RequestParams{URL: apiURL, Referer: url})
			if err != nil {
				fmt.Println("  API FETCH ERROR:", err)
				continue
			}
			data, _ := io.ReadAll(body)
			body.Close()
			if dir := os.Getenv("PROBE_API_DUMP_DIR"); dir != "" {
				fname := strings.NewReplacer("/", "_", ":", "_", "?", "_", "&", "_").Replace(apiURL)
				os.WriteFile(dir+"/"+fname+".json", data, 0644)
			}
			out := data
			if len(out) > 800 {
				out = out[:800]
			}
			fmt.Printf("  got %d bytes: %s\n", len(data), out)
		}
	}

	browser.Close()
}
