#!/bin/sh
set -e
if [ "$(id -u)" = "0" ]; then
  chown -R notionchat:notionchat /app/data /app/threads 2>/dev/null || true
  exec su-exec notionchat "$0" "$@"
fi
exec /app/notionchat "$@"