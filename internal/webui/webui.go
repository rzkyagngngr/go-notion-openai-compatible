package webui

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/mughu-id/notionchat/internal/account"
	"github.com/mughu-id/notionchat/internal/config"
	"github.com/mughu-id/notionchat/internal/credentials"
	"github.com/mughu-id/notionchat/internal/errors"
)

const loginPageHTML = `<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>NotionChat — Connect</title>
<style>
  :root { --bg:#0b0d12; --card:#151922; --border:#2a3142; --text:#eef1f6; --muted:#8b93a7; --accent:#5b8def; --ok:#3ecf8e; --err:#f07178; }
  * { box-sizing:border-box; }
  body { margin:0; font-family:Segoe UI,system-ui,sans-serif; background:var(--bg); color:var(--text); }
  .wrap { max-width:520px; margin:0 auto; padding:40px 20px 60px; }
  .logo { font-size:1.4rem; font-weight:700; margin-bottom:6px; }
  .sub { color:var(--muted); font-size:.9rem; margin-bottom:28px; line-height:1.5; }
  .card { background:var(--card); border:1px solid var(--border); border-radius:14px; padding:24px; margin-bottom:16px; }
  .connected { border-color:var(--ok); background:rgba(62,207,142,.08); }
  .badge { display:inline-block; font-size:.75rem; padding:4px 10px; border-radius:20px; background:rgba(91,141,239,.2); color:var(--accent); margin-bottom:12px; }
  .badge.ok { background:rgba(62,207,142,.2); color:var(--ok); }
  label { display:block; font-size:.82rem; color:var(--muted); margin-bottom:6px; }
  input, textarea { width:100%; background:#0e1118; border:1px solid var(--border); color:var(--text); border-radius:8px; padding:11px 12px; font-size:.9rem; margin-bottom:14px; }
  textarea { min-height:72px; font-family:Consolas,monospace; font-size:.82rem; resize:vertical; }
  button { width:100%; background:var(--accent); color:#fff; border:none; border-radius:8px; padding:13px; font-weight:600; cursor:pointer; font-size:.95rem; }
  button:hover { filter:brightness(1.08); }
  button.secondary { background:transparent; border:1px solid var(--accent); color:var(--accent); margin-top:10px; }
  button.danger { background:transparent; border:1px solid var(--err); color:var(--err); margin-top:10px; }
  .hint { font-size:.78rem; color:var(--muted); margin:-8px 0 14px; line-height:1.4; }
  .status { padding:12px; border-radius:8px; margin-bottom:14px; font-size:.88rem; display:none; }
  .status.ok { display:block; background:rgba(62,207,142,.12); color:var(--ok); }
  .status.err { display:block; background:rgba(240,113,120,.12); color:var(--err); }
  .meta { font-size:.85rem; color:var(--muted); line-height:1.6; }
  .meta strong { color:var(--text); }
  .links { margin-top:20px; text-align:center; font-size:.82rem; }
  .links a, .links button.link { color:var(--accent); text-decoration:none; margin:0 8px; background:none; border:none; cursor:pointer; font-size:.82rem; padding:0; }
  code { background:#0e1118; padding:2px 6px; border-radius:4px; font-size:.8rem; }
  .models { margin-top:12px; font-size:.8rem; color:var(--muted); max-height:120px; overflow-y:auto; }
  .models span { display:inline-block; background:#0e1118; border:1px solid var(--border); border-radius:4px; padding:2px 8px; margin:3px 4px 0 0; color:var(--text); }
  .client-box { margin-top:12px; padding:12px; background:#0e1118; border-radius:8px; font-size:.8rem; line-height:1.7; }
  pre.cmd { background:#0e1118; border:1px solid var(--border); border-radius:8px; padding:12px; font-size:.75rem; overflow-x:auto; white-space:pre-wrap; }
</style>
</head>
<body>
<div class="wrap">
  <div class="logo">NotionChat</div>
  <p class="sub">Sesi Notion diperbarui otomatis. Seed pertama dari Chrome Windows via <code>notionsync</code> — tanpa copy-paste manual.</p>

  <div id="status" class="status"></div>
  <div id="connected-card" class="card connected" style="display:none">
    <span class="badge ok">● Terhubung</span>
    <div class="meta" id="connected-meta"></div>
    <div class="client-box" id="client-info"></div>
    <div class="models" id="models-list"></div>
    <button type="button" class="secondary" id="browser-refresh">Recover session (browser)</button>
    <button type="button" class="danger" id="disconnect">Putuskan sesi</button>
  </div>

  <div id="setup-card" class="card">
    <span class="badge">Setup — notionsync (Windows)</span>
    <p class="hint">Login notion.com di Chrome, lalu jalankan di PowerShell (sesi = HTTP cookie dari browser kamu):</p>
    <pre class="cmd">chrome.exe --remote-debugging-port=9222
go run ./cmd/notionsync --cdp http://127.0.0.1:9222 --url {{.ServerURL}} --space WORKSPACE_ID</pre>
  </div>

  {{if .DebugManualAuth}}
  <div id="login-card" class="card">
    <span class="badge">Debug manual auth</span>
    <form id="connect-form">
      <label>notion_browser_id</label>
      <input name="notion_browser_id" placeholder="UUID">
      <label>token_v2</label>
      <input name="token_v2" type="password" placeholder="v03%3A...">
      <label>space_name (opsional)</label>
      <input name="space_name" placeholder="Workspace">
      <button type="submit">Connect</button>
    </form>
  </div>
  {{end}}

  <div class="links">
    <a href="/healthz">Health</a>
    <button type="button" class="link" id="show-models">Models</button>
  </div>
</div>
<script>
const status = document.getElementById('status');
const connectedCard = document.getElementById('connected-card');
const setupCard = document.getElementById('setup-card');
const connectedMeta = document.getElementById('connected-meta');
const apiKey = {{.APIKeyJSON}};

function showStatus(msg, ok) {
  status.textContent = msg;
  status.className = 'status ' + (ok ? 'ok' : 'err');
}

async function loadClientInfo() {
  const res = await fetch('/api/info');
  const info = await res.json();
  document.getElementById('client-info').innerHTML =
    '<strong>Cursor / Postman:</strong><br>' +
    'Base URL: <code>' + info.base_url + '</code><br>' +
    'API Key: <code>' + info.api_key + '</code>';
}

async function loadModels() {
  const res = await fetch('/api/models');
  const data = await res.json();
  const el = document.getElementById('models-list');
  if (!res.ok) {
    el.textContent = data.message || 'Gagal load models';
    return;
  }
  el.innerHTML = (data.data || []).map(m => '<span>' + m.id + '</span>').join('');
}

async function refreshStatus() {
  const res = await fetch('/api/session');
  const data = await res.json();
  if (data.connected) {
    connectedCard.style.display = 'block';
    if (setupCard) setupCard.style.display = 'none';
    connectedMeta.innerHTML =
      '<strong>' + (data.user_name || data.user_email || 'User') + '</strong><br>' +
      'Workspace: <strong>' + (data.space_name || '-') + '</strong><br>' +
      'Source: <code>' + (data.credential_source || '-') + '</code><br>' +
      'Browser: <code>' + (data.browser_mode || 'disabled') + '</code> ' +
      (data.browser_profile_ready ? '(profile ready)' : '(profile empty)') + '<br>' +
      'Token: <code>' + (data.token_v2_preview || '••••') + '</code><br>' +
      '<small>Updated: ' + (data.updated_at || '-') + '</small>';
    loadClientInfo();
    loadModels();
  } else {
    connectedCard.style.display = 'none';
    if (setupCard) setupCard.style.display = 'block';
  }
}

document.getElementById('show-models').addEventListener('click', loadModels);

document.getElementById('browser-refresh')?.addEventListener('click', async () => {
  const res = await fetch('/api/session/browser-refresh', {
    method: 'POST',
    headers: { Authorization: 'Bearer ' + apiKey }
  });
  const data = await res.json();
  showStatus(data.message || (res.ok ? 'OK' : 'Gagal'), res.ok);
  refreshStatus();
});

const form = document.getElementById('connect-form');
if (form) {
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const body = Object.fromEntries(fd.entries());
    const res = await fetch('/api/session', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const data = await res.json();
    showStatus(data.message || (res.ok ? 'Terhubung' : 'Gagal'), res.ok);
    if (res.ok) refreshStatus();
  });
}

document.getElementById('disconnect').addEventListener('click', async () => {
  await fetch('/api/session', { method: 'DELETE' });
  showStatus('Sesi diputus', true);
  refreshStatus();
});

refreshStatus();
</script>
</body>
</html>`

type pageData struct {
	APIKeyJSON       string
	ServerURL        string
	DebugManualAuth  bool
}

var pageTmpl = template.Must(template.New("login").Parse(loginPageHTML))

type Handler struct {
	store    *credentials.Store
	settings *config.Settings
}

func New(store *credentials.Store, settings *config.Settings) *Handler {
	return &Handler{store: store, settings: settings}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.handleHome)
	mux.HandleFunc("GET /api/session", h.handleSessionGet)
	mux.HandleFunc("POST /api/session", h.handleSessionPost)
	mux.HandleFunc("POST /api/session/refresh", h.handleSessionRefresh)
	mux.HandleFunc("POST /api/session/browser-refresh", h.handleSessionBrowserRefresh)
	mux.HandleFunc("POST /api/session/inject", h.handleSessionInject)
	mux.HandleFunc("DELETE /api/session", h.handleSessionDelete)
	mux.HandleFunc("/config", h.redirectHome)
	mux.HandleFunc("/config/", h.redirectHome)
}

func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	keyJSON, _ := json.Marshal(h.settings.APIKey)
	host := h.settings.Host
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	serverURL := "http://" + host
	if h.settings.Port != 80 && h.settings.Port != 443 {
		serverURL += ":" + strconv.Itoa(h.settings.Port)
	}
	_ = pageTmpl.Execute(w, pageData{
		APIKeyJSON:      string(keyJSON),
		ServerURL:       serverURL,
		DebugManualAuth: debugManualAuth(),
	})
}

func debugManualAuth() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("NOTION_DEBUG_MANUAL_AUTH")))
	return v == "1" || v == "true" || v == "yes"
}

func (h *Handler) redirectHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

func (h *Handler) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, h.store.Status())
}

func (h *Handler) handleSessionPost(w http.ResponseWriter, r *http.Request) {
	var input credentials.SessionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	acc, err := h.store.Connect(input)
	if err != nil {
		writeError(w, err.Error(), errors.HTTPStatus(err))
		return
	}
	writeJSON(w, map[string]any{
		"ok":         true,
		"message":    "Sesi Notion terhubung — langsung aktif tanpa restart.",
		"space_name": acc.SpaceName,
		"user_name":  acc.UserName,
	})
}

func (h *Handler) handleSessionRefresh(w http.ResponseWriter, r *http.Request) {
	changed, err := h.store.RefreshAll()
	if err != nil {
		writeError(w, err.Error(), errors.HTTPStatus(err))
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "refreshed": changed,
		"message": "Credential chain refreshed (env / HTTP / browser)",
	})
}

func (h *Handler) handleSessionBrowserRefresh(w http.ResponseWriter, r *http.Request) {
	if !h.verifyInjectKey(r) {
		writeError(w, "Missing or invalid API key", http.StatusUnauthorized)
		return
	}
	changed, err := h.store.RefreshFromBrowser(true)
	if err != nil {
		if strings.Contains(err.Error(), "busy") {
			writeJSONStatus(w, http.StatusGatewayTimeout, map[string]any{
				"ok": false, "error": "browser_busy", "message": err.Error(),
			})
			return
		}
		writeError(w, err.Error(), errors.HTTPStatus(err))
		return
	}
	st := h.store.Status()
	probeOK := h.store.SessionHealthy()
	msg := "Browser refresh complete"
	if !st.BrowserProfileReady {
		msg = "Browser profile not logged in — run notionsync from Windows (see docs/browser-login.md)"
		writeJSONStatus(w, http.StatusUnprocessableEntity, map[string]any{
			"ok": false, "refreshed": changed, "browser_profile_ready": false,
			"probe_ok": probeOK, "message": msg,
		})
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "refreshed": changed,
		"browser_profile_ready": st.BrowserProfileReady,
		"probe_ok":              probeOK,
		"credential_source":     st.CredentialSource,
		"message":               msg,
	})
}

func (h *Handler) handleSessionInject(w http.ResponseWriter, r *http.Request) {
	if !h.verifyInjectKey(r) {
		writeError(w, "Missing or invalid API key", http.StatusUnauthorized)
		return
	}
	var body struct {
		Cookie  string `json:"cookie"`
		TokenV2 string `json:"token_v2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	cookie := strings.TrimSpace(body.Cookie)
	if cookie == "" && body.TokenV2 != "" {
		token := strings.TrimSpace(body.TokenV2)
		if acc, err := h.store.GetAccount(); err == nil && acc != nil {
			cookie = account.BuildCookieFromParts(acc.BrowserID, acc.DeviceID, acc.UserID, token)
		} else {
			st := h.store.Status()
			cookie = account.BuildCookieFromParts(st.NotionBrowserID, "", "", token)
		}
	}
	changed, err := h.store.ApplyInjectedCookie(cookie)
	if err != nil {
		writeError(w, err.Error(), errors.HTTPStatus(err))
		return
	}
	writeJSON(w, map[string]any{
		"ok": true, "updated": changed,
		"message": "Cookie injected — active immediately, no restart",
	})
}

func (h *Handler) verifyInjectKey(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == h.settings.APIKey
}

func (h *Handler) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Disconnect(); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "message": "Sesi diputus"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, msg string, code int) {
	writeJSONStatus(w, code, map[string]any{"ok": false, "message": msg})
}