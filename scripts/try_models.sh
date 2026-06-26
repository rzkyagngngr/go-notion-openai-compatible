#!/bin/bash
set -euo pipefail
ACCOUNT="${1:-/tmp/notion_account.json}"
COOKIE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['full_cookie'])")
SPACE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['space_id'])")
USER=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['user_id'])")
CLIENT="23.13.20260616.2105"

for MODEL in oatmeal-cookie ambrosia-tart-high apricot-sorbet-high; do
  echo "=== model $MODEL ==="
  python3 - <<PY
import json, uuid, subprocess, os
from datetime import datetime, timezone

acc = json.load(open("$ACCOUNT"))
now = datetime.now(timezone.utc).astimezone().strftime("%Y-%m-%dT%H:%M:%S.000%z")
now = now[:-2] + ":" + now[-2:]
config_id = str(uuid.uuid4())
context_id = str(uuid.uuid4())
thread_id = str(uuid.uuid4())
trace_id = str(uuid.uuid4())
body = {
  "traceId": trace_id,
  "spaceId": acc["space_id"],
  "transcript": [
    {"id": config_id, "type": "config", "value": {
      "type": "workflow", "modelFromUser": True, "model": "$MODEL",
      "useWebSearch": False, "internetAccess": False,
      "searchScopes": [], "useRulePrioritization": True,
      "availableConnectors": [], "customConnectorInfo": [],
      "isHipaa": False, "useReadOnlyMode": False, "writerMode": False,
      "isCustomAgent": False, "isCustomAgentBuilder": False,
      "isAgentResearchRequest": False, "isMobile": False,
    }},
    {"id": context_id, "type": "context", "value": {
      "timezone": acc.get("timezone", "America/Los_Angeles"),
      "userName": acc.get("user_name") or acc.get("user_email", ""),
      "userId": acc["user_id"],
      "userEmail": acc.get("user_email", ""),
      "spaceName": acc.get("space_name", acc["space_id"]),
      "spaceId": acc["space_id"],
      "spaceViewId": acc.get("space_view_id", ""),
      "currentDatetime": now,
      "surface": "ai_module",
    }},
    {"id": str(uuid.uuid4()), "type": "user", "value": [["halo"]],
     "userId": acc["user_id"], "createdAt": now},
  ],
  "threadId": thread_id,
  "createThread": True,
  "isPartialTranscript": False,
  "generateTitle": False,
  "saveAllThreadOperations": False,
  "setUnreadState": False,
  "threadType": "workflow",
  "asPatchResponse": True,
  "patchResponseVersion": 2,
  "hasHeartbeat": False,
  "createdSource": "ai_module",
  "isUserInAnySalesAssistedSpace": False,
  "isSpaceSalesAssisted": False,
  "debugOverrides": {
    "emitAgentSearchExtractedResults": True,
    "cachedInferences": {},
    "annotationInferences": {},
    "emitInferences": False,
  },
  "threadParentPointer": {"table": "space", "id": acc["space_id"], "spaceId": acc["space_id"]},
}
open("/tmp/try_body.json", "w").write(json.dumps(body))
PY
  BYTES=$(curl -s -w '%{size_download}' -o /tmp/try_resp.bin \
    -X POST https://app.notion.com/api/v3/runInferenceTranscript \
    -H 'content-type: application/json' \
    -H 'accept: application/x-ndjson' \
    -H "notion-client-version: $CLIENT" \
    -H "x-notion-space-id: $SPACE" \
    -H "x-notion-active-user-header: $USER" \
    -H "cookie: $COOKIE" \
    --data-binary @/tmp/try_body.json)
  echo "bytes=$BYTES head=$(xxd -p /tmp/try_resp.bin | head -c 80)"
done