#!/bin/bash
# The 03 shot: the Language <select> open, on Catalan. A native popup can't be
# opened from JS, so this clicks it (cliclick) at its offset in the window;
# WebKit draws the popup inside the window, so one capture holds both. The
# pointer moves, but screencapture -l leaves the cursor out.
#
# usage: shot-select.sh OUT.png light|dark
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
. "$HERE/lib.sh"
BIN=${BIN:-$HERE/../../manga-downloader-gui}

SHOT_W=915 SHOT_H=578 SHOT_THEME=$2 SHOT_Q='?mock&scenario=loaded&freeze&shot' "$BIN" >/dev/null 2>&1 &
pid=$!
sleep 1.5
focus $pid
sleep 3
read -r id _ X Y _ _ < <("$HERE/bin/winall" $pid | awk '$2==0')
# the Language select, in points from the window's top-left (title bar included)
focus $pid
cliclick "c:$((X + 446)),$((Y + 114))"
sleep 1.2
screencapture -x -l "$id" "$1"
stop $pid
