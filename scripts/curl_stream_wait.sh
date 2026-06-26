#!/bin/bash
set -euo pipefail
ACCOUNT="${1:-/tmp/notion_account.json}"
BODY="${2:-/tmp/try_body.json}"
COOKIE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['full_cookie'])")
SPACE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['space_id'])")
USER=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['user_id'])")
echo "streaming 60s..."
timeout 60 curl -sN \
  -X POST https://app.notion.com/api/v3/runInferenceTranscript \
  -H 'content-type: application/json' \
  -H 'accept: application/x-ndjson' \
  -H 'notion-client-version: 23.13.20260616.2105' \
  -H "x-notion-space-id: $SPACE" \
  -H "x-notion-active-user-header: $USER" \
  -H "cookie: $COOKIE" \
  --data-binary @"$BODY" | tee /tmp/stream_out.bin | xxd | head -40
wc -c /tmp/stream_out.bin