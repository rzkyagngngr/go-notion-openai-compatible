package browserrefresh

import (
	"os"
	"strconv"
	"strings"
)

const (
	ModeDisabled = "disabled"
	ModeHeadless = "headless"
	ModeRemote   = "remote"
)

type Config struct {
	Mode           string
	CDPURL         string
	ProfileDir     string
	ChromiumPath   string
	LoginURL       string
	TimeoutSec     int
	MinIntervalSec int
	NoSandbox      bool
}

func LoadConfig() Config {
	cfg := Config{
		ProfileDir:     getenv("NOTION_BROWSER_PROFILE_DIR", "data/browser-profile"),
		ChromiumPath:   getenv("NOTION_BROWSER_CHROMIUM_PATH", ""),
		LoginURL:       getenv("NOTION_BROWSER_LOGIN_URL", "https://www.notion.so/login"),
		TimeoutSec:     getenvInt("NOTION_BROWSER_TIMEOUT_SEC", 120),
		MinIntervalSec: getenvInt("NOTION_BROWSER_MIN_INTERVAL_SEC", 600),
		NoSandbox:      getenvBool("NOTION_BROWSER_NO_SANDBOX", false),
		CDPURL:         strings.TrimSpace(os.Getenv("NOTION_BROWSER_CDP_URL")),
	}

	mode := strings.TrimSpace(os.Getenv("NOTION_BROWSER_MODE"))
	switch mode {
	case ModeDisabled, ModeHeadless, ModeRemote:
		cfg.Mode = mode
	case "":
		if getenvBool("NOTION_BROWSER_HEADLESS", true) {
			cfg.Mode = ModeHeadless
		} else {
			cfg.Mode = ModeRemote
		}
	default:
		cfg.Mode = ModeDisabled
	}

	if cfg.Mode == ModeHeadless {
		if cfg.ChromiumPath == "" {
			if _, err := os.Stat("/usr/bin/chromium"); err == nil {
				cfg.ChromiumPath = "/usr/bin/chromium"
			}
		}
		if cfg.ChromiumPath == "" {
			cfg.Mode = ModeDisabled
		} else if _, err := os.Stat(cfg.ChromiumPath); err != nil {
			cfg.Mode = ModeDisabled
		}
	}
	if cfg.Mode == ModeRemote && cfg.CDPURL == "" {
		cfg.Mode = ModeDisabled
	}
	if cfg.NoSandbox || os.Getenv("NOTION_BROWSER_NO_SANDBOX") == "true" {
		cfg.NoSandbox = true
	}
	return cfg
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
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}