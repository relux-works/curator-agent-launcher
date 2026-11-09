package launcherconfig

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Selected contains references from Core's effective selected machine source.
// It is separate from launcher.json host policy and never carries a credential.
type Selected struct {
	ConfigSchema int
	ConfigPath   string
	Proxy        *Proxy
	ClaudeAuth   *ClaudeAuth
	Notification *Notification
	// ClaudeEnv is set only for a config schema 9 projection; nil otherwise.
	ClaudeEnv *ClaudeEnv
}

// admitLauncherConfigV3 is the candidate gate for curator.launcher-config/3
// (config schema 9, extensions.claude_env). CIP-0029 is in Review and Decision
// 0035 is Proposed, so production readers keep refusing /3 until the
// allocation is accepted; tests enable it to qualify the candidate receiver.
var admitLauncherConfigV3 = false

var ErrSelected = errors.New("invalid selected Core launcher configuration")

const selectedOutputLimit = 64 * 1024

// ReadSelected asks the same PATH-selected Core used for fragment resolution
// exactly once. The caller's directory and environment preserve normal Core
// source selection. Failure never falls back to empty metadata or launcher.json.
func ReadSelected(ctx context.Context, dir string, env []string) (Selected, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, fragment.CuratorBinary, "config", "launcher")
	cmd.Dir, cmd.Env = dir, env
	return readSelectedCommand(cmd)
}

type selectedOutput struct{ buffer bytes.Buffer }

func (b *selectedOutput) Write(data []byte) (int, error) {
	if len(data) > selectedOutputLimit-b.buffer.Len() {
		return 0, ErrSelected
	}
	return b.buffer.Write(data)
}

func readSelectedCommand(cmd *exec.Cmd) (Selected, error) {
	var out selectedOutput
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, &out, io.Discard
	// A descendant may retain the pipe after Core exits; context cancellation
	// alone does not bound that drain. Refuse rather than use partial output.
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return Selected{}, ErrSelected
	}
	return ParseSelected(out.buffer.Bytes())
}

// ParseSelected validates the closed Core projection without opening any store.
// Diagnostics deliberately omit all input bytes, including unknown member names.
func ParseSelected(data []byte) (Selected, error) {
	root, err := fragment.ParseJSON(data)
	if err != nil || !exactMembers(root, "schema", "config_schema", "config_path", "extensions") {
		return Selected{}, ErrSelected
	}
	version, _ := root.Get("config_schema")
	if version.Kind != fragment.KindInt {
		return Selected{}, ErrSelected
	}
	switch version.Int {
	case 1, 2, 3, 5, 7:
		if stringMember(root, "schema") != "curator.launcher-config/1" {
			return Selected{}, ErrSelected
		}
	case 8:
		if stringMember(root, "schema") != "curator.launcher-config/2" {
			return Selected{}, ErrSelected
		}
	case 9:
		if !admitLauncherConfigV3 || stringMember(root, "schema") != "curator.launcher-config/3" {
			return Selected{}, ErrSelected
		}
	default:
		return Selected{}, ErrSelected
	}
	path := stringMember(root, "config_path")
	if !reference(path) {
		return Selected{}, ErrSelected
	}
	// /2 and /3 name the selected source by a clean absolute path on the
	// target platform. /1 keeps its admitted reference grammar.
	if version.Int >= 8 && (!filepath.IsAbs(path) || filepath.Clean(path) != path) {
		return Selected{}, ErrSelected
	}
	extensions, _ := root.Get("extensions")
	if extensions.Kind != fragment.KindObject || (version.Int < 7 && len(extensions.Obj) != 0) {
		return Selected{}, ErrSelected
	}
	result := Selected{ConfigSchema: int(version.Int), ConfigPath: path}
	for _, member := range extensions.Obj {
		switch member.Key {
		case "proxy":
			result.Proxy, err = parseProxy(member.Value)
		case "claude_auth":
			result.ClaudeAuth, err = parseClaudeAuth(member.Value)
		case "notification":
			if version.Int < 8 {
				return Selected{}, ErrSelected
			}
			result.Notification, err = parseNotification(member.Value)
		case "claude_env":
			if version.Int != 9 {
				return Selected{}, ErrSelected
			}
			// Member refusals keep their stable code; the error still
			// matches ErrSelected.
			if result.ClaudeEnv, err = parseClaudeEnv(member.Value); err != nil {
				return Selected{}, err
			}
		default:
			return Selected{}, ErrSelected
		}
		if err != nil {
			return Selected{}, ErrSelected
		}
	}
	return result, nil
}
