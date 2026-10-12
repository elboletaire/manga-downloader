# Reader patterns worth reusing

Most "the images aren't in the HTML" walls are plain data in disguise. Already implemented, and each generalizes to other sites:

- **`ts_reader.run(...)` blob** — every mangastream/themesia site, handled generically.
- **FoOlSlide `var pages = [...]`** — only the current page renders, but all URLs sit in that array (deathtollscans, furyosociety).
- **`${uid}` template literal** — `<img>` has a placeholder `src` plus a `uid`; the CDN prefix is regexed out of the page's own script rather than hardcoded, so it survives a CDN change (writerscans, mistscans, asmotoon).
- **Named JS array with an unstable variable name** — find the name from the assignment call, then pull the literal by name (mangakatana).
- **Alpine `x-data="immersiveReader({pages, baseLink})"`** — goquery decodes the escaped JSON via `.Attr()` (ritharscans). The live form is now `pages: JSON.parse('[...]')` with absolute CDN paths and a JS-escaped `baseLink`, decoded by `unescapeJSString` (`plainhtml.go`); the older literal relative-path form is still handled.
- **P.A.C.K.E.R.-packed JS** — a documented base-N token substitution, not a cipher; unpacks in pure Go with no JS engine (`unpackPackerJS` in `fanfox.go`). Don't reach for a browser just because a response starts with `eval(function(p,a,c,k,e,d)`.
- **TanStack Start / seroval page data** (hivetoons, vortexscans, azorafly) — the series page renders only the newest ~20 chapters, but the full list sits in an `initialChapters:$R[n]=[...]` literal followed by `,totalChapterCount:N`. `tanstackcomic.go` extracts it with `extractBalancedJSON` and `serovalToJSON` (unquoted keys, `!0`/`!1`, `void 0`, `$R[n]=` definitions); guard the list against the total so a shape change fails loudly.
- **Next.js RSC payloads** — `self.__next_f.push` chunks must be concatenated before searching (`nextFlightStream`), then `extractBalancedJSON` pulls a value by key marker, brace/bracket-matching through quoted strings, for objects and arrays alike. Sibling keys often can't share one struct, since the wrapper is a react element tuple — extract each separately (teamshadowi, witchtoons, templetoons).
- **Virtualized/infinite-scroll readers** — `GetHTMLWithScroll` steps a `Scroll.Container` (the document if empty) a viewport at a time until its `Done` JS holds or the bottom stops growing, so the page count needn't be known up front (comix.to: `.rpage-main`, done when no loader is left).
- **A CF 403 on one route may only be a direct-navigation rule** — mkissa's reader 403s on any direct navigation (cookie warm-up doesn't help) but loads fine when reached by clicking through the app's own routing, which issues no navigation request. `GetReaderHTML` generalizes this plus plateau-scrolling.
