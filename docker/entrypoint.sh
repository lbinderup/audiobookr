#!/bin/sh
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# Create or remap the runtime user/group to the requested ids.
if getent group abc >/dev/null 2>&1; then
    groupmod -o -g "$PGID" abc
else
    addgroup -g "$PGID" abc 2>/dev/null || groupmod -o -g "$PGID" "$(getent group "$PGID" | cut -d: -f1)" 2>/dev/null || addgroup abc
fi
if getent passwd abc >/dev/null 2>&1; then
    usermod -o -u "$PUID" abc
else
    adduser -D -H -G abc -u "$PUID" abc
fi

# Prepare /config BEFORE the app starts, so the app (running as abc) creates
# its own database with correct ownership. Non-recursive chown on purpose:
# recursing over a large log/work tree on NAS storage is slow, and files the
# app created are already owned by abc.
mkdir -p /config/logs /config/work
chown abc:abc /config /config/logs /config/work
# Migration aid: adopt a database created by an older/root run.
for f in /config/audiobookr.db /config/audiobookr.db-wal /config/audiobookr.db-shm; do
    [ -e "$f" ] && chown abc:abc "$f"
done

# Never chown /input or /output — set PUID/PGID to ids that already own your
# media, the same convention as linuxserver.io images.

exec su-exec abc:abc /app/audiobookr
