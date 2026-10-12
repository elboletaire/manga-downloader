# AGENTS.md

Guidance for coding agents working in this repository.

## Project

Go CLI (cobra) that downloads manga chapters from supported websites and packs them into CBZ files, plus a Wails desktop app in `gui/` (see the `gui` skill). Module: `github.com/elboletaire/manga-downloader`, targeting Go 1.26 (the `go` directive in `go.mod` and `gui/go.mod`, pinned by CI's test and smoke workflows).

## On-demand guides

These guides are not loaded with every session. Open the file when the task matches; agents that load skills by name can use the skill instead.

- **Sites and grabbers** (adding, fixing or investigating a site (new or supported), smoke-test failures, Cloudflare/browser problems): the `grabbers` skill, `.agents/skills/grabbers/SKILL.md`.
- **Desktop app** (`gui/`: Go backend, React frontend, bindings, packaging, theme, options): the `gui` skill, `.agents/skills/gui/SKILL.md`.

## Commands

```bash
make install          # go mod download
make build            # clean + test + build unix binary (injects version via ldflags)
make build/all        # unix + windows binaries
GOOS=windows GOARCH=386 make build/release   # one release binary, built as release.yaml does
make test             # go test -v ./... (uses richgo if installed)
go test ./ranges -run TestParse   # run a single test
make clean            # remove built binaries and *.cbz files
```

Unit test coverage is thin; real verification is the makefile smoke targets, which download from live sites — run them selectively. `make grabber` runs everything, `make grabber/html` all `PlainHTML` sites, `make grabber/<site>` just one; check the archives with `tools/verify-cbz`, since a run can succeed with empty or junk CBZs.

The makefile is tracked lowercase; on macOS `git add` needs the lowercase name. macOS also has no `timeout`.

Releases: the version lives only in the git tag (the makefile injects the short sha, the release workflow also injects `git describe`). There's no CHANGELOG and no hardcoded version. **`release.yaml` triggers on a *published GitHub Release*, not on a tag push** — pushing a tag alone releases nothing.

- **Website**: `website/` is the GitHub Pages site (Vite + TS, pnpm, deployed by `pages.yaml`). Its supported-sites list is parsed from this README's "Supported sites" section at build time, so keep that list's `- [domain (note)](url) \*` format and the "Show all N" count in sync. It's published in English, Spanish and Catalan: page text lives in `website/locales/*.json` (i18next format, rendered at build time), so add or change a string in all three files. See `website/README.md`.
- **Homebrew**: `packaging/homebrew/tap/` holds the formula (CLI) and cask (desktop app) for `elboletaire/homebrew-tap`. Each published release runs a `homebrew` job in `release.yaml` and `release-gui.yaml`, which copies them into the tap and sets the version and sha256s with `scripts/update-homebrew.sh`. Both jobs share one concurrency group. Keep the asset names in those files in step with the release workflows. See `packaging/homebrew/README.md`.

## Architecture

The download flow lives in `core` (`core/download.go`, `Series.Download`), UI-agnostic so the CLI and the GUI share it. `cmd/root.go` (`Run`) is the CLI shell around it: it parses flags and renders the `core.Events` through `cmd/progress.go`.

The steps:

1. `grabber.NewSite(url, settings)` → `IdentifySite()` calls `Test()` on each registered grabber (`grabber/site.go`), matching by domain or by fetching the URL. **Order matters**: domain-matching grabbers first, then `PlainHTML`, then the remaining fetch-testing ones. After all grabbers decline, `PlainHTML` gets a final chance via `resolveSeriesURL()`.
2. The matched `Site` fetches title and chapters; the range argument (`ranges.Parse`, `1-10,12,15-20`) filters them.
3. Each fetched chapter is passed through `Chapter.SkipPages` (`--skip-pages`, see below) before anything is downloaded, inside `core.Download`.
4. Chapters download concurrently (`downloader.FetchChapter`), bounded by `--concurrency` (max 5) and `--concurrency-pages` (max 10). Pages are plain GETs with a Referer.
5. `packer` writes CBZ files (`PackSingle`, or `PackBundle` with `--bundle`), named via a text/template (`--filename-template`, `packer/filename.go`); duplicate names get a `v{{.Version}}` suffix. Both paths go through `namePages` (`packer/pack.go`), which sniffs each page's format to name it `000.jpg`, `001.png`, … and transcodes per `--convert-images`.

**Chapter URL resolution**: both series and chapter links are accepted; see the grabbers skill for the full pattern.

**Design every feature for both the CLI and the desktop app.** They're meant to stay (almost) 1:1: a new option or behaviour lands in `core` where possible, then gets a flag in `cmd/` *and* a control in `gui/` (see the `gui` skill), with the same name, default and semantics, in the same change. The deliberate exceptions are few — `--concurrency`/`--concurrency-pages`/`--retry` aren't exposed in the app, and the app selects chapters from a list (with a range box that takes the same `1-10,12` syntax) instead of a positional range argument — so don't treat "the CLI already has it" as done, and say so explicitly when a feature is intentionally one-sided.

### Page conversion (`--convert-images`, #155)

Some sites serve pages e-readers can't render — AVIF (atsu.moe, mistscans, mangalib) and WebP (unreliable on e-ink). `packer/convert.go` re-encodes to JPEG q90: AVIF by default, WebP opt-in (`avif,webp`), `none` disables both.

- **The decoder is `github.com/gen2brain/avif` with `-tags nodynamic`** — libavif/dav1d as WASM under wazero, so `CGO_ENABLED=0` and the 8-way cross-compile matrix keep working (any cgo decoder is disqualified by that). The tag forces the embedded WASM instead of `dlopen`ing a system libavif. **It's set in one place, `BUILD_TAGS` in the `makefile`**: every build goes through it (`release.yaml` runs `make build/release`, `release-gui.yaml` and `gui-test.yaml` read the tags and ldflags with `make -s print-VAR`, the Dockerfiles run `make build/unix`). Don't pass build flags from a workflow directly: without the tag the library silently tries a system libavif first, so a mismatch is easy to miss.
- **The seam is `namePages`, not `grabber.Page.Transform`**: a `Transform` error re-runs the download retry loop, so a decode failure would pointlessly refetch the page. `namePages` is also where extensions are assigned, so converting there makes the `.jpg` follow automatically.
- **`flattenAlpha` composites onto white first**, because `jpeg.Encode` drops alpha rather than compositing — transparent margins would otherwise come out as solid black bars. Already-opaque images pass through untouched to keep `jpeg.Encode`'s fast path.
- **A page that fails to convert keeps its original bytes *and* extension**, with a warning: `pack()` has no partial-success path, so erroring would lose the whole chapter (or bundle) over one page. The extension must never claim `.jpg` for non-JPEG bytes.
- **`tools/verify-cbz` is what actually proves conversion happened in CI**, precisely because that failure is non-fatal: it rejects `.avif` entries (`-allow-avif` for `--convert-images=none` archives) and checks entry names against sniffed content.
- The WebP test fixture is a real file inlined as bytes (`x/image/webp` is decode-only, and Go has no test-only dependency scope), which also pins the load-bearing blank import in `convert.go`.

### Page skipping (`--skip-pages`)

Scanlation groups reserve pages for credits/ads (tcbscans: always page 2, often the last one too), and a page that 404s on the site's end fails the *whole* chapter with no way around it — `https://tcbonepiecechapters.com/chapters/7855/one-piece-chapter-1146` was flatly undownloadable until this flag existed. `Chapter.SkipPages` (`grabber/chapter.go`) drops the selected pages, called from `core.Download` right after `FetchChapter`.

- **Positions are 1-based indices into `chapter.Pages`, never `Page.Number`.** That field is not a position: most grabbers set `i+1`, a couple (`mgeko`, `projectsuki`) set a bare `i`, and `tcb` sets the *reader page* number, which several images share. `SkipPages` deliberately leaves `Page.Number` alone afterwards, so a download error still names the page the site numbers; the archive stays contiguous anyway because `namePages` numbers by slice index.
- **Skipping happens before the download, not at packing time.** That's the entire point (a skipped page costs nothing and can't 404 the chapter), and `downloader.FetchChapter` aborts the chapter on the first page error, so a post-download filter could never rescue a broken page.
- **`core.Download` calls `FetchChapter` twice in `--bundle` mode** — once in the totals loop that sizes the progress bar, once in the download goroutine. Any per-chapter filtering has to be applied at *both*, or the bar never reaches its total and `p.Wait()` hangs until the `Abort` fallback. `SkipPages` also updates `PagesCount` for the same reason.
- **`ranges.Parse` rejects negatives; `ranges.ParseRelative` accepts them.** Two entry points rather than one, because counting from the end is meaningless for chapter *numbers* (and chapter 0 exists), so the chapter-range caller should fail loudly on a negative instead of filtering nothing. `ranges.Resolve(rngs, total)` is what turns `-1` into a position, per chapter.
- The parser is regex-based now: it used to `strings.Split(part, "-")`, which can't tell `-3--1` apart from anything. Reversed ranges still clamp to their first bound (`10-5` → just 10) as they always have, but only when both bounds share a sign — `2--1` ("page 2 to the last") is ordered, just not comparably so until `Resolve` knows the total.
- **pflag takes the argument after a long flag verbatim, dash and all**, so `--skip-pages -1` works and the `=` form isn't actually required (`parseLongArg` never inspects the value for a leading `-`). A *shorthand* would be a different story, which is why this flag has none. The README documents both forms.
