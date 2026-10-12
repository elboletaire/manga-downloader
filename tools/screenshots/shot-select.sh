#!/bin/bash
# The 03 shot: the scanlation group <select> open. A native popup can't be
# opened from JS, so this clicks it (cliclick) at its offset in the window;
# WebKit draws the popup inside the window, so one capture holds both. The
# pointer moves, but screencapture -l leaves the cursor out.
#
# usage: shot-select.sh OUT.png light|dark
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
BIN=${BIN:-$HERE/../../manga-downloader-gui}

SHOT_W=915 SHOT_H=578 SHOT_THEME=$2 SHOT_Q='?mock&scenario=loaded&freeze&site=groups' "$BIN" >/dev/null 2>&1 &
pid=$!
sleep 1.5
osascript -e "tell application \"System Events\" to set frontmost of (first process whose unix id is $pid) to true" >/dev/null
sleep 3
read -r id _ X Y _ _ < <("$HERE/bin/winall" $pid | awk '$2==0')
# the Scanlator select, in points from the window's top-left (title bar included)
cliclick "c:$((X + 638)),$((Y + 114))"
sleep 1.2
screencapture -x -l "$id" "$1"
cliclick kp:esc
kill $pid
wait $pid 2>/dev/null || true
