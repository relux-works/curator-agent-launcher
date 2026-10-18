package selector

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
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

// Append never waits for another writer. A failed line increments a separate,
// best-effort counter, whose lock is also attempted without waiting.
func Append(path string, record Observation) error {
	return appendRecord(path, record, 0)
}

// AppendWait is reserved for the detached worker; its lock wait is bounded.
func AppendWait(path string, record Observation) error {
	return appendRecord(path, record, 5*time.Second)
}

func appendRecord(path string, record Observation, wait time.Duration) (err error) {
	defer func() {
		if err != nil {
			incrementDropped(path)
		}
	}()
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
	return appendLine(f, data, wait)
}

func incrementDropped(path string) {
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	f, err := openRegular(filepath.Join(filepath.Dir(path), "dropped"), os.O_CREATE|os.O_RDWR)
	if err != nil {
		return
	}
	defer f.Close()
	if lockAppend(f, 0) != nil {
		return
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	data, err := io.ReadAll(io.LimitReader(f, 32))
	if err != nil {
		return
	}
	var count uint64
	if len(data) != 0 {
		count, err = strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		if err != nil || count == ^uint64(0) {
			return
		}
	}
	next := []byte(strconv.FormatUint(count+1, 10) + "\n")
	if _, err = f.WriteAt(next, 0); err == nil {
		_ = f.Truncate(int64(len(next)))
	}
}
