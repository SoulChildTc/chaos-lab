// Package runlog writes one JSON record per instance to ~/.lab/runs. Records are
// written, never read, by the CLI: an evaluation harness walks the directory to
// compute success rate and time-to-fix.
package runlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Result values for Record.Result.
const (
	ResultPassed      = "passed"
	ResultTimeout     = "timeout"
	ResultAbandoned   = "abandoned"
	ResultDirty       = "dirty"
	ResultStartFailed = "start_failed"
)

type Record struct {
	Instance       string          `json:"instance"`
	Plugin         string          `json:"plugin"`
	Case           string          `json:"case"`
	Args           map[string]any  `json:"args,omitempty"`
	StartedAt      time.Time       `json:"started_at"`
	EndedAt        time.Time       `json:"ended_at"`
	DurationSec    int             `json:"duration_sec"`
	Result         string          `json:"result"`
	Checks         int             `json:"checks"`
	Report         json.RawMessage `json:"report,omitempty"`
	ClaimsReleased *bool           `json:"claims_released,omitempty"`
}

// Write stores r at path, creating the parent directory.
func Write(path string, r Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Update rewrites an existing record. A missing record is not an error: the
// caller has nothing to update.
func Update(path string, fn func(*Record)) error {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	fn(&r)
	return Write(path, r)
}
