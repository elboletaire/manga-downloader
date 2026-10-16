#!/bin/bash
# Retakes every desktop-app screenshot (light and dark) from the app in mock
# mode, then exports the README PNGs and, with --install, the website WebPs.
# macOS only: it captures the real window with screencapture. See README.md.
#
# usage: tools/screenshots/capture-all.sh [--install]
#   without --install the PNGs stay in tools/screenshots/out/
#
# Needs: Xcode command line tools (swiftc), cliclick, pngquant, ImageMagick
# (website step), and python3. The terminal running it needs Accessibility
# (to focus the window and click) and Screen Recording permissions.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
WORK=$HERE/work
cd "$HERE"

# helpers: window ids and bounds by pid
mkdir -p bin
swiftc -O winid.swift -o bin/winid
swiftc -O winall.swift -o bin/winall

# Pillow for the crop, in a throwaway venv
[ -x "$WORK/venv/bin/python" ] || { python3 -m venv "$WORK/venv" && "$WORK/venv/bin/pip" -q install pillow; }

# build the app with the capture hooks, and take them out again whatever happens
git -C "$ROOT" apply "$HERE/capture.patch"
trap 'git -C "$ROOT" apply -R "$HERE/capture.patch"' EXIT
VITE_MOCK=1 make -C "$ROOT" build/gui
git -C "$ROOT" apply -R "$HERE/capture.patch"
trap - EXIT

rm -rf "$WORK/raw" out
mkdir -p "$WORK/raw"
# the Options panel scrolled to Format / Pack as / Convert to JPEG
SCROLL='const o=document.querySelector(".options");let e=o;while(e&&e.scrollHeight<=e.clientHeight)e=e.parentElement;if(e)e.scrollTop=105;'
# two volumes folded, to show the groups collapse. One at a time: React
# re-renders between them, and the list is virtual, so the second header only
# exists once the first volume has folded.
FOLD='const f=()=>document.querySelector(".fold.open")?.click();f();setTimeout(f,400);'

for theme in light dark; do
  shot() { ./shot.sh "$WORK/raw/$1-$theme.png" $theme "$2" 4 "${3:-}"; }
  shot 02-loaded '?mock&scenario=loaded&freeze&shot'
  ./shot-select.sh "$WORK/raw/03-language-selector-$theme.png" $theme
  shot 04-downloading '?mock&scenario=running&freeze&shot'
  shot 05-done '?mock&scenario=done&freeze&shot'
  shot 06-options '?mock&scenario=loaded&freeze&shot&epub' "$SCROLL"
  shot 07-volumes '?mock&scenario=volumes&freeze&shot' "$FOLD"
  shot 08-browser-alert '?mock&scenario=check&freeze&shot'
  shot 09-update '?mock&scenario=update&freeze&shot'
  shot 10-failed '?mock&scenario=done&freeze&fail&shot'
  shot 11-update-prompt '?mock&scenario=update&freeze&shot&prompt'
done

# 2054x1444 -> 2000x1390, then 8-bit palette PNGs (~190-230 KB)
"$WORK/venv/bin/python" -I crop.py "$WORK/raw" out
pngquant --force --ext .png --speed 1 256 out/*.png

[ "${1:-}" = --install ] || { echo "PNGs in $HERE/out"; exit 0; }

cp -f out/*.png "$ROOT/assets/screenshots/"
# website: the crops (geometry in the script) and the hero, see website/README.md
cd "$ROOT/website"
build/crop-screenshots.sh ../assets/screenshots src/assets/screenshots
for theme in light dark; do
  for width in 1600 1000; do
    magick "../assets/screenshots/02-loaded-$theme.png" -filter Lanczos -resize "${width}x" -strip \
      -define webp:method=6 -quality 85 "src/assets/screenshots/app-overview-$theme-$width.webp"
  done
done
