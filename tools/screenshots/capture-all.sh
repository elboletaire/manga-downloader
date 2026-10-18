#!/bin/bash
# Retakes every desktop-app screenshot (light and dark) from the app in mock
# mode, then keeps the lossless PNGs in out/ and, with --install, writes the
# README and website WebPs from them.
# macOS only: it captures the real window with screencapture. See README.md.
#
# usage: [SHOT_LANG=en|es|ca|all] tools/screenshots/capture-all.sh [--install]
#   without --install the PNGs stay in tools/screenshots/out/<lang>/
#   SHOT_LANG (default en) is the window's language, all for every one of them;
#   --install writes en's README WebPs to assets/screenshots/ (the README is
#   English-only) and every language's website crops to the website's folders
#
# Needs: Xcode command line tools (swiftc), cliclick, ImageMagick (the WebPs and
# the website crops), and python3. The terminal running it needs Accessibility
# (to focus the window and click) and Screen Recording permissions.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
WORK=$HERE/work
cd "$HERE"

SHOT_LANG=${SHOT_LANG:-en}
case $SHOT_LANG in
  en | es | ca) LANGS=$SHOT_LANG ;;
  all) LANGS="en es ca" ;;
  *) echo "SHOT_LANG must be en, es, ca or all" >&2; exit 2 ;;
esac
export SHOT_LANG # shot-select.sh reads it

# helpers: window ids and bounds by pid
mkdir -p bin
swiftc -O winid.swift -o bin/winid
swiftc -O winall.swift -o bin/winall

# Pillow for the crop, in a throwaway venv
[ -x "$WORK/venv/bin/python" ] || { python3 -m venv "$WORK/venv" && "$WORK/venv/bin/pip" -q install pillow; }

# build the app with the capture hooks, and take them out again whatever happens
# (the language is a query parameter, so one build serves every language)
git -C "$ROOT" apply "$HERE/capture.patch"
trap 'git -C "$ROOT" apply -R "$HERE/capture.patch"' EXIT
VITE_MOCK=1 make -C "$ROOT" build/gui
git -C "$ROOT" apply -R "$HERE/capture.patch"
trap - EXIT

rm -rf "$WORK/raw" out
# the Options panel scrolled to Format / Pack as / Convert to JPEG
SCROLL='const o=document.querySelector(".options");let e=o;while(e&&e.scrollHeight<=e.clientHeight)e=e.parentElement;if(e)e.scrollTop=105;'
# two volumes (or parts) folded, to show the groups collapse. One at a time: React
# re-renders between them, and the list is virtual, so the second header only
# exists once the first volume has folded.
FOLD='const f=()=>document.querySelector(".fold.open")?.click();f();setTimeout(f,400);'

# --install: en's WebPs go to the README; every language gets its website crops, es and
# ca in their own folders. All read the lossless PNGs in out/, never the WebPs they write.
install_lang() {
  local src=$HERE/out/$SHOT_LANG site=src/assets/screenshots
  if [ "$SHOT_LANG" = en ]; then
    local dest=$ROOT/assets/screenshots
    mkdir -p "$dest"
    for png in "$src"/*.png; do
      magick "$png" -strip -define webp:method=6 -quality 90 "$dest/$(basename "${png%.png}").webp"
    done
    echo "WebPs in $dest"
  else
    site=$site/$SHOT_LANG
  fi
  # website: the crops (geometry in the script) and the hero, see website/README.md
  (
    cd "$ROOT/website"
    mkdir -p "$site"
    build/crop-screenshots.sh "$src" "$site"
    for theme in light dark; do
      for width in 1600 1000; do
        magick "$src/02-loaded-$theme.png" -filter Lanczos -resize "${width}x" -strip \
          -define webp:method=6 -quality 85 "$site/app-overview-$theme-$width.webp"
      done
    done
  )
}

for SHOT_LANG in $LANGS; do
  export SHOT_LANG
  raw=$WORK/raw/$SHOT_LANG
  mkdir -p "$raw"
  for theme in light dark; do
    shot() { ./shot.sh "$raw/$1-$theme.png" $theme "$2&lang=$SHOT_LANG" 4 "${3:-}"; }
    shot 02-loaded '?mock&scenario=loaded&freeze&shot'
    ./shot-select.sh "$raw/03-language-selector-$theme.png" $theme
    shot 04-downloading '?mock&scenario=running&freeze&shot'
    shot 05-done '?mock&scenario=done&freeze&shot'
    shot 06-options '?mock&scenario=loaded&freeze&shot&epub' "$SCROLL"
    shot 07-volumes '?mock&scenario=volumes&freeze&shot' "$FOLD"
    shot 12-parts '?mock&scenario=parts&freeze&shot' "$FOLD"
    shot 08-browser-alert '?mock&scenario=check&freeze&shot'
    shot 09-update '?mock&scenario=update&freeze&shot'
    shot 10-failed '?mock&scenario=done&freeze&fail&shot'
    shot 11-update-prompt '?mock&scenario=update&freeze&shot&prompt'
    shot 13-settings '?mock&scenario=loaded&freeze&shot' 'document.querySelector(".settings-btn")?.click()'
  done

  # 2054x1444 -> 2000x1390, kept as lossless PNGs (the WebPs and crops are made from them)
  "$WORK/venv/bin/python" -I crop.py "$raw" "out/$SHOT_LANG"

  if [ "${1:-}" = --install ]; then install_lang; fi
done

[ "${1:-}" = --install ] || echo "PNGs in $HERE/out/<lang>/"
