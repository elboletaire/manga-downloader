---
name: grabbers
description: Use when adding support for a new manga/comic site, fixing or debugging a grabber, investigating why a site fails (including Cloudflare or browser problems), running or triaging the makefile smoke targets, using tools/probe, editing the README Supported sites list, or scouting new sites to support.
---

Read this for any site or grabber work: adding, fixing, debugging, investigating or smoke-testing one. The reference files next to it hold the detail, listed at the end.

## Workflow

1. Triage the site first, as described in "Investigating a new site" below.
2. Pick the grabber kind from the "Then:" list there: a `PlainHTML` `SiteSelector`, a `BrowserSiteSelector`, or a dedicated grabber.
3. Keep `Test()` markup-based, per the rule at the end of "Investigating a new site".
4. Add the smoke target, the README "Supported sites" line (and bump both counts: the features-list bullet and "Show all N") and, for plain-HTTP grabbers, the CI matrix entry in `.github/workflows/smoke.yaml`.
5. Verify with `tools/verify-cbz` on the downloaded archives, not just the smoke target's exit code.

### Investigating a new site

Triage with curl first: fetch with a browser UA and check the `<title>`. "Just a moment…" means a CF JS challenge; TLS drops, empty SPA shells and encrypted-image readers are equally out for plain HTTP. But check the *content*, not just status/size — fanfox returns complete HTML yet swaps every series page for an age gate until an `isAdult=1` cookie is sent, and a 200 can be a parked or ransom page. 403/500 in the app but 200 in curl almost always means headers (`http/request.go` sends UA, Accept, Accept-Language and a trailing-slash referer, all because some site demanded them). Old chapters are often dropped from CDNs, so "chapter 1" is usually the worst test target. And a near-empty `grep '<img src="'` proves nothing — attributes may be single-quoted, and `grep -c` on minified HTML counts lines, not matches.

Then:

- Plain HTML → add a `SiteSelector` to `PlainHTML.Test()`, plus a `grabber/html` smoke target and a README line.
- Needs a browser but selector-friendly → add a `BrowserSiteSelector` to `browserSelectors`.
- Otherwise → new grabber implementing `Site` (embed `*Grabber`), registered in `IdentifySite()`, with its own smoke target in the top-level `grabber:` aggregate (not `grabber/html`).

**Make `Test()` recognise the platform by its markup, not the site by its domain.** Sites get cloned and rebranded constantly, and a domain regex turns away every clone our code could already download: that's why six sites worked with zero code (themesia markup for `PlainHTML`, Madara's `ajax/chapters` for the domain-agnostic `tcb`), while each new TanStack or RSC-comic domain needed its own wrapper grabber (`azorafly.go` is ~20 lines that only say "this domain is TanStack"). For a new grabber that targets a platform rather than one service, fingerprint the fetched page with something specific (e.g. TanStack's `initialChapters:$R[` + `totalChapterCount`), place it among the fetch-testing grabbers, and keep a domain only for URL rewrites a site genuinely needs (kaynscan's `/series/{slug}` → `/series/comic/{slug}`). Domain matching is still right for browser-gated sites (a browser is too expensive to start just to identify a page) and for single-service API grabbers (mangadex, mangaplus, webtoons). The existing domain-matched platform grabbers (`tanstackComic`, `rscComic`, the manganelo and mangahere families) predate this rule and haven't been converted; do it, with a shared page fetch so the structural tests don't each GET the URL, as a deliberate refactor rather than in passing.

## The grabber package

- `Site` (`grabber/site.go`) is the contract: `Test`, `FetchTitle`, `FetchChapters`, `FetchChapter`, … The base `Grabber` provides shared settings/helpers.
- `Filterable`/`Filterables` (`filterable.go`) abstracts chapters for sorting/filtering by number; each grabber has its own chapter struct embedding `grabber.Chapter`.
- **Chapter URL resolution**: a pasted chapter link must load the whole series. Dedicated grabbers do it in a `resolveSeriesURL()` called from `FetchTitle()`/`FetchChapters()`: a no-op for series URLs, deriving the series from the URL before fetching (at most once), matching on the path so `www.`, case, query and fragment don't matter, returning `errSeriesNotFound` when it gives up, with a table test per rewrite. Mobile hosts are rewritten to the desktop one in `Test()` (mangatown, mangahere, fanfox). `PlainHTML` resolves reader pages as a last resort, after every grabber has declined in `IdentifySite()`: a selector's `SeriesLink` (the reader's back-to-series link, applied only when that selector's `Image` matches the page), then breadcrumbs and parent paths, at most 3 fetches; `FollowSeriesLink` follows it even when the reader shows a partial chapter dropdown.
- **`PlainHTML`** (`plainhtml.go`): generic goquery scraper driven by a list of `SiteSelector` entries. Order in that list matters — a loose entry shadows later ones. `getPlainHTMLImageURL` handles readers that don't just expose `<img src>` (see `patterns.md`).
- **`PlainHTML.Test()` declines** when the first matching entry's rows can't be numbered: `rowChapterNumber` (shared with `FetchChapters`, URL fallback included) decides, so a domain-agnostic fallback like `tcb` takes Madara sites a loose entry would otherwise claim with zero chapters.
- **`PlainHTMLBrowser`** (`plainhtmlbrowser.go`): same selector parsing, but matches **by domain** (`browserSelectors`, no fetch — starting a browser is expensive) and renders through Chrome. Registered first in `IdentifySite()`.
- **The two selector lists are the source of truth for which sites are covered** — they drift constantly, so read them rather than any list in these guides.
- Dedicated grabbers exist where selectors can't work: `inmanga`, `mangadex` (API-based, language-aware), `manganelo` (mangabats + mangakakalot + natomanga + manganato.gg: the DOM renders only the newest 50 chapters, the full list comes from a paginated JSON API), `mangafire` (browser-driven, see `browser.md`), `mangak`, `qimanga`, `tcb` (Madara), `leercapitulo`, `witchtoons`, `teamshadowi`, `baozimh`, `atsumaru`, `mkissa`, `fanfox`, `mangatown` (the mangahere family without chapterfun.ashx: one HTML document per reader page, count from `total_pages`; m. host and chapter URLs are rewritten to the www series URL), `webtoons` (episode list from the m.webtoons.com `/api/v1/{webtoon|canvas}/{titleNo}/episodes` cursor API with pageSize=1000, HTML list walk as fallback; canvas ids only answer on the canvas route; the image CDN 403s without a webtoons.com Referer; free episodes only, since locked ones aren't listed on the web), and others.

The baozimh and Manga Plus notes live in `sites.md`, under "Per-grabber notes".

## Reference files

- `patterns.md`: reader patterns for images that aren't plain `<img src>` (`ts_reader`, FoOlSlide, `${uid}` templates, TanStack/seroval, Next.js RSC, P.A.C.K.E.R., infinite-scroll readers). Read when a reader's images aren't in the HTML.
- `browser.md`: the `browser` package (chromedp, Cloudflare clearance, the challenge clicker, container flags, `tools/probe`). Read before writing a browser-driven grabber or when a site is Cloudflare-gated.
- `rules.md`: hard-won rules on paywalls, chapter lists, titles, selector scoping and CBZ verification. Read before writing selectors, when a chapter list looks short or stops at a round number, when a run "succeeds" with bad archives, or when a browser wait selector passes too early.
- `sites.md`: sites that moved, or that we deliberately don't support, plus per-grabber notes (`baozimh`, `mangaplus`). Read when a site stops working or before starting on a requested site.
