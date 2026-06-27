#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
SPACE="${1:-38298ed3-0d46-8144-a1eb-00033da67864}"
curl -s -X POST http://127.0.0.1:8787/api/session/refresh -H "Content-Type: application/json"
echo
curl -s "http://127.0.0.1:8787/api/session" | head -c 800
echo