// Probe is a throwaway dev tool to inspect what the browser package sees on a
// given URL. Not part of the app.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/browser"
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

	browser.Close()
}
