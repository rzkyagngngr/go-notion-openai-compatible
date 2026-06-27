package browserrefresh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrBrowserBusy = errors.New("browser refresh already in progress")

type Refresher interface {
	Mode() string
	Enabled() bool
	ExtractSession(ctx context.Context, spaceID string) (cookie string, loggedIn bool, err error)
	Ready(ctx context.Context, spaceID string) (bool, error)
}

type disabledRefresher struct{ mode string }

func (d *disabledRefresher) Mode() string  { return d.mode }
func (d *disabledRefresher) Enabled() bool { return false }
func (d *disabledRefresher) ExtractSession(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (d *disabledRefresher) Ready(context.Context, string) (bool, error) { return false, nil }

func NewRefresher(cfg Config) Refresher {
	if cfg.Mode == ModeDisabled {
		return &disabledRefresher{mode: ModeDisabled}
	}
	return &rodRefresher{cfg: cfg}
}

func clearProfileLocks(profileDir string) {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		_ = os.Remove(filepath.Join(profileDir, name))
	}
}

func workspaceURL(spaceID string) string {
	if spaceID == "" {
		return ""
	}
	compact := strings.ReplaceAll(spaceID, "-", "")
	return fmt.Sprintf("https://www.notion.so/%s", compact)
}