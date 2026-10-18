//go:build unix

// Package doorbell owns the transitional project-local Claude hook boundary.
package doorbell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

type Installer struct {
	Environment         []string
	InvocationDirectory string
	Diagnostics         io.Writer
	once                sync.Once
	executable          string
	selected            string
	err                 error
}

func failure(step string) error {
	return &client.Error{Code: "doorbell_setup_failed", Details: map[string]string{"step": step}}
}
func envValue(env []string, key string) string {
	for _, v := range env {
		if strings.HasPrefix(v, key+"=") {
			return strings.TrimPrefix(v, key+"=")
		}
	}
	return ""
}
func inside(candidate, root string) bool {
	if root == "" {
		return false
	}
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Resolve freezes the invocation PATH choice. Relative entries and both lexical
// and physical workspace/git paths are excluded; no repository helper is run.
func (i *Installer) Resolve(workspace string) (string, error) {
	i.once.Do(func() {
		var roots []string
		for _, root := range []string{i.InvocationDirectory, workspace, envValue(i.Environment, "GIT_WORK_TREE"), envValue(i.Environment, "GIT_DIR")} {
			if root == "" {
				continue
			}
			if !filepath.IsAbs(root) {
				root = filepath.Join(i.InvocationDirectory, root)
			}
			root = filepath.Clean(root)
			roots = append(roots, root)
			if physical, err := filepath.EvalSymlinks(root); err == nil {
				roots = append(roots, physical)
			}
		}
		for _, dir := range filepath.SplitList(envValue(i.Environment, "PATH")) {
			if !filepath.IsAbs(dir) {
				continue
			}
			candidate := filepath.Join(dir, "a2a")
			physical, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				continue
			}
			forbidden := false
			for _, root := range roots {
				forbidden = forbidden || inside(candidate, root) || inside(physical, root)
			}
			if forbidden {
				continue
			}
			info, err := os.Stat(physical)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
				continue
			}
			i.executable, i.selected = physical, candidate
			if i.Diagnostics != nil {
				fmt.Fprintf(i.Diagnostics, "curator-run: a2a executable=%q\n", physical)
			}
			return
		}
		i.err = failure("install")
	})
	// A list invocation can inspect multiple workspaces after freezing PATH.
	// Recheck each workspace without selecting another executable.
	if i.err == nil {
		roots := []string{workspace}
		if physical, err := filepath.EvalSymlinks(workspace); err == nil {
			roots = append(roots, physical)
		}
		for _, root := range roots {
			if inside(i.selected, root) || inside(i.executable, root) {
				return "", failure("install")
			}
		}
	}
	return i.executable, i.err
}
func (i *Installer) Ensure(ctx context.Context, workspace, identity string) error {
	executable, err := i.Resolve(workspace)
	if err != nil {
		return err
	}
	target, err := pinSettings(workspace)
	if err != nil {
		return failure("install")
	}
	defer target.close()
	stage, err := i.stageDirectory()
	if err != nil {
		return failure("install")
	}
	defer os.RemoveAll(stage)
	settings := filepath.Join(stage, "settings.local.json")
	if target.original != nil {
		if err := os.WriteFile(settings, target.data, 0600); err != nil {
			return failure("install")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "hook", "install", "--agent", identity, "--runtime", "claude", "--settings", settings)
	cmd.Dir, cmd.Env = workspace, i.Environment
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 100 * time.Millisecond
	if err = cmd.Start(); err != nil {
		return failure("install")
	}
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if err = cmd.Wait(); err != nil {
		return failure("install")
	}
	staged, err := pinSettingsDirectory(stage)
	if err != nil {
		return failure("verify")
	}
	data, info, err := readSettings(staged)
	_ = unix.Close(staged)
	if err != nil || info == nil || !verify(data, identity, executable) {
		return failure("verify")
	}
	if err := target.commit(data); err != nil {
		return failure("install")
	}
	data, info, err = readSettings(target.directory)
	if err != nil || info == nil || !verify(data, identity, executable) || !target.confirm(workspace) {
		return failure("verify")
	}
	return nil
}

func ownedRegular(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() && stat.Uid == uint32(os.Geteuid()) && stat.Nlink == 1
}

func verify(data []byte, identity, executable string) bool {
	if _, err := fragment.ParseJSON(data); err != nil {
		return false
	}
	var value struct {
		Env   map[string]json.RawMessage `json:"env"`
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	var agent string
	if json.Unmarshal(value.Env["A2A_AGENT"], &agent) != nil || agent != identity {
		return false
	}
	for event, sub := range map[string]string{"SessionStart": "session-start", "UserPromptSubmit": "prompt", "Stop": "stop"} {
		var entries []json.RawMessage
		if json.Unmarshal(value.Hooks[event], &entries) != nil {
			return false
		}
		found := false
		for _, raw := range entries {
			var entry struct {
				Hooks []json.RawMessage `json:"hooks"`
			}
			if json.Unmarshal(raw, &entry) != nil {
				continue
			}
			for _, rawHook := range entry.Hooks {
				var hook struct{ Type, Command string }
				if json.Unmarshal(rawHook, &hook) != nil {
					continue
				}
				words := strings.Split(hook.Command, " ")
				if hook.Type != "command" || len(words) != 7 || !filepath.IsAbs(words[0]) {
					continue
				}
				path, err := filepath.EvalSymlinks(words[0])
				if err != nil || path != executable {
					continue
				}
				found = found || strings.Join(words[1:], " ") == "--as "+identity+" hook "+sub+" --runtime claude"
			}
		}
		if !found {
			return false
		}
	}
	return true
}
