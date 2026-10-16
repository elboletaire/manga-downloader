# Desktop-app screenshots

What took the screenshots in `assets/screenshots/` (and, from them, the
website's WebPs) for #248.

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
- **Export**: `pngquant` to 8-bit palette PNGs (~190–230 KB). The website's
  WebPs come from those with `website/build/crop-screenshots.sh` and the
  ImageMagick line for the hero in `website/README.md`.

## The patch

`capture.patch` holds throwaway changes, applied only to build the capture
binary:

- `gui/main.go`: `SHOT_W`/`SHOT_H` (window size), `SHOT_THEME` (appearance),
  `SHOT_Q` (the mock query, set on the first load) and `SHOT_JS` (run 1.2s
  after the mock page loads, e.g. to scroll the Options panel).

The mock is built in with `VITE_MOCK=1` (production builds leave it out), and
every query adds `&shot`: the release version, a macOS folder path and no debug
line in `update`. `&fail` is only used for `10-failed`, and `&prompt` (with the
`update` scenario) for `11-update-prompt`: it makes the update one-click and
opens its dialog. When the app's version moves on, update the `&shot` constants
near `RELEASE_VERSION` in `mock.ts`.

## Files

- `shot.sh OUT light|dark QUERY [WAIT] [JS]`: one capture.
- `shot-select.sh OUT light|dark`: 03, the Language `<select>` open on Catalan.
  A native popup can't be opened from JS, so it clicks it with `cliclick`;
  WebKit draws the popup inside the window, and `screencapture -l` leaves the
  cursor out.
- `winid.swift`, `winall.swift`: window id and bounds for a pid.
- `lib.sh`: `focus` (waits until the app really is frontmost; an inactive
  window comes out greyed, with a smaller shadow) and `stop` (Esc first, as an
  open native popup keeps the app from quitting, then SIGKILL if needed).
- `crop.py`: 2054x1444 → 2000x1390.
- `capture-all.sh [--install]`: all of the above for every shot and both
  themes, then the export; `--install` copies the PNGs into
  `assets/screenshots/` and regenerates the website's WebPs.

The individual steps are the ones used for #248; `capture-all.sh` puts them
together, and retook every shot end to end for #245.

Needs macOS, Xcode command line tools (`swiftc`), `cliclick`, `pngquant`,
ImageMagick and python3, and Accessibility and Screen Recording permissions
for the terminal.
