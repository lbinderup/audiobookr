#!/bin/sh
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# UMASK controls the permissions of everything audiobookr creates (and
# everything ffmpeg/tone create, since they inherit it).
#
#   002 (default) -> files 664, dirs 775: the group can write.
#   022           -> files 644, dirs 755: only the owner can write.
#
# 002 is the default because this app writes into a media library that is
# normally shared between your own account and other containers' service
# accounts. With a shared group on the library, both can manage the files.
umask "${UMASK:-002}"

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

# If extra group ids are supplied, add the runtime user to them. This is how
# you give the container membership of a shared media group without changing
# its primary group:  SUPPLEMENTARY_GIDS=100,1002
if [ -n "$SUPPLEMENTARY_GIDS" ]; then
    for gid in $(echo "$SUPPLEMENTARY_GIDS" | tr ',' ' '); do
        gname="$(getent group "$gid" | cut -d: -f1)"
        if [ -z "$gname" ]; then
            gname="extra$gid"
            addgroup -g "$gid" "$gname" 2>/dev/null || true
        fi
        addgroup abc "$gname" 2>/dev/null || true
    done
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

echo "audiobookr starting as uid=$PUID gid=$PGID umask=$(umask)"
exec su-exec abc:abc /app/audiobookr
