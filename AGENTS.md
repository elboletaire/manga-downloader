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
- Site-specific grabbers with their own logic: `inmanga.go`, `mangadex.go` (both API-based, language-aware), `mangabats.go` (JSON chapters API + js image variables), `mangafire.go` (open JSON API: `/api/titles/{hid}` + paginated `/chapters` + `/api/chapters/{id}` for pages, language-aware), `mangak.go` (mangak.io, the mangabuddy rebrand: Next.js site, full chapter list and page-image URLs parsed from the `__NEXT_DATA__` JSON blob — the visible HTML only lists ~6 chapters, and the JSON `number` field is a 1-based ordinal so the real number is parsed from the chapter name), `qimanga.go` (paginated JSON chapters API + SSR reader), `tcb.go` (Madara-based wordpress sites, i.e. lhtranslation.net), `leercapitulo.go` (plain HTML chapter list, but the reader page only decodes its obfuscated page-image blob client-side; uses `browser.GetHTMLWithLocalStorage` to flip the site's own "load all pages" preference before scraping the resulting `<img>` tags — no cloudflare, no cipher reversing needed).

### The browser package

`browser/browser.go` drives a locally-installed Chromium browser (Chrome/Chromium/Brave/Edge, auto-discovered by chromedp's `ExecAllocator`) for sites plain HTTP can't scrape: JS-rendered pages, TLS-fingerprint blocks, or Cloudflare challenges. One shared browser process is started lazily and reused. `GetHTML(url, waitSelector, timeout)` returns rendered HTML and harvests the session cookies + real UA into the `http` package (`http/session.go`), so images still download via the fast plain-HTTP path reusing the browser's Cloudflare clearance.

`grabber/plainhtmlbrowser.go` (`PlainHTMLBrowser`) is the browser-rendered twin of `PlainHTML`: it reuses all the selector-driven parsing (`chapterFromDoc`), but matches sites **by domain** (a `browserSelectors` list of `BrowserSiteSelector`, no fetch — starting a browser is expensive) and fetches series/reader pages through Chrome. Registered first in `IdentifySite()`. Covers toongod, dragontea, kappabeast, sushiscan, drakecomic.org.

- **Headless loses to Cloudflare; headed usually wins.** CDP automation is detectable, so CF challenges time out in headless mode. `browser.GetHTML` handles this automatically: it tries a short headless probe (`headlessProbeTimeout`) and, if the wait selector times out (a `challengeError`), tears down the headless browser and reopens a visible one (`goVisible`), where managed challenges resolve in a couple of seconds and the user can solve an interactive one manually. `--browser-visible` (`browser.SetVisible`) just forces headed from the start, skipping the wasted headless probe.
- **Always try the site's own API/HTTP first.** mangafire looks like it needs a browser (SPA behind CF), but its JSON API is wide open to plain HTTP — no browser needed. zonatmo's successor domain dropped the TLS block, so it's plain `PlainHTML` now. Only reach for `PlainHTMLBrowser` when HTTP genuinely can't get the data.
- **`tools/probe`** is the investigation tool: renders a URL in Chrome and tests selectors. Env knobs: `PROBE_VISIBLE=1` (headed), `PROBE_TIMEOUT=4m` (page-load timeout, default 45s — pair with a long `PROBE_SLEEP` to leave time for solving an interactive challenge manually), `PROBE_SLEEP=8s` (settle time for lazy SPAs), `PROBE_DUMP=/tmp/x.html` (dump HTML), `PROBE_NETLOG=1` (log non-asset network responses — how the mangafire `/api/chapters/{id}` endpoint was found), `PROBE_FETCH_SEL="sel"` (grab first matching image and test a plain-HTTP download with the harvested session), `PROBE_API_URL="url1,url2"` (after the browser clears a challenge, fetch arbitrary JSON API URLs via plain HTTP reusing the harvested session — checks whether cookies carry over to another host, e.g. an `api.` subdomain; pair with `PROBE_API_DUMP_DIR=/tmp/x` to save full bodies instead of a truncated preview).
- **Sites keep moving/locking down.** zonatmo.com → taken down (Spanish police, Apr 2026) → successor zonatmo.org. colamanga → redirects to yoyomanga.com and now gates the reader behind their app (`APP观看该内容`) on top of client-side image encryption — not feasible, left unsupported (#30). mangabuddy.com → rebranded as mangak.io (supported, see `mangak.go`); the old mangabuddy.com domain now redirects to comizy.io, a different JS-rendered platform. sakuramangas.org (#90) → **still unsupported; the wall is the reader's client-side image decryption, not Cloudflare** (two July 2026 re-investigations refined this — the first wrongly blamed CF, the second wrongly called the reader "frozen"). Its CF *is* beatable: a real Chrome the user launches themselves with `--remote-debugging-port` (no debugger attached at challenge time) clears the managed challenge, then chromedp attaches over CDP (`chromedp.NewRemoteAllocator`, `ws://127.0.0.1:9222`) and reuses the cached `cf_clearance` — it's our own chromedp-*launched* Chrome that CF detects and loops forever, not headed mode per se. With that, chapter listing works (`.chapter-item[data-url]`, link `a.a-scan`, title `.title-text`, number parsed from the `/obras/{slug}/{num}` URL) and the first ~6 pages download as clean plain JPEGs (`/imagens/{hash}/NNN.jpg`, direct-fetchable via the harvested session). The reader itself is **not** frozen: the real page counter is `.sp-page-indicator` (the visible `0/500` is an unrelated decoy widget), all N `.pg-item` divs exist, and CDP-injected *trusted* `Input.dispatchKeyEvent` ArrowRight/Left (isTrusted:true — JS `dispatchEvent`/`.click()` are isTrusted:false and ignored) advance it cleanly 1→14. The wall is that each page's image is decrypted client-side (`window.YggdrasilDecipher.decipher`, their "AetherCipher") into a `blob:` `<img>` **only on genuine, dwelt human interaction** — after a fresh navigate the active page never decodes on its own (40s+ dwell → no `<img>`), and every automated arming recipe (trusted clicks, arrow wiggles, `Page.bringToFront` + long per-page dwell, `_activeImgRefs` inspection) produced **zero decoded images** across pages 2–14. Pages past the first batch also 403 on their direct URL, so they exist *only* as those on-interaction blobs; the blobs are horizontally-mirrored (`scaleX(-1)`, reversible via `sips -f horizontal`) but that's moot when they never decode under automation. Getting a full, ordered chapter would require reverse-engineering AetherCipher directly (rotating obfuscated `security.oby.js` keys, 29×SHA-256 PoW headers, CSRF/challenge tokens) — the exact stack that got the dedicated Tachiyomi/keiyoushi extension permanently delisted — or a fragile manual babysitting flow. Not worth it; left unsupported. The reusable takeaways: **CF clearance via attach-to-a-user-launched-Chrome** (`chromedp.NewRemoteAllocator` + `page.BringToFront()`, since attached tabs are backgrounded) is a real technique for "works in my Chrome, not in automated Chrome"; and CDP `Input.dispatch*Event` gives isTrusted:true events that drive reader gestures a page's JS refuses from synthetic events — sakura just gates decoding on human-like interaction our automation couldn't fake.
- **An open API doesn't mean there's anything to download.** comick.dev/api.comick.dev sits behind Cloudflare, but a visible browser clears the challenge and the harvested cookies do carry over to the `api.` subdomain (no code changes needed — `http/session.go`'s cookie matching is already domain-suffix based), giving plain-HTTP access to comic/chapter-list JSON. However the actual page-image endpoint (`/chapter/{hid}/get_images`) returns an empty array `[]` anonymously for every chapter tried, from an obscure webtoon to a hugely popular, officially-scanlated one (Solo Leveling) — and the live rendered reader page itself never receives any page images either, matching manual browsing. Not supportable without an account; left unimplemented.
- **A CF challenge that keeps timing out on a rich selector may still pass in seconds — the selector is just too specific too early.** drakecomic.org (Drake Scans, a `mangareader`-themed WordPress/themesia site, same family as sushiscan.net: `#chapterlist li`/`.chapternum`/`ts_reader.run(...)`) repeatedly failed `browser.GetHTML` waiting on `li.wp-manga-chapter`/`#chapterlist li` (up to 4 minutes, visible browser) even though `PROBE_NETLOG` showed the real page answering 200 within seconds of the challenge — the page's `<body>` genuinely renders empty at first and only gets populated a few seconds later by an `admin-ajax.php` call. `chromedp.WaitVisible` already polls until the selector appears (no extra settle needed), so this wasn't actually a dead end; waiting on `"body"` with a probe `PROBE_SLEEP` first, then diffing the dump for the real selectors, unblocked it. Its images (`wp-content/uploads/.../*.webp`) are also CF-protected (403 to plain curl) but download fine via the harvested browser session, same as any other Path C site.
- **A blob in the DOM that isn't comma-separated URLs isn't necessarily unbreakable.** leercapitulo.co's reader hides page images in an obfuscated `#array_data` string that plain HTTP/regex can't decode (unlike mangabats' plain `var chapImages = '...,...'`). But the site's own javascript decodes it fine — the problem was it only showed one page at a time by default. Rendering the page in a real browser (`browser.GetHTML`) revealed a `localStorage` flag (`display_mode`) that switches the reader to "load all pages at once"; setting it and reloading before scraping got every page's real URL with zero cipher reverse-engineering. `browser.GetHTMLWithLocalStorage` generalizes this (set key/value, reload, then wait/extract) for any site gating content behind a client-side preference. Before assuming a reader's images are "encrypted and unsupportable" like colamanga, check whether a browser alone (no interaction) already resolves them — colamanga's are genuinely client-side-encrypted even after rendering, this one wasn't.

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
