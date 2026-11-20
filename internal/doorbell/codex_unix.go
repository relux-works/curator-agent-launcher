//go:build unix

package doorbell

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"golang.org/x/sys/unix"
)

const codexSettings = "hooks.json"

// codexHooks recognizes only the fixed a2a session hooks. A different identity,
// duplicate handler or conditional group cannot satisfy this launch.
func codexHooks(data []byte, identity, executable string) (bool, error) {
	if len(data) == 0 {
		return false, nil
	}
	if _, err := fragment.ParseJSON(data); err != nil {
		return false, err
	}
	var doc struct {
		Hooks map[string][]struct {
			Matcher string
			Hooks   []struct{ Type, Command string }
		}
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, err
	}
	expected := map[string]string{"SessionStart": "session-start", "UserPromptSubmit": "prompt", "Stop": "stop"}
	seen := map[string]bool{}
	for event, groups := range doc.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				parts := strings.Fields(hook.Command)
				if len(parts) == 0 || filepath.Base(parts[0]) != "a2a" || !strings.Contains(hook.Command, " hook ") {
					continue
				}
				path, err := filepath.EvalSymlinks(parts[0])
				if err != nil || path != executable || !filepath.IsAbs(parts[0]) || expected[event] == "" || group.Matcher != "" ||
					seen[event] || hook.Type != "command" || strings.Join(parts[1:], " ") != "--as "+identity+" hook "+expected[event]+" --runtime codex" {
					return false, errors.New("conflicting a2a hook in target settings")
				}
				seen[event] = true
			}
		}
	}
	if len(seen) > 0 && len(seen) != len(expected) {
		return false, errors.New("incomplete existing a2a hook set")
	}
	return len(seen) > 0, nil
}

// EnsureCodex reuses matching project hooks or merges the sanctioned installer
// output into the target profile. It never writes Codex trust decisions.
func (i *Installer) EnsureCodex(ctx context.Context, workspace, home, identity string) (string, error) {
	exe, err := i.Resolve(workspace)
	if err != nil {
		return "", err
	}
	project := filepath.Join(workspace, ".codex")
	var projectReady bool
	projectFD, err := pinSettingsDirectory(project)
	if err == nil {
		defer unix.Close(projectFD)
		data, _, err := readNamedSettings(projectFD, codexSettings)
		if err != nil {
			return "", err
		}
		projectReady, err = codexHooks(data, identity, exe)
		if err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	fd, err := pinSettingsDirectory(home)
	if err != nil {
		return "", err
	}
	target := settingsTarget{directory: fd, filename: codexSettings}
	defer target.close()
	// Serialize concurrent changes made by this installer to the same home.
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return "", err
	}
	target.data, target.original, err = readNamedSettings(fd, codexSettings)
	if err != nil {
		return "", err
	}
	ready, err := codexHooks(target.data, identity, exe)
	if err != nil {
		return "", err
	}
	if projectReady && ready {
		return "", errors.New("duplicate project and profile a2a hooks")
	}
	if projectReady {
		return filepath.Join(project, codexSettings), nil
	}
	path := filepath.Join(home, codexSettings)
	if ready {
		return path, nil
	}
	stage, err := i.stageDirectory()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	scratch := filepath.Join(stage, codexSettings)
	if target.original != nil {
		if err := os.WriteFile(scratch, target.data, 0600); err != nil {
			return "", err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pathOption := "--executable"
	if info, err := os.Lstat(i.selected); err == nil && info.Mode()&os.ModeSymlink != 0 {
		pathOption = "--installed-command"
	}
	cmd := exec.CommandContext(ctx, exe, "hook", "install", "--agent", identity, "--runtime", "codex", "--settings", scratch, pathOption, i.selected)
	cmd.Dir, cmd.Env = workspace, i.Environment
	if err := cmd.Run(); err != nil {
		return "", failure("install")
	}
	stageFD, err := pinSettingsDirectory(stage)
	if err != nil {
		return "", err
	}
	defer unix.Close(stageFD)
	data, _, err := readNamedSettings(stageFD, codexSettings)
	if err != nil {
		return "", err
	}
	ok, err := codexHooks(data, identity, exe)
	if err != nil || !ok {
		return "", failure("verify")
	}
	if err := target.commit(data); err != nil {
		return "", err
	}
	return path, nil
}
