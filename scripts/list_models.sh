#!/bin/bash
set -euo pipefail
ACCOUNT="${1:-/tmp/notion_account.json}"
COOKIE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['full_cookie'])")
SPACE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['space_id'])")
USER=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['user_id'])")
curl -s \
  -X POST https://app.notion.com/api/v3/getAvailableModels \
  -H 'content-type: application/json' \
  -H 'accept: application/json' \
  -H 'notion-client-version: 23.13.20260616.2105' \
  -H "x-notion-space-id: $SPACE" \
  -H "x-notion-active-user-header: $USER" \
  -H "cookie: $COOKIE" \
  -d "{\"spaceId\":\"$SPACE\"}" > /tmp/models.json
python3 - <<'PY'
import json
data = json.load(open("/tmp/models.json"))
print("restricted:", data.get("restrictedAccessModelsInPickerConfig"))
enabled = [m for m in data.get("models", []) if not m.get("isDisabled")]
disabled = [m for m in data.get("models", []) if m.get("isDisabled")]
print("enabled_count", len(enabled))
for m in enabled[:15]:
    print(" OK", m.get("model"), m.get("modelMessage"))
print("disabled_count", len(disabled))
for m in disabled[:10]:
    print(" NO", m.get("model"), m.get("modelMessage"), m.get("disabledReason"))
PY