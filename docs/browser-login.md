# Browser session sync

Notion session = HTTP cookies in **your Chrome** (`token_v2`, `notion_browser_id`).  
The server receives them via **HTTP POST** — not by logging in on the Linux server.

## Primary: `notionsync` (Windows)

1. Login to [notion.com](https://www.notion.com) in Chrome (normal browsing).

2. Start Chrome with remote debugging:

```powershell
& "C:\Program Files\Google\Chrome\Application\chrome.exe" --remote-debugging-port=9222
```

3. Sync to local or production server:

```powershell
go run ./cmd/notionsync `
  --cdp http://127.0.0.1:9222 `
  --url https://notion.rizky.app `
  --space 38298ed3-0d46-8144-a1eb-00033da67864
```

4. Verify:

```powershell
curl.exe https://notion.rizky.app/api/session
curl.exe https://notion.rizky.app/healthz
```

## Optional: Task Scheduler

Run `notionsync` every 30 minutes while your PC is on — replaces manual token paste when `token_v2` expires.

## Server fallback (headless Chromium)

After the first HTTP sync, Docker can refresh via headless browser profile (`NOTION_BROWSER_MODE=headless`).  
Trigger manually:

```bash
curl -X POST https://notion.rizky.app/api/session/browser-refresh \
  -H "Authorization: Bearer sk-notionchat"
```

## Break-glass: Linux Xvfb on server

Only if Windows sync is unavailable:

```bash
xvfb-run -a go run ./cmd/notionlogin --profile /app/data/browser-profile --headless
```

## Deploy

`scripts/deploy.ps1` only deploys Docker — run `notionsync` from Windows **before** or **after** deploy to seed `session.json`.