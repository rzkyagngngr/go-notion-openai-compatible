#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
SPACE="${1:-38298ed3-0d46-8144-a1eb-00033da67864}"
SESSION=$(docker compose exec -T notionchat cat /app/data/session.json)
BID=$(echo "$SESSION" | sed -n 's/.*"notion_browser_id": "\([^"]*\)".*/\1/p')
TOKEN=$(echo "$SESSION" | sed -n 's/.*"token_v2": "\([^"]*\)".*/\1/p')
if [ -z "$BID" ] || [ -z "$TOKEN" ]; then
  echo "Failed to read session.json from container" >&2
  exit 1
fi
curl -s -X POST http://127.0.0.1:8787/api/session \
  -H "Content-Type: application/json" \
  -d "{\"notion_browser_id\":\"$BID\",\"token_v2\":\"$TOKEN\",\"space_name\":\"$SPACE\"}"
echo
curl -s http://127.0.0.1:8787/api/session
echo