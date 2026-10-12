#!/bin/bash
# Launches the patched app on one mock state and captures its window.
#
# usage: shot.sh OUT.png light|dark QUERY [WAIT] [JS]
#   QUERY  the mock URL query, e.g. '?mock&scenario=loaded&freeze'
#   WAIT   seconds to let it settle after focusing it (default 3)
#   JS     run in the page 1.2s after it loads (scrolling a panel, say)
#
# The window is 915x578 pt of content plus macOS's 32 pt title bar, so
# 915x610 pt; screencapture at 2x with the system shadow gives 2054x1444.
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
BIN=${BIN:-$HERE/../../manga-downloader-gui}

SHOT_W=${SHOT_W:-915} SHOT_H=${SHOT_H:-578} SHOT_THEME=$2 SHOT_Q="$3" SHOT_JS="${5:-}" \
  "$BIN" >/dev/null 2>&1 &
pid=$!
id=
for _ in $(seq 1 40); do
  id=$("$HERE/bin/winid" $pid | awk '{print $1}' | head -1)
  [ -n "$id" ] && break
  sleep 0.25
done
# a focused window gets the coloured traffic lights and the larger shadow
osascript -e "tell application \"System Events\" to set frontmost of (first process whose unix id is $pid) to true" >/dev/null
sleep "${4:-3}"
screencapture -x -l "$id" "$1"
kill $pid
wait $pid 2>/dev/null || true
