package thread

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mughu-id/notionchat/internal/errors"
)

type State struct {
	ThreadID         string   `json:"thread_id"`
	ConfigID         string   `json:"config_id"`
	ContextID        string   `json:"context_id"`
	OriginalDatetime string   `json:"original_datetime"`
	NotionModel      string   `json:"notion_model"`
	UpdatedConfigIDs []string `json:"updated_config_ids,omitempty"`
	LastActivityISO  string   `json:"last_activity_iso,omitempty"`
}

func Path(threadID, baseDir string) string {
	return filepath.Join(baseDir, threadID+".json")
}

func Save(state *State, baseDir string) error {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(state.ThreadID, baseDir), append(b, '\n'), 0o644)
}

func Load(threadID, baseDir string) (*State, error) {
	p := Path(threadID, baseDir)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, errors.New("No saved thread state for "+threadID+". Start a new conversation.", 400)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, errors.New("Corrupt thread state: "+err.Error(), 500)
	}
	return &state, nil
}