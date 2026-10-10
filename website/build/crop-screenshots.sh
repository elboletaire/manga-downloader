#!/usr/bin/env bash
# Crops the showcase screenshots from the 2000 px originals and writes WebP
# derivatives at 1000 and 600 px wide. See README.md ("Screenshots").
#
# usage: build/crop-screenshots.sh ORIGINALS_DIR OUT_DIR
set -euo pipefail

SRC=$1
OUT=$2
mkdir -p "$OUT"
PAD=56 # transparent margin around the window, for the shadow
RADIUS=18 # window corner radius
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

# crop NAME THEME ORIGINAL WxH+X+Y
crop() {
  local name=$1 theme=$2 original=$3 geo=$4
  local w=${geo%%x*}
  local rest=${geo#*x}
  local h=${rest%%+*}
  local base="$TMP/$name-$theme"

  magick "$SRC/$original" -crop "$geo" +repage "$base-crop.png"
  magick -size "${w}x${h}" xc:none -fill white \
    -draw "roundrectangle 0,0 $((w - 1)),$((h - 1)) $RADIUS $RADIUS" "$base-mask.png"
  magick "$base-crop.png" "$base-mask.png" -compose DstIn -composite -compose over \
    -background none -gravity center -extent "$((w + 2 * PAD))x$((h + 2 * PAD))" \
    \( +clone -background black -shadow 45x14+0+10 \) +swap \
    -background none -compose over -layers merge +repage "$base.png"

  for width in 1000 600; do
    magick "$base.png" -filter Lanczos -resize "${width}x" -strip \
      -define webp:method=6 -quality 85 "$OUT/$name-$theme-$width.webp"
  done
}

for theme in light dark; do
  crop language-selector "$theme" "03-language-selector-$theme.png" 900x1000+85+58
  crop chapter-list "$theme" "04-downloading-$theme.png" 1265x1000+100+278
  crop finished-run "$theme" "05-done-$theme.png" 580x760+1335+248
done
