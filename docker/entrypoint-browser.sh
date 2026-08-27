#!/bin/sh
set -e

# Start a virtual framebuffer unless the user forwarded a real display
# (-e DISPLAY=$DISPLAY -v /tmp/.X11-unix:/tmp/.X11-unix, for interactive
# challenges that need a human click). goVisible() escalates to a headed
# (non-headless) browser when a Cloudflare challenge blocks the headless
# probe; headed Chrome needs a display, and under Xvfb it isn't flagged as
# headless, so managed challenges resolve on their own.
if [ -z "${DISPLAY}" ]; then
    Xvfb :99 -screen 0 1920x1080x24 -nolisten tcp -ac &
    export DISPLAY=:99

    # wait for the X socket so a fast first fetch can't beat the framebuffer
    i=0
    while [ ! -S /tmp/.X11-unix/X99 ] && [ "$i" -lt 50 ]; do
        sleep 0.1
        i=$((i + 1))
    done
fi

# su-exec resets HOME to the passwd entry (/home/manga), which entrypoint.sh's
# adduser -H never creates. Chromium dies without a writable home (profile and
# crashpad dirs — the misleading "chrome_crashpad_handler: --database is
# required" error), so create it owned by the runtime user up front.
mkdir -p /home/manga
chown "${USER_ID:-1000}:${GROUP_ID:-1000}" /home/manga

exec /usr/bin/entrypoint.sh "$@"
