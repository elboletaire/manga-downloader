#!/bin/bash
# The 03 shot: the Language <select> open on the mock's default site, with
# the window's language selected. A native popup can't be opened from JS, so this clicks it
# (cliclick); WebKit draws the popup inside the window, so one capture holds
# both. The pointer moves, but screencapture -l leaves the cursor out.
#
# usage: [SHOT_LANG=en|es|ca] shot-select.sh OUT.png light|dark
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
. "$HERE/lib.sh"
BIN=${BIN:-$HERE/../../manga-downloader-gui}

SHOT_W=915 SHOT_H=578 SHOT_THEME=$2 SHOT_Q="?mock&scenario=loaded&freeze&shot&lang=${SHOT_LANG:-en}" "$BIN" >/dev/null 2>&1 &
pid=$!
sleep 1.5
focus $pid
sleep 3
read -r id _ X Y _ _ < <("$HERE/bin/winall" $pid | awk '$2==0')
# the Language select, in points from the window's top-left (title bar
# included): its centre in the 2x capture, less crop.py's 85/56 px margin, /2
focus $pid
cliclick "c:$((X + 265)),$((Y + 114))"
sleep 1.2
screencapture -x -l "$id" "$1"
stop $pid
