# CLAUDE.md

Guidance for Claude Code (claude.ai/code) when working in this repository.

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

Unit test coverage is thin; real verification is the makefile smoke targets, which download from live sites — run them selectively. `make grabber` runs everything, `make grabber/html` all `PlainHTML` sites, `make grabber/<site>` just one.

Releases: the version lives only in the git tag (the makefile injects the short sha, the release workflow also injects `git describe`). There's no CHANGELOG and no hardcoded version. **`release.yaml` triggers on a *published GitHub Release*, not on a tag push** — pushing a tag alone releases nothing.

## Architecture

The download flow, orchestrated from `cmd/root.go` (`Run`):

1. `grabber.NewSite(url, settings)` → `IdentifySite()` calls `Test()` on each registered grabber (`grabber/site.go`), matching by domain or by fetching the URL. **Order matters**: domain-matching grabbers first, then `PlainHTML`, then the remaining fetch-testing ones.
2. The matched `Site` fetches title and chapters; the range argument (`ranges.Parse`, `1-10,12,15-20`) filters them.
3. Chapters download concurrently (`downloader.FetchChapter`), bounded by `--concurrency` (max 5) and `--concurrency-pages` (max 10). Pages are plain GETs with a Referer.
4. `packer` writes CBZ files (`PackSingle`, or `PackBundle` with `--bundle`), named via a text/template (`--filename-template`, `packer/filename.go`); duplicate names get a `v{{.Version}}` suffix. Both paths go through `namePages` (`packer/pack.go`), which sniffs each page's format to name it `000.jpg`, `001.png`, … and transcodes per `--convert-images`.

### Page conversion (`--convert-images`, #155)

Some sites serve pages e-readers can't render — AVIF (atsu.moe, mistscans, mangalib) and WebP (unreliable on e-ink). `packer/convert.go` re-encodes to JPEG q90: AVIF by default, WebP opt-in (`avif,webp`), `none` disables both.

- **The decoder is `github.com/gen2brain/avif` with `-tags nodynamic`** — libavif/dav1d as WASM under wazero, so `CGO_ENABLED=0` and the 7-way cross-compile matrix keep working (any cgo decoder is disqualified by that). The tag forces the embedded WASM instead of `dlopen`ing a system libavif. **It must stay in sync in two places: `GOTAGS` in the `makefile` and `build_flags` in `.github/workflows/release.yaml`** — the release is the only build path that bypasses the makefile, and the library silently degrades to WASM without the tag, so a mismatch is easy to miss.
- **The seam is `namePages`, not `grabber.Page.Transform`**: a `Transform` error re-runs the download retry loop, so a decode failure would pointlessly refetch the page. `namePages` is also where extensions are assigned, so converting there makes the `.jpg` follow automatically.
- **`flattenAlpha` composites onto white first**, because `jpeg.Encode` drops alpha rather than compositing — transparent margins would otherwise come out as solid black bars. Already-opaque images pass through untouched to keep `jpeg.Encode`'s fast path.
- **A page that fails to convert keeps its original bytes *and* extension**, with a warning: `pack()` has no partial-success path, so erroring would lose the whole chapter (or bundle) over one page. The extension must never claim `.jpg` for non-JPEG bytes.
- **`tools/verify-cbz` is what actually proves conversion happened in CI**, precisely because that failure is non-fatal: it rejects `.avif` entries (`-allow-avif` for `--convert-images=none` archives) and checks entry names against sniffed content.
- The WebP test fixture is a real file inlined as bytes (`x/image/webp` is decode-only, and Go has no test-only dependency scope), which also pins the load-bearing blank import in `convert.go`.

### The grabber package

- `Site` (`grabber/site.go`) is the contract: `Test`, `FetchTitle`, `FetchChapters`, `FetchChapter`, … The base `Grabber` provides shared settings/helpers.
- `Filterable`/`Filterables` (`filterable.go`) abstracts chapters for sorting/filtering by number; each grabber has its own chapter struct embedding `grabber.Chapter`.
- **`PlainHTML`** (`plainhtml.go`): generic goquery scraper driven by a list of `SiteSelector` entries. Order in that list matters — a loose entry shadows later ones. `getPlainHTMLImageURL` handles readers that don't just expose `<img src>` (see patterns below).
- **`PlainHTMLBrowser`** (`plainhtmlbrowser.go`): same selector parsing, but matches **by domain** (`browserSelectors`, no fetch — starting a browser is expensive) and renders through Chrome. Registered first in `IdentifySite()`.
- **The two selector lists are the source of truth for which sites are covered** — they drift constantly, so read them rather than any list in this file.
- Dedicated grabbers exist where selectors can't work: `inmanga`, `mangadex` (API-based, language-aware), `mangabats`, `mangafire` (browser-driven, see below), `mangak`, `qimanga`, `tcb` (Madara), `leercapitulo`, `witchtoons`, `teamshadowi`, `baozimh`, `atsumaru`, `mkissa`, `fanfox`, and others.
- **`baozimh.go`** (包子漫画) is plain HTML but **geo-routed by visitor IP**: mainland-China IPs get simplified-script pages, everyone else traditional. Same markup either way, but the chapter-number regex must accept both `话`/`話` (a hardcoded one silently drops every chapter for half the world) and a bare leading number, while rejecting digit-led dates in code since RE2 has no lookahead. Long chapters span several reader pages linked by a "next page" link with overlapping images, merged and deduped by CDN URL. Repeated chapter *numbers* are often genuinely distinct files and all download.

### The browser package

`browser/browser.go` drives a locally-installed Chromium (auto-discovered by chromedp) for sites plain HTTP can't scrape. One process, started lazily and reused. `GetHTML` returns rendered HTML and harvests cookies + UA into `http/session.go`, so images still download over fast plain HTTP reusing the browser's Cloudflare clearance. Variants: `GetHTMLWithScroll`, `GetHTMLWithLocalStorage`, `GetReaderHTML`, `GetAPIResponses`.

- **Always try the site's own API/HTTP first.** mangafire's API was wide open until it started signing calls with a CF-gated `vrf` token (403 "Missing token"), forcing a browser; zonatmo went the other way and dropped its TLS block. Re-verify instead of assuming either direction is permanent.
- **Headless loses to Cloudflare; headed usually wins.** `GetHTML` handles this: a short headless probe, then a teardown and reopen as visible on `challengeError`. `--browser-visible` skips the wasted probe.
- **A theme tells you nothing about the hosting.** The same themesia or Madara markup is plain HTTP on one domain and CF-gated on another — check, don't assume.
- **`tools/probe`** renders a URL and tests selectors. Env knobs: `PROBE_VISIBLE=1`, `PROBE_TIMEOUT=4m` (default 45s), `PROBE_SLEEP=8s` (settle for lazy SPAs), `PROBE_DUMP=/tmp/x.html`, `PROBE_NETLOG=1` (log non-asset responses — how mangafire's API was found), `PROBE_FETCH_SEL` (test a plain-HTTP image download with the harvested session), `PROBE_API_URL` + `PROBE_API_DUMP_DIR` (fetch JSON APIs reusing the session; checks cookie carry-over to e.g. an `api.` subdomain).

### Reader patterns worth reusing

Most "the images aren't in the HTML" walls are plain data in disguise. Already implemented, and each generalizes to other sites:

- **`ts_reader.run(...)` blob** — every mangastream/themesia site, handled generically.
- **FoOlSlide `var pages = [...]`** — only the current page renders, but all URLs sit in that array (deathtollscans, furyosociety).
- **`${uid}` template literal** — `<img>` has a placeholder `src` plus a `uid`; the CDN prefix is regexed out of the page's own script rather than hardcoded, so it survives a CDN change (writerscans, mistscans, asmotoon).
- **Named JS array with an unstable variable name** — find the name from the assignment call, then pull the literal by name (mangakatana).
- **Alpine `x-data="immersiveReader({pages, baseLink})"`** — goquery decodes the escaped JSON via `.Attr()` (ritharscans).
- **P.A.C.K.E.R.-packed JS** — a documented base-N token substitution, not a cipher; unpacks in pure Go with no JS engine (`unpackPackerJS` in `fanfox.go`). Don't reach for a browser just because a response starts with `eval(function(p,a,c,k,e,d)`.
- **Next.js RSC payloads** — `self.__next_f.push` chunks must be concatenated before searching (`nextFlightStream`), then `extractBalancedJSON` pulls a value by key marker, brace/bracket-matching through quoted strings, for objects and arrays alike. Sibling keys often can't share one struct, since the wrapper is a react element tuple — extract each separately (teamshadowi, witchtoons, templetoons).
- **A client-side preference can be flipped instead of reversed** — leercapitulo's obfuscated blob decodes itself once a `localStorage` flag switches the reader to "load all pages" (`GetHTMLWithLocalStorage`). Check whether a browser alone resolves the images before declaring a reader encrypted.
- **Virtualized/infinite-scroll readers** — `GetHTMLWithScroll` steps through the *growing* `scrollHeight`, so the page count needn't be known up front (comix.to).
- **A CF 403 on one route may only be a direct-navigation rule** — mkissa's reader 403s on any direct navigation (cookie warm-up doesn't help) but loads fine when reached by clicking through the app's own routing, which issues no navigation request. `GetReaderHTML` generalizes this plus plateau-scrolling.

### Rules learned the hard way

- **Paywalls mostly look like success.** Recent chapters commonly render zero images with no error, or replace the link with a modal. Several `Rows` selectors end in `a[href]` just to filter those out. **Smoke-test a chapter slightly behind the latest**, never the newest.
- **Verify CBZ contents, not their existence.** A run can "succeed" with empty or junk archives; `tools/verify-cbz` checks sizes and magic bytes.
- **Chapter titles need `sanitizeTitle()` too**, not just series titles — several themes pad `.chapternum` with raw tabs/newlines or mix in a trailing date, which leaked into filenames.
- **Scope bare `h1` selectors** — sidebar labels and decorative stat headings are common (`h1[itemprop="name"]`).
- **Scope Madara reader selectors** — `div.reading-content img` picks up decorative banners/footers on some sites, and `body.Find("li")` matches "Volume N" wrappers as bogus chapters.
- **`tcb.go`'s `Test()` is domain-agnostic**, so any Madara site with the same `ajax/chapters` endpoint works with zero code — just a smoke target and a README line (mangasushi, mangalivre.to were both free this way).
- **A wait selector matching the first element isn't proof the rest loaded** — readers that append images progressively need `BrowserSiteSelector.Settle`. Verify by re-counting matches at increasing `PROBE_SLEEP` until the count stops growing.
- **A CF challenge timing out on a rich selector may just be too specific too early** — wait on `"body"`, dump, then find the real selectors.
- The makefile is tracked lowercase; on macOS `git add` needs the lowercase name. macOS also has no `timeout`.
- **Use a temp dir namespaced to your agent/worktree** when investigating — generic `/tmp/series.html` paths collide across parallel agents and get overwritten mid-investigation.

### Investigating a new site

Triage with curl first: fetch with a browser UA and check the `<title>`. "Just a moment…" means a CF JS challenge; TLS drops, empty SPA shells and encrypted-image readers are equally out for plain HTTP. But check the *content*, not just status/size — fanfox returns complete HTML yet swaps every series page for an age gate until an `isAdult=1` cookie is sent, and a 200 can be a parked or ransom page. 403/500 in the app but 200 in curl almost always means headers (`http/request.go` sends UA, Accept, Accept-Language and a trailing-slash referer, all because some site demanded them). Old chapters are often dropped from CDNs, so "chapter 1" is usually the worst test target. And a near-empty `grep '<img src="'` proves nothing — attributes may be single-quoted, and `grep -c` on minified HTML counts lines, not matches.

Then:

- Plain HTML → add a `SiteSelector` to `PlainHTML.Test()`, plus a `grabber/html` smoke target and a README line.
- Needs a browser but selector-friendly → add a `BrowserSiteSelector` to `browserSelectors`.
- Otherwise → new grabber implementing `Site` (embed `*Grabber`), registered in `IdentifySite()`, with its own smoke target in the top-level `grabber:` aggregate (not `grabber/html`).

### Sites that moved, or that we deliberately don't support

Sites rebrand constantly; probe for a successor before declaring one dead, and don't guess slug patterns — get a real series URL from the site's own listing.

- **Moved, still supported**: tcbscans → tcbonepiecechapters; mangabat → mangabats; asuratoon → asurascans; zonatmo.com → zonatmo.org (seized Apr 2026); aurorascans → qimanhwa → qimanga; mangalivre.tv/.net → mangalivre.to; mangabuddy → mangak.io; witchscans → witchtoons.net; mangapark.to/.io → mangapark.page; utoon.net (now a "site suspended" hijack page) → utoon.us.
- **mangapark.page** is now browser-gated (Aug 2026): Cloudflare challenges the *whole* domain, static assets included, so there is nothing left to curl. Don't go mirror-hunting — `.to` now lands on the parked `comicpark.org`, and the `.com`→`.net`→`.org`→`.me`→`.io` chain dead-ends; `.page` is the only live host. Only `fetchSeriesData` renders: the harvested clearance carries the `get-chapter-list` API *and* the ~200 images per chapter over plain HTTP, so don't reach for the browser in `FetchChapter`. It still tries plain HTTP first, so re-check whether the challenge is still on before assuming the browser is permanent. Two red herrings: `window.seriesData` gained a `slug_hash` ("rowdy-reunion.NTFztg") that the site's own links use, but the bare slug still works everywhere and the API 400s on the hash; and the wait selector must be something only the real page has (`a[href*="/series/"]`) — waiting on `body` makes the interstitial look like a successful render and the headless→visible escalation never fires.
- **witchtoons.net** deserves a note: the redirect from witchscans drops the path and the platform changed entirely, so old URLs can't be remapped. It's server-rendered Next.js, so no browser — but the chapter list is in the RSC payload, not the DOM (the series page renders exactly *one* chapter link, so a selector-driven grabber sees 1 of 61 and looks like it works), and is paginated at 100 per page. Page URLs are signed with a ~1h TTL, harmless since they download immediately.
- **colamanga** (#30): app-gated reader on top of client-side image encryption. Not feasible.
- **comick.dev**: CF is beatable and the chapter-list JSON is reachable, but the page-image endpoint returns `[]` anonymously for every chapter tried, and the live reader receives no images either. Needs an account.
- **sakuramangas.org** (#90): the wall is client-side image decryption, **not** Cloudflare — two earlier investigations got this wrong. Pages exist only as `blob:`s their cipher decodes on genuine, dwelt human interaction; every automated recipe produced zero decoded images, and direct URLs 403 past the first few. Beating it properly means reverse-engineering rotating obfuscated keys, PoW headers and CSRF tokens — the stack that got the dedicated Tachiyomi extension delisted. Not worth it. Two techniques from it *are* reusable, though:
  - **CF clearance by attaching to a user-launched Chrome**: a Chrome the user starts with `--remote-debugging-port` (no debugger attached at challenge time) clears the challenge; chromedp then attaches via `NewRemoteAllocator` and reuses the cached `cf_clearance`, plus `BringToFront()` since attached tabs are backgrounded. It's our chromedp-*launched* Chrome that CF detects, not headed mode — so this is the move for "works in my Chrome, not in automated Chrome".
  - **CDP `Input.dispatch*Event` produces `isTrusted:true` events**, which drive reader gestures that a page's JS ignores from synthetic `dispatchEvent`/`.click()` calls.
