# manga downloader website

The project site, published to GitHub Pages at
<https://elboletaire.github.io/manga-downloader/> by
[`.github/workflows/pages.yaml`](../.github/workflows/pages.yaml) on every push
to `master` that touches `website/` or `README.md`.

A single static page: Vite and TypeScript, no UI framework. Everything is in
the built HTML, so it works and indexes without JavaScript; the script only
picks the right download for the visitor's OS, filters the sites list and adds
copy buttons.

## Develop

```bash
pnpm install
pnpm dev       # http://localhost:5173/manga-downloader/
pnpm test      # vitest
pnpm build     # typecheck + build to dist/
pnpm preview   # serve dist/
```

## Where things come from

- **Supported sites**: parsed at build time from the `## Supported sites`
  section of the repo's [`README.md`](../README.md) (`build/sites.ts`, wired in
  by the `site-data` plugin in `vite.config.ts`), so the list never drifts.
  Items marked `\*` get the "browser" tag. Its test checks the parsed count
  against the README's "Show all N supported sites" line.
- **Download button**: `src/release.ts` (pure, tested) detects the OS and
  matches it against the assets of the latest GitHub release. If the API fails
  or the release has no matching desktop app file, it links to the releases
  page. Without a detectable OS it offers the desktop app downloads section.
- **Brand assets**: copied from the approved brand kit into `src/assets/brand/`
  and `public/` (favicons, `opengraph.png`). Don't redraw them.
- **Terminal GIFs**: referenced from `demos/` on `master` via raw GitHub URLs,
  lazy-loaded.

## Screenshots

The desktop app section shows four captures from `src/assets/screenshots/`,
each in a light and a dark version. A `<picture>` swaps the dark ones in with
`prefers-color-scheme`, and Vite fingerprints them into `dist`. WebP only, which
every browser the site cares about reads, so there are no PNG fallbacks.

- `app-overview` is the full window (`02-loaded-{light,dark}.png`), at 1000 and
  1600 px wide: the hero.
- `language-selector`, `chapter-list` and `finished-run` are crops of the
  2000 px originals in the repo's [`assets/screenshots/`](../assets/screenshots/),
  at 600 and 1000 px wide.
  They are cut to the part that matters, so they stay readable at the
  ~500 px they are shown at. Each gets the window's rounded corners and a soft
  shadow on a transparent margin of 56 px.

The files are converted offline and committed, so the build needs no image tools.
The crop script is [`build/crop-screenshots.sh`](build/crop-screenshots.sh). The
geometry is `WxH+X+Y` on the 2000x1390 original, with the window's top-left at
(85,58), so `03-language-selector-*.png` crops to `900x1000+85+58`. The
`finished-run` crop starts at x=1362, between the chapter list and the Activity
panel, so no edge of the list shows in it:

| Crop                | Original                                | Geometry            |
| ------------------- | --------------------------------------- | ------------------- |
| `language-selector` | `03-language-selector-{light,dark}.png` | `900x1000+85+58`    |
| `chapter-list`      | `04-downloading-{light,dark}.png`       | `1265x1000+100+278` |
| `finished-run`      | `05-done-{light,dark}.png`              | `553x760+1362+248`  |

To regenerate from the originals, run it with the repo's originals folder and an
output folder, from `website/`:

```bash
build/crop-screenshots.sh ../assets/screenshots src/assets/screenshots
```

The shadow widens each derivative beyond the crop plus its 56 px margin, so the
`<img>` sizes in `index.html` come from the output, not from the geometry.

The hero (`app-overview`) is not cropped. Its WebP derivatives come from
`02-loaded-{light,dark}.png` with ImageMagick:

```bash
magick IN.png -filter Lanczos -resize 1600x -strip -define webp:method=6 -quality 85 OUT-1600.webp
```

Repeat with `1000x` for the smaller width.

Each crop is resized with ImageMagick to 1000 and 600 px wide, converted with
`-define webp:method=6 -quality 85`, and stripped of metadata. The `<img>`
`width` and `height` are the largest derivative's size, so the layout doesn't shift. Every
screenshot is `loading="lazy"`: none of them is above the fold at 1440×900, and
the hero text is the LCP element.

## Another domain

`BASE_PATH` (default `/manga-downloader/`) and `SITE_URL` (default
`https://elboletaire.github.io/manga-downloader/`) set the base path, the
canonical and social URLs, `robots.txt` and `sitemap.xml`. For a custom domain,
set `BASE_PATH=/` and `SITE_URL=https://example.com/` in the workflow.
