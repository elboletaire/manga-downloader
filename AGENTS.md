# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go CLI (cobra) that downloads manga chapters from supported websites and packs them into CBZ files. Module: `github.com/elboletaire/manga-downloader`, Go 1.19.

## Commands

```bash
make install          # go mod download
make build            # clean + test + build unix binary (injects version via ldflags)
make build/all        # unix + windows binaries
make test             # go test -v ./... (uses richgo if installed)
go test ./ranges -run TestParse   # run a single test
make clean            # remove built binaries and *.cbz files
```

Unit test coverage is minimal (only `ranges/parser_test.go`). Real-world verification is done with the makefile smoke targets, which run the app against live sites:

```bash
make grabber          # all of the below
make grabber/inmanga
make grabber/mangadex
make grabber/mangabats
make grabber/tcb
make grabber/html     # runs a list of plainhtml-based sites
```

These hit live websites and download actual chapters, so run them selectively.

## Architecture

The download flow, all orchestrated from `cmd/root.go` (`Run`):

1. `grabber.NewSite(url, settings)` → `IdentifySite()` iterates the registered grabbers (`grabber/site.go`) and calls `Test()` on each, which fetches the URL and checks if the site matches. **Order matters**: `PlainHTML` is tried first, then site-specific grabbers (Inmanga, Mangadex, Tcb).
2. The matched `Site` fetches title and chapters (`FetchChapters` returns `Filterables`), the user's range argument (`ranges.Parse`, format `1-10,12,15-20`) filters them.
3. Chapters download concurrently (`downloader.FetchChapter`), bounded by `--concurrency` (chapters, max 5) and `--concurrency-pages` (max 10) semaphores. Pages are fetched via the `http` package (plain GETs with a Referer header).
4. `packer` writes CBZ files (`PackSingle` per chapter or `PackBundle` with `--bundle`), naming them via a Go text/template (`--filename-template`, parts in `packer/filename.go`). Duplicate filenames get a `v{{.Version}}` suffix instead of being overwritten.

### The grabber package

- `Site` interface (`grabber/site.go`) is the contract for all grabbers: `Test`, `FetchTitle`, `FetchChapters`, `FetchChapter`, etc. The base `Grabber` struct provides shared settings/helpers.
- `Filterable`/`Filterables` (`grabber/filterable.go`) abstracts chapters so they can be sorted/filtered by number; each grabber has its own chapter struct embedding `grabber.Chapter`.
- `PlainHTML` (`grabber/plainhtml.go`) is a generic goquery scraper driven by a list of `SiteSelector` entries (CSS selectors for title/rows/chapter/image). It covers mangabuddy, tcbonepiecechapters.com (TCB Scans) and asurascans.com. Selector order in the list matters because some sites have very similar markup.
- Site-specific grabbers with their own logic: `inmanga.go`, `mangadex.go` (both API-based, language-aware), `mangabats.go` (JSON chapters API + js image variables), `mangafire.go` (open JSON API: `/api/titles/{hid}` + paginated `/chapters` + `/api/chapters/{id}` for pages, language-aware), `qimanga.go` (paginated JSON chapters API + SSR reader), `tcb.go` (Madara-based wordpress sites, i.e. lhtranslation.net).

### The browser package

`browser/browser.go` drives a locally-installed Chromium browser (Chrome/Chromium/Brave/Edge, auto-discovered by chromedp's `ExecAllocator`) for sites plain HTTP can't scrape: JS-rendered pages, TLS-fingerprint blocks, or Cloudflare challenges. One shared browser process is started lazily and reused. `GetHTML(url, waitSelector, timeout)` returns rendered HTML and harvests the session cookies + real UA into the `http` package (`http/session.go`), so images still download via the fast plain-HTTP path reusing the browser's Cloudflare clearance.

`grabber/plainhtmlbrowser.go` (`PlainHTMLBrowser`) is the browser-rendered twin of `PlainHTML`: it reuses all the selector-driven parsing (`chapterFromDoc`), but matches sites **by domain** (a `browserSelectors` list of `BrowserSiteSelector`, no fetch — starting a browser is expensive) and fetches series/reader pages through Chrome. Registered first in `IdentifySite()`. Covers toongod, dragontea, kappabeast, sushiscan.

- **Headless loses to Cloudflare; headed usually wins.** CDP automation is detectable, so CF challenges time out in headless mode. `browser.GetHTML` handles this automatically: it tries a short headless probe (`headlessProbeTimeout`) and, if the wait selector times out (a `challengeError`), tears down the headless browser and reopens a visible one (`goVisible`), where managed challenges resolve in a couple of seconds and the user can solve an interactive one manually. `--browser-visible` (`browser.SetVisible`) just forces headed from the start, skipping the wasted headless probe.
- **Always try the site's own API/HTTP first.** mangafire looks like it needs a browser (SPA behind CF), but its JSON API is wide open to plain HTTP — no browser needed. zonatmo's successor domain dropped the TLS block, so it's plain `PlainHTML` now. Only reach for `PlainHTMLBrowser` when HTTP genuinely can't get the data.
- **`tools/probe`** is the investigation tool: renders a URL in Chrome and tests selectors. Env knobs: `PROBE_VISIBLE=1` (headed), `PROBE_SLEEP=8s` (settle time for lazy SPAs), `PROBE_DUMP=/tmp/x.html` (dump HTML), `PROBE_NETLOG=1` (log non-asset network responses — how the mangafire `/api/chapters/{id}` endpoint was found), `PROBE_FETCH_SEL="sel"` (grab first matching image and test a plain-HTTP download with the harvested session).
- **Sites keep moving/locking down.** zonatmo.com → taken down (Spanish police, Apr 2026) → successor zonatmo.org. colamanga → redirects to yoyomanga.com and now gates the reader behind their app (`APP观看该内容`) on top of client-side image encryption — not feasible, left unsupported (#30).

### Adding support for a new site

- If the site is plain HTML (chapter list + images in the reader page): add a `SiteSelector` entry to the list in `PlainHTML.Test()`, and add a smoke-test URL to the `grabber/html` makefile target and the README's supported sites list.
- Otherwise: create a new grabber implementing `Site` (embed `*Grabber`), and register it in `IdentifySite()`.

### Investigating/testing sites

Lessons from past site-support work (July 2026):

- **Triage with curl before touching Go.** Fetch the manga page with a browser User-Agent and check the `<title>`: "Just a moment..." means a Cloudflare JS challenge and the site cannot be supported with plain HTTP (same for TLS-level connection drops, empty-`<div id="app-root">` SPAs, or encrypted-image readers like colamanga). Don't burn time debugging the grabber in those cases.
- **Verify CBZ contents, not just their existence.** A run can "succeed" while producing empty or junk archives. Check that entries have non-zero sizes and real image magic bytes (JPG/PNG/WebP/GIF).
- **403/500 in the app but 200 in curl almost always means headers.** `http/request.go` sends a browser User-Agent, Accept, Accept-Language, and referers with a trailing slash — all four exist because some site rejected requests without them. Compare headers before assuming a site is down.
- **Smoke-test with recent chapters.** Some sites drop old content from their CDNs (inmanga 404s on early One Piece chapters) and MangaDex replaces licensed chapters with pageless external stubs, so "chapter 1" is often the worst test target.
- **Sites move domains constantly.** Before declaring a site dead, probe for successors (e.g. tcbscans.com → tcbonepiecechapters.com, mangabat → mangabats.com, asuratoon → asurascans.com); redirects may only preserve the homepage, not series paths.
- The makefile is tracked as lowercase `makefile`: on macOS's case-insensitive filesystem editing `Makefile` works, but `git add` needs the lowercase name.
- macOS has no `timeout` command; use the Bash tool's timeout or a background-kill wrapper when running live-site smoke tests.
