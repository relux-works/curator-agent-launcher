package selector

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Observation and decision records join on the CCJ-1 request digest. Decision
// weights describe the selector; applied weights stay null for plain launches.
type Observation struct {
	Kind          string    `json:"kind"`
	RequestDigest string    `json:"request_digest"`
	Runtime       string    `json:"runtime"`
	Model         string    `json:"model"`
	Effort        string    `json:"effort"`
	WeightsID     *string   `json:"weights_id"`
	Outcome       string    `json:"outcome,omitempty"`
	Decision      *Decision `json:"decision,omitempty"`
	Error         string    `json:"error,omitempty"`
}

func StatePath(xdg, home string) string {
	if xdg == "" {
		xdg = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(xdg, "swarma", "launcher", "selector", "observations.ndjson")
}
func Append(path string, record Observation) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// No final symlink: an observation is never a route for writing another file.
	f, err := openAppend(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return appendLine(f, data)
}
