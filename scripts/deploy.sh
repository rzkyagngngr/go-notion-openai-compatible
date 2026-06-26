#!/usr/bin/env bash
# Fast deploy — pakai layer cache Docker, tanpa --no-cache.
set -euo pipefail

REPO_DIR="${1:-/home/Code/project/go-notion-openai-compatible}"

cd "$REPO_DIR"
git pull --ff-only
docker compose up -d --build
docker compose ps notionchat
docker compose logs --tail=8 notionchat