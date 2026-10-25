# manga downloader: brand

Sources of the brand as the repo uses them. The GUI's mascot (`gui/frontend/src/Mascot.tsx`) and the README header read these files directly; the GUI's colour tokens in `gui/frontend/src/styles.css` follow the mapping below.

Mascot **Neko Book**: an open manga volume that is also a cat's face. One body, two expressions.

## Files

| path | what |
|---|---|
| `mark/soft/`, `mark/happy/` | `mark` (light), `mark-dark`, `mark-small` / `mark-small-dark` (16–32px), `mark-mono` / `mark-mono-small` (`currentColor`) |
| `lockup/` | `lockup` (stacked, primary), `lockup-line` (one line, tight spaces); each `-dark` and `-mono` |
| `wordmark/` | `wordmark-line` and `-dark`: the lockup's lettering alone, cropped to the text (the GUI header uses it beside the live mascot) |
| `app/appicon.svg` | source of the app icons, which live in `gui/build/`: `appicon.png` (1024², macOS tile; Wails builds the .icns from it), `windows/icon.ico` (16/24/32 small artwork, BMP, + 48/64 BMP + 256 PNG), `linux/icon.png` (free shape, transparent; window icon and .desktop file) |
| `favicon/` | `favicon.svg` (switches colour with `prefers-color-scheme`) → the GUI's page icon; `icon-pwa.svg`, `apple-touch-icon.svg` (sources for the PNGs) |
| `social/opengraph.{svg,png}` | 1280×640 social preview (English). The es, ca and ja cards are `website/public/opengraph-{es,ca,ja}.png`, rendered from the master by `website/build/opengraph.mjs` (see the website README) |
| `readme/readme-header-{light,dark}.svg` | 1600×400 README header |

The `.svg` masters are kept for every raster that's shipped. Other sizes (the app's Linux icon set, the favicon PNGs, the free-shape 1024 app icon, the brand sheet) are regenerated from the SVGs when needed and aren't committed.

All SVGs are self-contained: the text is converted to paths, and they use no fonts or raster images.

README usage (this repo's README uses these paths):

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/brand/readme/readme-header-dark.svg">
  <img alt="manga downloader" src="assets/brand/readme/readme-header-light.svg">
</picture>
```

## Expressions

- **Soft** (default): the logo, app icon, favicon, idle/working states. Calm open eyes, ω mouth.
- **Happy**: success and celebration only. A run where every chapter downloaded, a "done" toast, friendly empty states. Closed ^ ^ eyes and an open smile. The body, ears, book and blush are identical to Soft, so swapping one for the other reads as the same character changing face. Below 48px Happy uses Soft's face (`mark-small` is identical for both), so don't rely on the expression in tray or taskbar icons.
- Never show a sad or error face. Errors are handled by the UI colours and copy, not the mascot.

## Palette

| name | hex | role | on white | on Night |
|---|---|---|---|---|
| Violet | `#7C5CE0` | Primary brand colour: mark body, buttons, links on light. | 4.7:1 | 3.8:1 |
| Violet deep | `#5B3FC4` | "downloader" in the wordmark, accent text on light. | 7.1:1 | 2.5:1 |
| Violet soft | `#A993FF` | "downloader" and accent on dark. | 2.5:1 | 7.2:1 |
| Violet (dark mark) | `#8B6EF0` | Mark body on dark surfaces. | 3.8:1 | 4.8:1 |
| Mint | `#4FD8B0` | Success/done, the download cue. Graphics only on light. | 1.8:1 | 10.2:1 |
| Mint deep | `#1B7A5E` | Success text on light. | 5.3:1 | 3.4:1 |
| Pink | `#FF8FB3` | Blush, ears, sparkles. Never for errors. | 2.1:1 | 8.5:1 |
| Lavender | `#E6DEFD` | Page details, soft surfaces. | 1.3:1 | 14.0:1 |
| Lavender mist | `#F6F2FF` | Brand backgrounds (social, README). | 1.1:1 | 16.4:1 |
| Ink | `#1E1A33` | "manga" in the wordmark, eyes, body text. | 16.8:1 | 1.1:1 |
| Night | `#16132A` | Brand dark background. | 18.1:1 | 1.0:1 |
| Moon | `#F4F1FF` | Text on Night. | 1.1:1 | 16.3:1 |

Text rules: body text uses Ink or Moon. Violet deep (light) and Violet soft (dark) are the only violets for text smaller than 24px. Mint and Pink are for graphics only on light backgrounds.

## Wordmark

Fredoka SemiBold (600, SIL OFL), lowercase: "manga" in Ink and "downloader" in Violet deep, or Moon / Violet soft on dark. The lockups are pre-outlined, so use them as they are and don't retype them. For UI text next to the brand, use the app's own UI font, not Fredoka.

## Clear space & minimum size

- Clear space around the mark and lockups = **¼ of the mark's height** (about one ear) on every side.
- Mark: `mark.svg` from 48px up, `mark-small.svg` at 16–40px. Don't go below 16px.
- Stacked lockup: at least 40px tall (about 160px wide). Below that, use `lockup-line` (min 24px tall) or the mark alone.

## Don'ts

- Don't stretch, rotate or skew the mark, or redraw the face.
- Don't recolour the body outside the palette, or use the light-palette mark on dark surfaces (use `mark-dark`).
- Don't add outlines, glows or heavy drop shadows (the macOS tile shadow is the only one).
- Don't put the mark on busy photos or on Violet without the white tile (use `mark-mono` in white instead).
- Don't use the large `mark.svg` below 48px. Its details turn to mush; use `mark-small.svg`.
- Don't use Pink for errors, or swap the wordmark colours.

## GUI colour mapping (proposal)

The backgrounds and neutrals stay as they are (`--bg` `#f4f3f0` / `#121214`, surfaces, borders, text). The brand drives the accent, success and danger tokens. The warning tokens stay amber, since the palette has no amber.

### Light

| token | value | note | contrast |
|---|---|---|---|
| `--accent` | `#7C5CE0` | was #3550c2 | white text on it 4.7:1, on --bg 4.2:1 |
| `--accent-hover` | `#6A4BD3` | was #2b43ab | 5.9:1 on surface, 5.3:1 on --bg |
| `--accent-text` | `#5B3FC4` | was #2c45ad | 7.1:1 on surface, 6.4:1 on --bg |
| `--accent-soft` | `#EEE9FD` | was #eaeefb |  |
| `--accent-softer` | `#F7F4FE` | was #f4f6fd |  |
| `--on-accent` | `#FFFFFF` | unchanged |  |
| `--success` | `#1B7A5E` | was #237245 | 5.3:1 on surface, 4.7:1 on --bg |
| `--success-solid` | `#1FA27D` | new: mint fills (progress, done ticks) | 3.2:1 on surface (UI graphic, 3:1 target) |
| `--success-soft` | `#E2F7EF` | was #e5f2ea |  |
| `--warning` | `#8F5500` | unchanged | 6.1:1 on surface, 5.5:1 on --bg |
| `--warning-solid` | `#C47A0E` | unchanged | 3.4:1 on surface (UI graphic, 3:1 target) |
| `--danger` | `#C0344B` | was #b8322a | 5.5:1 on surface, 4.9:1 on --bg |
| `--danger-soft` | `#FCE7EB` | was #fbe8e5 |  |
| `--bg` | `#F4F3F0` | unchanged |  |
| `--surface` | `#FFFFFF` | unchanged |  |
| `--text` | `#1B1A18` | unchanged | 17.4:1 on surface, 15.7:1 on --bg |

### Dark

| token | value | note | contrast |
|---|---|---|---|
| `--accent` | `#A993FF` | was #7d93f2 | --on-accent on it 7.2:1, on --bg 7.4:1 |
| `--accent-hover` | `#BBA9FF` | was #93a6f6 | 8.4:1 on surface, 9.1:1 on --bg |
| `--accent-text` | `#BCAAFF` | was #a9b8fa | 8.5:1 on surface, 9.2:1 on --bg |
| `--accent-soft` | `rgb(169 147 255 / 15%)` | was rgb(125 147 242 / 15%) |  |
| `--accent-softer` | `rgb(169 147 255 / 7%)` |  |  |
| `--on-accent` | `#16132A` | was #0d1233 |  |
| `--success` | `#5EE0B8` | was #66c58f | 10.5:1 on surface, 11.4:1 on --bg |
| `--success-solid` | `#5EE0B8` | new | 10.5:1 on surface (UI graphic, 3:1 target) |
| `--success-soft` | `rgb(94 224 184 / 13%)` |  |  |
| `--warning` | `#E9B25A` | unchanged | 9.0:1 on surface, 9.8:1 on --bg |
| `--danger` | `#FF8296` | was #f2786e | 7.3:1 on surface, 7.9:1 on --bg |
| `--danger-soft` | `rgb(255 130 150 / 13%)` |  |  |
| `--bg` | `#121214` | unchanged |  |
| `--surface` | `#1B1B1E` | unchanged |  |
| `--text` | `#ECECEF` | unchanged | 14.6:1 on surface, 15.9:1 on --bg |

`--ring` already derives from `--accent`. Pink is the mascot's colour and is deliberately not mapped to any state. The danger red is cooler (rose-red) so it sits with the palette while staying clearly distinct from the blush pink.
