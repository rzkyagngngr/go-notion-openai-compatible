#!/bin/bash
set -euo pipefail
ACCOUNT="${1:-/tmp/notion_account.json}"
COOKIE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['full_cookie'])")
USER=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['user_id'])")
CLIENT="23.13.20260616.2105"

for SPACE in ed8d9f12-34bb-81d8-ac2b-0003af05cdbd d7ade223-7922-8119-8163-0003b9bbc3bc; do
  echo "=== space $SPACE ==="
  python3 - <<PY
import json, uuid
from datetime import datetime, timezone
acc = json.load(open("$ACCOUNT"))
now = datetime.now(timezone.utc).astimezone().strftime("%Y-%m-%dT%H:%M:%S.000%z")
now = now[:-2] + ":" + now[-2:]
sid = "$SPACE"
body = {
  "traceId": str(uuid.uuid4()),
  "spaceId": sid,
  "transcript": [
    {"id": str(uuid.uuid4()), "type": "config", "value": {
      "type": "workflow", "modelFromUser": True, "model": "oatmeal-cookie",
      "useWebSearch": False, "internetAccess": False, "searchScopes": [],
      "useRulePrioritization": True, "availableConnectors": [], "customConnectorInfo": [],
      "isHipaa": False, "useReadOnlyMode": False, "writerMode": False,
      "isCustomAgent": False, "isCustomAgentBuilder": False, "isAgentResearchRequest": False, "isMobile": False,
    }},
    {"id": str(uuid.uuid4()), "type": "context", "value": {
      "timezone": acc.get("timezone", "America/Los_Angeles"),
      "userName": acc.get("user_name") or acc.get("user_email", ""),
      "userId": acc["user_id"], "userEmail": acc.get("user_email", ""),
      "spaceName": sid, "spaceId": sid,
      "spaceViewId": acc.get("space_view_id", ""),
      "currentDatetime": now, "surface": "ai_module",
    }},
    {"id": str(uuid.uuid4()), "type": "user", "value": [["halo"]],
     "userId": acc["user_id"], "createdAt": now},
  ],
  "threadId": str(uuid.uuid4()),
  "createThread": True, "isPartialTranscript": False,
  "generateTitle": False, "saveAllThreadOperations": False, "setUnreadState": False,
  "threadType": "workflow", "asPatchResponse": True, "patchResponseVersion": 2,
  "hasHeartbeat": False, "createdSource": "ai_module",
  "isUserInAnySalesAssistedSpace": False, "isSpaceSalesAssisted": False,
  "debugOverrides": {"emitAgentSearchExtractedResults": True, "cachedInferences": {}, "annotationInferences": {}, "emitInferences": False},
  "threadParentPointer": {"table": "space", "id": sid, "spaceId": sid},
}
open("/tmp/space_body.json","w").write(json.dumps(body))
PY
  BYTES=$(curl -s -w '%{size_download}' -o /tmp/space_resp.bin \
    -X POST https://app.notion.com/api/v3/runInferenceTranscript \
    -H 'content-type: application/json' -H 'accept: application/x-ndjson' \
    -H "notion-client-version: $CLIENT" \
    -H "x-notion-space-id: $SPACE" \
    -H "x-notion-active-user-header: $USER" \
    -H "cookie: $COOKIE" \
    --data-binary @/tmp/space_body.json)
  echo "bytes=$BYTES hex=$(xxd -p /tmp/space_resp.bin | head -c 120)"
done