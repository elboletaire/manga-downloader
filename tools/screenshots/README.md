# Desktop-app screenshots

What took the screenshots in `assets/screenshots/` (WebP, for the README and `docs/`) and
the website's WebPs, for #248.

## How they were taken

- **The real app, not a browser**: the Wails app on macOS, built with
  `capture.patch` applied, loading the frontend's mock (`?mock&scenario=…&freeze`,
  see the header of `gui/frontend/src/mock.ts`), so no site is involved and
  each state is fixed.
- **Window**: 915x578 pt of content plus macOS's 32 pt title bar, so 915x610 pt.
  `screencapture -l <window id>` captures it at 2x (Retina) with the system
  shadow, 2054x1444 px; `crop.py` trims the shadow margin to 2000x1390.
- **Light and dark**: set on the window, not the system. The patch sets the
  window's `NSAppearance` (Aqua or DarkAqua) from `SHOT_THEME`, and WebKit
  follows it for `prefers-color-scheme`; the startup background is set to match.
- **The window frame is macOS's own**: the title bar, traffic lights, rounded
  corners and shadow are what `screencapture` captures. Nothing is drawn on.
  The window is brought to the front first, so it has the coloured traffic
  lights and the active window's shadow.
- **Export**: the 2000x1390 PNGs stay lossless in `out/` (not committed). The
  README's WebPs are made from them at `-quality 90 -define webp:method=6`
  (~100–165 KB each), and so are the website's, with
  `website/build/crop-screenshots.sh` and the ImageMagick line for the hero in
  `website/README.md`.

## The patch

`capture.patch` holds throwaway changes, applied only to build the capture
binary:

- `gui/main.go`: `SHOT_W`/`SHOT_H` (window size), `SHOT_THEME` (appearance),
  `SHOT_Q` (the mock query, set on the first load) and `SHOT_JS` (run 1.2s
  after the mock page loads, e.g. to scroll the Options panel).

The mock is built in with `VITE_MOCK=1` (production builds leave it out), and
every query adds `&shot`: the release version, a macOS folder path and no debug
line in `update`. `&fail` is only used for `11-failed`, and `&prompt` (with the
`update` scenario) for `12-update-prompt`: it makes the update one-click and
opens its dialog. `&exists` (with `running`) is only used for
`14-skip-existing`. When the app's version moves on, update the `&shot`
constants near `RELEASE_VERSION` in `mock.ts`.

## Files

- `shot.sh OUT light|dark QUERY [WAIT] [JS]`: one capture.
- `shot-select.sh OUT light|dark`: 03, the Language `<select>` open on the window's language (`SHOT_LANG`).
  A native popup can't be opened from JS, so it clicks it with `cliclick`;
  WebKit draws the popup inside the window, and `screencapture -l` leaves the
  cursor out.
- `winid.swift`, `winall.swift`: window id and bounds for a pid.
- `lib.sh`: `focus` (waits until the app really is frontmost; an inactive
  window comes out greyed, with a smaller shadow) and `stop` (Esc first, as an
  open native popup keeps the app from quitting, then SIGKILL if needed).
- `crop.py`: 2054x1444 → 2000x1390.
- `capture-all.sh [--install]`: all of the above for every shot and both
  themes, then the export; `--install` writes the English WebPs into
  `assets/screenshots/` and regenerates the website's WebPs, both from `out/`.
  `SHOT_LANG` picks the language (see below).
- `01-start`: the empty window on the simple view (`simpleStart` is on by default in the mock).
- `13-settings`: the Settings dialog, opened by clicking the header's gear
  (`.settings-btn`) on the loaded mock.
- `14-skip-existing`: a run in progress whose first two chapters are already in
  the folder (`&exists` with `running`), listed as skipped above the rows
  downloading.

## Languages

`SHOT_LANG` (`en`, `es`, `ca`, `ja` or `all`, default `en`) sets the window's
language, as `&lang=` on every shot query (see `mock.ts`). `all` runs the four
in turn after one build, which is what a release needs:

```bash
SHOT_LANG=all tools/screenshots/capture-all.sh --install
```

`--install` puts English's README WebPs in `assets/screenshots/` (the README is
English-only), and every language's crops and hero in the website's
`src/assets/screenshots/`: English at its root, `es`, `ca` and `ja` in a `<lang>/`
folder. Without `--install` the PNGs stay in `out/<lang>/`.

`SHOTS` retakes only the shots named, for a change that touched one or two of
them. The website's crops and hero are made from the shots taken, and the
others keep their crops on the site; the README's WebPs are only the shots taken too:

```bash
SHOTS="01-start 13-settings" tools/screenshots/capture-all.sh --install
```

A new shot reaches the website and the README this way, without retaking the rest:

```bash
SHOTS=14-skip-existing SHOT_LANG=all tools/screenshots/capture-all.sh --install
```

A translated label that changes a panel's size needs its crop checked in
`website/build/crop-screenshots.sh`: the crops assume the panels sit where they
do in English, which holds only while the window size is fixed. The Language
select that `shot-select.sh` clicks is positioned the same way, so check its
03 shot too.

The individual steps are the ones used for #248; `capture-all.sh` puts them
together, and retook every shot end to end for #245.

Needs macOS, Xcode command line tools (`swiftc`), `cliclick`, ImageMagick and
python3, and Accessibility and Screen Recording permissions for the terminal.
