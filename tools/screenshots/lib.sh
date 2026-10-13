# Sourced by the shot scripts.
#
# focus PID brings the app to the front and waits
# until macOS reports it frontmost: a single request sometimes loses the race
# with the window appearing, and an inactive window is captured with grey
# traffic lights and a smaller shadow (1922x1312 instead of 2054x1444).
focus() {
  local front
  for _ in $(seq 1 20); do
    osascript -e "tell application \"System Events\" to set frontmost of (first process whose unix id is $1) to true" >/dev/null 2>&1 || true
    sleep 0.25
    front=$(osascript -e 'tell application "System Events" to unix id of first process whose frontmost is true' 2>/dev/null || true)
    [ "$front" = "$1" ] && return 0
  done
  echo "couldn't bring pid $1 to the front" >&2
  return 1
}

# stop PID closes the app. An open native popup (03's <select>) runs its own
# event loop and the app ignores SIGTERM until it closes, so it gets Esc first,
# sent to that process rather than whatever is frontmost, and SIGKILL if it's
# still there after 2s.
stop() {
  osascript -e "tell application \"System Events\" to tell (first process whose unix id is $1) to key code 53" >/dev/null 2>&1 || true
  kill "$1" 2>/dev/null || true
  for _ in $(seq 1 8); do
    kill -0 "$1" 2>/dev/null || { wait "$1" 2>/dev/null || true; return 0; }
    sleep 0.25
  done
  kill -9 "$1" 2>/dev/null || true
  wait "$1" 2>/dev/null || true
}
