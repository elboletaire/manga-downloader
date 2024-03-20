#!/bin/sh

# default user id and group id
USER_ID=${USER_ID:-1000}
GROUP_ID=${GROUP_ID:-1000}

# execute the manga-downloader binary with all arguments passed to this script
manga-downloader "$@"

# Set ownership to USER_ID:GROUP_ID for /downloads
chown -R ${USER_ID}:${GROUP_ID} /downloads
