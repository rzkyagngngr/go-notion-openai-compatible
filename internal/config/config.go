package config

import (
	"os"
	"strings"
	"sync"
)

const (
	DefaultBaseURL = "https://app.notion.com/api/v3"
	DefaultAPIKey  = "sk-notionchat"
	DefaultHost    = "127.0.0.1"
	DefaultPort    = 8787
	DefaultModel   = "ambrosia-tart-high"
	DefaultAccount = "notion_account.json"
	DefaultThreads = "threads"
	DefaultSession = "data/session.json"
)

// Settings holds server-only configuration. Notion credentials live in credentials.Store.
type Settings struct {
	APIKey         string
	Host           string
	Port           int
	AccountPath    string
	ThreadStateDir string
	SessionFile    string
	BaseURL        string
	DefaultModel   string
}

var (
	mu       sync.RWMutex
	settings *Settings
)

func Load() *Settings {
	mu.Lock()
	defer mu.Unlock()
	loadDotEnv(".env")
	s := &Settings{
		APIKey:         getenv("NOTIONCHAT_API_KEY", DefaultAPIKey),
		Host:           getenv("NOTIONCHAT_HOST", DefaultHost),
		Port:           getenvInt("NOTIONCHAT_PORT", DefaultPort),
		AccountPath:    getenv("NOTIONCHAT_ACCOUNT", DefaultAccount),
		ThreadStateDir: getenv("NOTIONCHAT_THREADS_DIR", DefaultThreads),
		SessionFile:    getenv("NOTIONCHAT_SESSION_FILE", DefaultSession),
		BaseURL:        strings.TrimRight(getenv("NOTIONCHAT_NOTION_BASE_URL", DefaultBaseURL), "/"),
		DefaultModel:   getenv("NOTIONCHAT_DEFAULT_MODEL", DefaultModel),
	}
	settings = s
	return s
}

func Get() *Settings {
	mu.RLock()
	if settings != nil {
		s := *settings
		mu.RUnlock()
		return &s
	}
	mu.RUnlock()
	return Load()
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}