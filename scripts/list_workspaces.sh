#!/bin/bash
set -euo pipefail
ACCOUNT="${1:-/tmp/notion_account.json}"
COOKIE=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['full_cookie'])")
USER=$(python3 -c "import json; print(json.load(open('$ACCOUNT'))['user_id'])")
curl -s \
  -X POST https://app.notion.com/api/v3/loadUserContent \
  -H 'content-type: application/json' \
  -H 'accept: application/json' \
  -H 'notion-client-version: 23.13.20260616.2105' \
  -H "x-notion-active-user-header: $USER" \
  -H "cookie: $COOKIE" \
  -d '{"cursor":{"stack":[]},"limit":100}' > /tmp/luc.json
python3 - <<'PY'
import json
data = json.load(open("/tmp/luc.json"))
rm = data.get("recordMap", {})
spaces = rm.get("space", {})
views = rm.get("space_view", {})
print("workspaces:")
for vid, vent in views.items():
    val = (vent.get("value") or {}).get("value") or {}
    sid = val.get("space_id") or val.get("parent_id")
    sval = ((spaces.get(sid) or {}).get("value") or {}).get("value") or {}
    name = sid
    nl = sval.get("name") or []
    if nl and nl[0]:
        name = nl[0][0]
    print(f"  space_id={sid} space_view_id={vid} name={name!r}")
print("current account:", json.load(open("/tmp/notion_account.json")))
PY