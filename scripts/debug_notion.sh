#!/bin/bash
set -euo pipefail
ACCOUNT="${1:-/tmp/notion_account.json}"
BODY="${2:-/tmp/infer_body.json}"
COOKIE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['full_cookie'])")
SPACE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['space_id'])")
USER=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['user_id'])")

echo "=== infer body head ==="
head -c 1500 "$BODY"
echo
echo "=== getAvailableModels ==="
curl -s -w '\nbytes:%{size_download} code:%{http_code}\n' \
  -X POST https://app.notion.com/api/v3/getAvailableModels \
  -H 'content-type: application/json' \
  -H 'accept: application/json' \
  -H 'notion-client-version: 23.13.20260616.2105' \
  -H "x-notion-space-id: $SPACE" \
  -H "x-notion-active-user-header: $USER" \
  -H "cookie: $COOKIE" \
  -d "{\"spaceId\":\"$SPACE\"}" | head -c 400
echo
echo "=== runInferenceTranscript ==="
bash "$(dirname "$0")/curl_notion_infer.sh" "$ACCOUNT" "$BODY"