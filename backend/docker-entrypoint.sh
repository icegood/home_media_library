#!/bin/sh
set -eu

mkdir -p \
  /runtime/app-data \
  /runtime/app-config/gateway \
  /runtime/app-config/logs \
  /runtime/thumbnails \
  /runtime/caddy-data \
  /runtime/caddy-config

chown -R app:app /runtime
find /runtime/thumbnails -type d -exec chmod 750 {} +
find /runtime/thumbnails -type f -exec chmod 640 {} +

umask 0027
exec su-exec app "$@"
