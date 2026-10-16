---
name: pre-release
description: Use when preparing a release and the README and website images need retaking - the desktop-app screenshots (assets/screenshots and the website WebPs) and the terminal demo GIFs (assets/demos), plus the brand rasters when the brand changed. macOS only; check the OS before doing anything.
---

Read this before a release to refresh the images that show the UI. It covers only the assets; the release itself (the version in the git tag, the published GitHub Release that triggers `release.yaml`, the Homebrew jobs) is in `AGENTS.md`.

## Step 0: macOS only

Run `uname -s` first. If it isn't `Darwin`, **stop and tell the maintainer this must run on the maintainer's Mac; don't capture, record or commit anything.** The screenshots are the real Wails window grabbed with `screencapture`, and the GIFs and crops depend on the Mac's fonts, display scale and tools; images made on Linux or WSL look wrong (other fonts, no macOS window frame, wrong size).

## What gets regenerated

Do it from a clean worktree off `origin/master`, at the commit the release will be cut from, so the images show what ships.

| Asset | Source | Used in | Refresh |
|---|---|---|---|
| App screenshots, `assets/screenshots/{02..11}-*-{light,dark}.png` | `tools/screenshots/` | README | every release that changed the app's UI |
| Website WebPs, `website/src/assets/screenshots/*.webp` | the PNGs above, cropped by `website/build/crop-screenshots.sh` | website | with the PNGs (`capture-all.sh --install` does it) |
| Terminal GIFs, `assets/demos/*.gif` (dark) and `*-light.gif` | `assets/demos/*.tape` and `*-light.tape` twins | README (`download`, `prompt`, `groups`, `bundle`, `volumes`), website (`download`, `bundle`, `volumes`, via raw GitHub URLs on `master`) | every release |
| Brand rasters | `assets/brand/**.svg` | see "Brand" below | only when the brand changed |

## 1. App screenshots

Prerequisites: Xcode command line tools (`swiftc`), `cliclick`, `pngquant`, ImageMagick (`magick`), python3, Node + pnpm and Wails' build needs (the app is built with `make build/gui`), and Accessibility and Screen Recording permissions for the terminal. The window must be frontmost and nothing may cover it while it runs; leave the Mac alone until it finishes. **The screen must be unlocked**: on a locked screen the app can't come to the front and the script stops with `couldn't bring pid … to the front`. That's why the shots go first, while the maintainer is at the Mac; the GIFs record headless and can run unattended afterwards.

```bash
tools/screenshots/capture-all.sh --install
```

It builds the app with `capture.patch` applied (and reverts it, whatever happens) and `VITE_MOCK=1`, so the mock scenarios in `gui/frontend/src/mock.ts` are used and no site is involved. It then captures the 11 shots in light and dark (the window's own appearance, not the system's), crops them to 2000x1390, quantizes with `pngquant`, and, with `--install`, copies them into `assets/screenshots/` and rebuilds the website's WebPs (`website/build/crop-screenshots.sh` for the crops, `magick` for the `app-overview` hero). Without `--install` the PNGs stay in `tools/screenshots/out/` for a look first. Details are in `tools/screenshots/README.md` and `website/README.md` ("Screenshots").

- **Update the mock's version constants first**: the shots show fixed versions, `RELEASE_VERSION_SHOT` and `UPDATE_VERSION_SHOT` in `gui/frontend/src/mock.ts` (and the tests in `mock.test.ts` that pin them). Bump both: the first to the version being released, the second kept above it (the `09-update` shot is the "newer release" one, and `11-update-prompt` shows both in the one-click dialog).
- If a shot's layout changed (new control, different window content), check that the crop geometries still hold: the table in `website/README.md` and `website/build/crop-screenshots.sh`. Changing the crops means checking the `<img>` `width`/`height` in `website/index.html`, which come from the output.
- New or removed screenshots also mean `capture-all.sh`, the README and `website/index.html`.

## 2. Terminal GIFs

Prerequisites: [`vhs`](https://github.com/charmbracelet/vhs) (brings `ttyd` and `ffmpeg`), Go.

1. `make build/demo` writes `./manga-downloader` at the repo root, built with the `demo` tag. It serves `demo/series/*.json` from fake data (pages from `demo.invalid`, delays scaled by `MANGA_DOWNLOADER_DEMO_DELAY`, which each tape sets in its hidden setup line) and never touches the network. See `demo/README.md`. `make test/demo` tests it.
2. From the repo root, record every tape, dark and light:
   ```bash
   for t in assets/demos/*.tape; do vhs "$t"; done
   ```
   Each tape writes its own GIF (`Output assets/demos/<name>.gif`) and looks for the binary through `PATH="$PWD:$PATH"`, so run it from the repo root. One tape: `vhs assets/demos/prompt.tape`.
3. **`make build/unix` afterwards**: the demo binary can't download anything, so don't leave it in the checkout. `manga-downloader` is ignored by git, but a later real run would fail.
4. If a demo's range, chapters or series changed, edit `demo/series/*.json` and the tape first, and check the archive with `go run ./tools/verify-cbz` (see `demo/README.md`).

Each tape has a light twin (`OneHalfLight` instead of `OneHalfDark`, otherwise the same commands, output name `-light`). If you change one, change the other; the README and website pick the pair with `prefers-color-scheme`.

## 3. Brand (only when it changed)

The brand rasters are not part of a normal release refresh; redo them only when `assets/brand/` changed, per `assets/brand/brand.md` and the `gui` skill:

- `assets/brand/social/opengraph.png` (from `opengraph.svg`, 1280x640), copied to `website/public/opengraph.png`.
- The favicon and PWA PNGs in `website/public/` (from `assets/brand/favicon/`) and the website's copies of the SVGs in `website/src/assets/brand/`.
- The app icons in `gui/build/` (`appicon.png`, `windows/icon.ico`, `linux/icon.png`) from `assets/brand/app/appicon.svg`.
- The README header SVGs are used directly, nothing to render.

Keep the copies identical to their sources (`cmp` them), and don't redraw anything.

## 4. Check before committing

- Open a few frames or files and look: a GIF of each kind (start, middle, end), and the screenshots, a light and dark pair of each at least. No stray cursor, no half-drawn progress bar, no leftover terminal text, no grey (inactive) window, no wrong version, no real paths of the Mac in the shots (the `&shot` flag uses fixed ones).
- Light and dark pairs must show the same content and framing.
- File sizes are in the usual range: GIFs 50-250 KB, PNGs ~190-230 KB, WebPs 13-100 KB. A big jump means a wrong size or a failed `pngquant`.
- `git status` lists only images (and the mock constant), and `capture.patch` is not applied (`git diff` shows no `gui/main.go` changes).
- Website: `cd website && pnpm install && pnpm build` (and `pnpm test`) still pass.

## 5. Commit

One commit, conventional, like the earlier ones (`docs: retake the screenshots and re-record the demo gifs (#275)`):

```
docs: retake the screenshots and re-record the demo gifs
```

Add the `mock.ts`/`mock.test.ts`/`website` changes in it when they were needed. No `Co-authored-by`. Then the release goes on as `AGENTS.md` describes ("Releases").
