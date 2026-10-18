package werkstatt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func ancestors(path string) error {
	if filepath.Clean(path) == string(filepath.Separator) {
		return nil
	}
	for p := filepath.Dir(filepath.Clean(path)); ; p = filepath.Dir(p) {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0444 == 0 {
			return failure("werkstatt_context_check_failed", map[string]string{"path": p})
		}
		if _, err = os.ReadDir(p); err != nil {
			return failure("werkstatt_context_check_failed", map[string]string{"path": p})
		}
		for _, name := range []string{"CLAUDE.md", "CLAUDE.local.md", "AGENTS.md", "AGENTS.override.md"} {
			path := filepath.Join(p, name)
			_, err := os.Lstat(path)
			if err == nil {
				return failure("werkstatt_context_leak", map[string]string{"path": path})
			}
			if !errors.Is(err, os.ErrNotExist) {
				return failure("werkstatt_context_check_failed", map[string]string{"path": p})
			}
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}

// Workspace reads no instruction or credential contents. Both lexical and
// physical strict ancestors must be inspectable and free of context files.
func Workspace(ctx context.Context, r *Record, identityDirectory string) error {
	info, err := os.Stat(r.Clone)
	if os.IsPermission(err) {
		return failure("werkstatt_context_check_failed", map[string]string{"path": filepath.Dir(r.Clone)})
	}
	if err != nil || !info.IsDir() {
		return failure("werkstatt_clone_missing", map[string]string{"path": r.Clone})
	}
	physical, err := filepath.EvalSymlinks(r.Clone)
	if err != nil {
		return failure("werkstatt_context_check_failed", map[string]string{"path": r.Clone})
	}
	if r.GitDir != "" {
		bad := func(reason string) error {
			return failure("werkstatt_git_layout_invalid", map[string]string{"reason": reason})
		}
		gitdir, err := filepath.EvalSymlinks(r.GitDir)
		if err != nil {
			return bad("missing")
		}
		info, err := os.Stat(gitdir)
		if err != nil || !info.IsDir() {
			return bad("missing")
		}
		rel, err := filepath.Rel(physical, gitdir)
		if err != nil || rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return bad("inside")
		}
		if _, err := os.Lstat(filepath.Join(r.Clone, ".git")); !errors.Is(err, os.ErrNotExist) {
			return bad("dotgit_present")
		}
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(bounded, "git", "config", "--no-includes", "--file", filepath.Join(gitdir, "config"), "--get", "core.worktree")
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1"}
		cmd.WaitDelay = 100 * time.Millisecond
		b, err := cmd.Output()
		if err != nil {
			return bad("worktree_mismatch")
		}
		target := strings.TrimSuffix(string(b), "\n")
		if !filepath.IsAbs(target) {
			target = filepath.Join(gitdir, target)
		}
		target, err = filepath.EvalSymlinks(target)
		if err != nil || target != physical {
			return bad("worktree_mismatch")
		}
	}
	if err := ancestors(r.Clone); err != nil {
		return err
	}
	if err := ancestors(physical); err != nil {
		return err
	}
	// Presence only; never open or decode an a2a credential.
	if info, err := os.Stat(filepath.Join(identityDirectory, r.Identity+".json")); err != nil || !info.Mode().IsRegular() {
		return failure("werkstatt_identity_missing", map[string]string{"identity": r.Identity})
	}
	return nil
}
