#!/bin/sh
set -e

# default user id and group id
USER_ID="${USER_ID:-1000}"
GROUP_ID="${GROUP_ID:-1000}"

# Set up a runtime user matching the requested UID/GID so files are written
# with the correct ownership from the start. Only /etc/passwd and /etc/group
# are touched — never the mounted volume.
if ! getent group manga >/dev/null 2>&1; then
    addgroup -g "${GROUP_ID}" manga 2>/dev/null || addgroup manga
fi
if ! id manga >/dev/null 2>&1; then
    adduser -D -H -u "${USER_ID}" -G manga manga 2>/dev/null || \
        adduser -D -H -u "${USER_ID}" manga
fi

# Ensure the top-level /downloads dir is writable by the runtime user.
# Non-recursive on purpose: a mounted host volume is left untouched beyond
# this single directory node (already owned by the user in the common
# -v $PWD:/downloads case, so this is a no-op there).
chown "${USER_ID}:${GROUP_ID}" /downloads 2>/dev/null || true

# su-exec resets HOME to the passwd entry (/home/manga), which adduser -H never
# creates. Without a writable home the Manga Plus device secret can't be cached
# (a new device would be registered on every run) and Chromium dies in the
# :browser image (the misleading "chrome_crashpad_handler: --database is
# required" error). Its .config is created too, so a volume mounted there is
# writable by the runtime user. Non-recursive, like /downloads above.
mkdir -p /home/manga/.config
chown "${USER_ID}:${GROUP_ID}" /home/manga /home/manga/.config 2>/dev/null || true

# Drop root and run manga-downloader as the non-root user. exec keeps
# manga-downloader as PID 1 so SIGINT/SIGTERM are delivered directly.
exec su-exec manga manga-downloader "$@"
