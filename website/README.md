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
  matches it against the assets of the latest GitHub release. If the API fails,
  it links to the releases page; if the release has no desktop app yet, it
  offers the command line build and says the app is coming.
- **Brand assets**: copied from the approved brand kit into `src/assets/brand/`
  and `public/` (favicons, `opengraph.png`). Don't redraw them.
- **Terminal GIFs**: referenced from `demos/` on `master` via raw GitHub URLs,
  lazy-loaded.

## Screenshots

The desktop app section has four placeholders, marked with
`TODO(screenshot)` comments and `data-todo="screenshot"` in `index.html`:
`app-overview` (16:10), `chapter-list`, `language-selector` and `finished-run`
(4:3, the Happy mascot moment). Drop the images in `src/assets/screenshots/`
and replace each `.shot-placeholder` div with the `<img>` in its comment,
keeping `width`, `height`, `loading="lazy"` and a real `alt`.

## Another domain

`BASE_PATH` (default `/manga-downloader/`) and `SITE_URL` (default
`https://elboletaire.github.io/manga-downloader/`) set the base path, the
canonical and social URLs, `robots.txt` and `sitemap.xml`. For a custom domain,
set `BASE_PATH=/` and `SITE_URL=https://example.com/` in the workflow.
