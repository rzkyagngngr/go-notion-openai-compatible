# Notion AI to OpenAI Compatible (Go)

OpenAI-compatible HTTP API that routes chat requests to **Notion AI** (`runInferenceTranscript`) using your Notion browser session.

## Features

- `POST /v1/chat/completions` (streaming + non-streaming)
- `GET /v1/models`
- `GET /healthz` — session + browser refresh status
- **Automatic credential refresh** — HTTP rotate + headless Chromium fallback (rod)
- **Windows sync** — `cmd/notionsync` harvests cookies from Chrome via CDP → HTTP POST (no manual copy-paste)

## Quick start

### 1. Build & run server

```bash
go run ./cmd/notionchat
```

### 2. Seed session from Windows Chrome

```powershell
# Chrome with CDP
& "C:\Program Files\Google\Chrome\Application\chrome.exe" --remote-debugging-port=9222

# Sync (login notion.com in that Chrome first)
go run ./cmd/notionsync --cdp http://127.0.0.1:9222 --url http://127.0.0.1:8787 --space YOUR_SPACE_ID
```

### 3. Test

```bash
curl http://127.0.0.1:8787/healthz
curl http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer sk-notionchat" \
  -H "Content-Type: application/json" \
  -d '{"model":"ambrosia-tart-high","messages":[{"role":"user","content":"hi"}]}'
```

## Docker (production)

```bash
docker compose up -d --build
```

Seed session via `notionsync` pointing at `https://notion.rizky.app` — see [docs/browser-login.md](docs/browser-login.md).

| Variable | Description |
|----------|-------------|
| `NOTIONCHAT_API_KEY` | Bearer token for clients |
| `NOTION_BROWSER_MODE` | `headless` \| `remote` \| `disabled` |
| `NOTION_ALLOW_INJECT_FILE` | `true` until browser profile seeded |
| `NOTION_DEBUG_MANUAL_AUTH` | `true` to show manual token form at `/` |

## CLIs

| Command | Purpose |
|---------|---------|
| `go run ./cmd/notionsync` | Harvest Chrome cookies → POST `/api/session` |
| `go run ./cmd/notiontool account-cookie` | Print cookie from account file |
| `go run ./cmd/notionlogin` | Break-glass profile seed (Xvfb on Linux) |

## Architecture

```
Windows Chrome (token_v2)
    → notionsync (CDP)
    → HTTP POST /api/session
    → Docker server (session.json)
    → runInferenceTranscript
```

Background worker: env cookie → inject file → HTTP Set-Cookie → headless browser (if stale).

> Educational / unofficial — not affiliated with Notion.