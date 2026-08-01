#!/bin/sh
set -eu

# 1. Fallback default UIDs/GIDs if not passed via env
USER_UID="${MEDIA_UID}"
USER_GID="${MEDIA_GID}"
DOCKER_SOCKET_GID="${DOCKER_GID}"

# 2. Ensure all target runtime directories exist
mkdir -p \
  /runtime/app-data \
  /runtime/app-config/gateway \
  /runtime/app-config/logs \
  /runtime/thumbnails \
  /runtime/caddy-data \
  /runtime/caddy-config \
  /data \
  /gateway

# 3. Resolve or create Group by GID
if ! getent group "$USER_GID" >/dev/null 2>&1; then
    addgroup -g "$USER_GID" appgroup
fi
APP_GROUP="$(getent group "$USER_GID" | cut -d: -f1)"

# 4. Resolve or create User by UID
if ! getent passwd "$USER_UID" >/dev/null 2>&1; then
    adduser -D -u "$USER_UID" -G "$APP_GROUP" appuser
fi
APP_USER="$(getent passwd "$USER_UID" | cut -d: -f1)"

# 5. Optionally configure Docker socket permissions
if [ -n "$DOCKER_SOCKET_GID" ]; then
    if ! getent group "$DOCKER_SOCKET_GID" >/dev/null 2>&1; then
        addgroup -g "$DOCKER_SOCKET_GID" dockersock
    fi
    DOCKER_GROUP="$(getent group "$DOCKER_SOCKET_GID" | cut -d: -f1)"
    addgroup "$APP_USER" "$DOCKER_GROUP" >/dev/null 2>&1 || true
fi

# 6. Apply permissions & file mask
chown -R "$USER_UID:$USER_GID" /runtime /data /gateway
find /runtime/thumbnails -type d -exec chmod 770 {} +
find /runtime/thumbnails -type f -exec chmod 660 {} +

umask 0027

# 7. Drop privileges and execute app as PID 1
exec su-exec "$APP_USER" "$@"