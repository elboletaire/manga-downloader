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

	start := time.Now()
	html, err := browser.GetHTML(url, wait, 45*time.Second)
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

	browser.Close()
}
