//go:build unix

package doorbell

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CodexTrust asks the target provider for the effective hooks in this workspace.
// The read-only RPC cannot grant approval or establish an a2a session binding.
func (i *Installer) CodexTrust(ctx context.Context, binary, cwd, home, identity string) error {
	exe, err := i.Resolve(cwd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "app-server")
	cmd.Dir = cwd
	cmd.Env = append(append([]string(nil), i.Environment...), "CODEX_HOME="+home)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() { in.Close(); cancel(); cmd.Wait() }()
	stream := bufio.NewScanner(io.LimitReader(out, 4<<20))
	stream.Buffer(make([]byte, 4096), 2<<20)
	write := json.NewEncoder(in)
	request := func(id int, method string, params any) (json.RawMessage, error) {
		if err := write.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return nil, err
		}
		for stream.Scan() {
			var reply struct {
				ID     int
				Result json.RawMessage
			}
			if err := json.Unmarshal(stream.Bytes(), &reply); err != nil {
				return nil, err
			}
			if reply.ID == id && len(reply.Result) > 0 {
				return reply.Result, nil
			}
			if reply.ID == id {
				break
			}
		}
		return nil, errors.New("Codex hooks RPC unavailable")
	}
	if _, err := request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "curator_run", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}); err != nil {
		return err
	}
	if err := write.Encode(map[string]string{"method": "initialized"}); err != nil {
		return err
	}
	raw, err := request(2, "hooks/list", map[string][]string{"cwds": {cwd}})
	if err != nil {
		return err
	}
	var result struct {
		Data []struct {
			CWD    string
			Errors []json.RawMessage
			Hooks  []struct {
				Command, HandlerType, EventName, SourcePath, TrustStatus, Matcher string
				Enabled                                                           bool
			}
		}
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if len(result.Data) != 1 || result.Data[0].CWD != cwd || len(result.Data[0].Errors) != 0 {
		return errors.New("Codex cannot resolve this workspace's hooks")
	}
	events := map[string]string{"sessionStart": "session-start", "userPromptSubmit": "prompt", "stop": "stop"}
	seen := map[string]bool{}
	for _, h := range result.Data[0].Hooks {
		parts := strings.Fields(h.Command)
		if len(parts) == 0 || filepath.Base(parts[0]) != "a2a" || !strings.Contains(h.Command, " hook ") {
			continue
		}
		actual, err := filepath.EvalSymlinks(parts[0])
		which, ok := events[h.EventName]
		if !ok || seen[h.EventName] || err != nil || !filepath.IsAbs(parts[0]) || actual != exe || (h.SourcePath != filepath.Join(home, codexSettings) && h.SourcePath != filepath.Join(cwd, ".codex", codexSettings)) || strings.Join(parts[1:], " ") != "--as "+identity+" hook "+which+" --runtime codex" || h.Matcher != "" || h.HandlerType != "command" || !h.Enabled || (h.TrustStatus != "trusted" && h.TrustStatus != "managed") {
			return errors.New("a2a hooks require inspection in /hooks")
		}
		seen[h.EventName] = true
	}
	if len(seen) != len(events) {
		return errors.New("Codex did not load all three a2a hooks")
	}
	return nil
}
