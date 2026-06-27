#!/bin/bash
set -euo pipefail
ACCOUNT="${NOTIONCHAT_ACCOUNT:-data/notion_account.json}"
go run ./cmd/notiontool account-field space_id
go run ./cmd/notiontool account-field space_name
go run ./cmd/notiontool account-field space_view_id