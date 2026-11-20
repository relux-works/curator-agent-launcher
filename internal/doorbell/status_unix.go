//go:build unix

package doorbell

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"time"
)

// WaitForTerminal checks the selected lane through a2a's read-only status API.
// The CLI owns lease freshness and runtime readiness. These observations do not
// identify a provider thread or prove that a particular mailbox event was read.
func (i *Installer) WaitForTerminal(ctx context.Context, cwd, identity string) error {
	exe, err := i.Resolve(cwd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	for {
		cmd := exec.CommandContext(ctx, exe, "--as", identity, "bind", "status", "--json")
		cmd.Dir, cmd.Env = cwd, i.Environment
		var result struct {
			Refresh                 string
			Readiness, Notification struct{ State string }
		}
		if raw, err := cmd.Output(); err == nil && json.Unmarshal(raw, &result) == nil &&
			result.Refresh == "current" && result.Readiness.State == "ready" && result.Notification.State == "ready" {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("a2a has not confirmed a current terminal binding ready for notifications")
		case <-time.After(400 * time.Millisecond):
		}
	}
}
