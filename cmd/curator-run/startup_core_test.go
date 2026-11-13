//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
)

type startupCoreFixture struct {
	entry                     *pipelineFixture
	core, config, lock, skill string
	context, mcp              string
	signer, publicKey         string
	env                       []string
}

// This optional docking suite executes a separately supplied Core binary and
// the production launcher entry/resolver/selected reader, ending at an inert
// Go child. All config, Git transport, native adapter homes and generated state
// belong to the fixture. Real HOME stays unchanged; no live profile is adopted.
// Local Git transport does not qualify a real network or vendor credential.
func newStartupCoreFixture(t *testing.T, core string, native, legacy, withMCP bool) *startupCoreFixture {
	t.Helper()
	f := entryFixture(t, "codex_cli")
	state := filepath.Join(f.dir, "core-state")
	for _, path := range []string{state, filepath.Join(f.dir, "native", "codex"), filepath.Join(f.dir, "native", "claude"), filepath.Join(f.dir, "native", "xdg"), filepath.Join(f.dir, "native", "pi")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(core)
	if err != nil {
		t.Fatal(err)
	}
	core = filepath.Join(f.dir, "curator")
	writeFixture(t, core, data, 0700)
	path := f.dir + string(os.PathListSeparator) + os.Getenv("PATH")
	// exec.Command resolves curator using the parent PATH before setting Env.
	t.Setenv("PATH", path)
	x := &startupCoreFixture{entry: f, core: core, config: filepath.Join(state, "config.json"), lock: filepath.Join(state, "profiles", "startup-fixture", "lock.json")}
	gitConfig := filepath.Join(f.dir, "gitconfig")
	writeFixture(t, gitConfig, nil, 0600)
	x.env = []string{
		"PATH=" + path, "HOME=" + os.Getenv("HOME"),
		"CURATOR_CONFIG=" + x.config, "CURATOR_SYSTEM_CONFIG=" + filepath.Join(state, "system.json"),
		"CODEX_HOME=" + filepath.Join(f.dir, "native", "codex"),
		"CLAUDE_CONFIG_DIR=" + filepath.Join(f.dir, "native", "claude"),
		"XDG_CONFIG_HOME=" + filepath.Join(f.dir, "native", "xdg"),
		"PI_CODING_AGENT_DIR=" + filepath.Join(f.dir, "native", "pi"),
		"GIT_CONFIG_GLOBAL=" + gitConfig, "GIT_CONFIG_NOSYSTEM=1", "GIT_ALLOW_PROTOCOL=file", "GIT_TERMINAL_PROMPT=0",
		"CURATOR_TEST_RELEASE=codex-cli 0.153.2",
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		if value := os.Getenv(name); value != "" {
			x.env = append(x.env, name+"="+value)
		}
	}
	writeFixture(t, filepath.Join(state, "system.json"), []byte(`{"schema_version":2}`), 0600)
	// A synthetic file exists only in the redirected fixture native home.
	writeFixture(t, filepath.Join(f.dir, "native", "codex", "auth.json"), []byte(`{"fixture":true}`), 0600)
	// Preserve hardened source verification with fixture-only signed commits.
	x.signer = filepath.Join(f.dir, "fixture-signer")
	x.command(t, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", x.signer)
	x.publicKey = strings.TrimSpace(string(x.command(t, "ssh-keygen", "-y", "-f", x.signer)))
	x.level(t, 3)
	skillRepo := filepath.Join(f.dir, "skill-repo")
	contextRepo := filepath.Join(f.dir, "context-repo")
	for _, dir := range []string{skillRepo, filepath.Join(contextRepo, "context")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(t, filepath.Join(skillRepo, "SKILL.md"), []byte("---\nname: verified-skill\ndescription: Fixture skill.\n---\nFixture context.\n"), 0600)
	writeFixture(t, filepath.Join(skillRepo, "README.md"), []byte("Frozen reference.\n"), 0600)
	if native {
		writeFixture(t, filepath.Join(skillRepo, "agent-skill.json"), []byte(`{"schema_version":3,"runtime_roots":[],"commands":{},"capabilities":{"network":"none","filesystem":"none","exec":"none","secrets":"none","env_read":"none"}}`), 0600)
	}
	skillCommit := x.gitRepo(t, skillRepo)
	manifest := map[string]any{
		"schema_version": 1, "name": "startup-fixture", "version": "1.0.0",
		"context":  map[string]any{"modules": []any{map[string]any{"path": "root.md"}}},
		"requires": map[string]any{"skills": map[string]any{"verified-skill": map[string]any{"git": "https://example.invalid/fixture/skill", "revision": skillCommit}}},
	}
	mcpRepo, mcpCommit := filepath.Join(f.dir, "mcp-repo"), ""
	if withMCP {
		if err := os.Mkdir(mcpRepo, 0700); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(mcpRepo, "agent-mcp.json"), []byte(`{"schema_version":1,"name":"verified-tool","version":"1.0.0","server":{"transport":"http","url":"https://example.invalid/fixture/service","env_names":[],"environments":["codex_cli"]}}`), 0600)
		writeFixture(t, filepath.Join(mcpRepo, "README.md"), []byte("Frozen tool reference.\n"), 0600)
		mcpCommit = x.gitRepo(t, mcpRepo)
		manifest["requires"].(map[string]any)["mcp"] = map[string]any{"verified-tool": map[string]any{"git": "https://example.invalid/fixture/mcp", "revision": mcpCommit}}
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(contextRepo, "agent-context.json"), encoded, 0600)
	writeFixture(t, filepath.Join(contextRepo, "context", "root.md"), []byte("Fixture instructions.\n"), 0600)
	contextCommit := x.gitRepo(t, contextRepo)
	remotes := [][2]string{{skillRepo, "https://example.invalid/fixture/skill"}, {contextRepo, "https://example.invalid/fixture/context"}}
	if withMCP {
		remotes = append(remotes, [2]string{mcpRepo, "https://example.invalid/fixture/mcp"})
	}
	for _, pair := range remotes {
		// This is the repository's existing isolated local-remote fixture form.
		remote := "file://" + filepath.ToSlash(pair[0])
		x.command(t, "git", "config", "--file", gitConfig, "url."+remote+".insteadOf", pair[1])
	}
	x.command(t, core, "profile", "install", "https://example.invalid/fixture/context", "--revision", contextCommit, "--as", "startup-fixture")
	x.skill = filepath.Join(state, "contexts", "skill", "verified-skill", skillCommit, "README.md")
	x.context = filepath.Join(state, "contexts", "context", "startup-fixture", contextCommit, "context", "root.md")
	if withMCP {
		x.mcp = filepath.Join(state, "contexts", "mcp", "verified-tool", mcpCommit, "README.md")
	}
	lock := x.readLock(t)
	if lock["schema_version"] != float64(6) {
		t.Fatalf("fixture did not freeze complete evidence: schema %v", lock["schema_version"])
	}
	// Provision the managed home before taking the startup state preimages.
	x.resolve(t)
	if legacy {
		lock["schema_version"] = 4
		delete(lock, "skill_integrity")
		delete(lock, "startup_verification_level")
		x.writeLock(t, lock)
		x.level(t, 1)
		x.resolve(t)
		if x.readLock(t)["schema_version"] != float64(4) {
			t.Fatal("startup silently migrated the legacy lock")
		}
	}
	// The production entry inherits process cwd/env for Core resolution.
	// Redirect those actual inputs as well as the later launch dependencies.
	for _, assignment := range x.env {
		key, value, _ := strings.Cut(assignment, "=")
		t.Setenv(key, value)
	}
	t.Chdir(f.dir)
	f.deps.resolver = fragment.NewWithRunner(core, fragment.ExecRunner{})
	f.deps.environ = func() []string { return append([]string(nil), x.env...) }
	f.deps.readSelectedConfig = func(ctx context.Context) (launcherconfig.Selected, error) {
		return launcherconfig.ReadSelected(ctx, f.dir, x.env)
	}
	f.args = []string{"codex_cli", "--native", "--profile", "startup-fixture", "--model", "gpt-6-astra", "--effort", "medium", "--", "fixture prompt"}
	return x
}

func (x *startupCoreFixture) command(t *testing.T, binary string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir, cmd.Env = x.entry.dir, x.env
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture child pid=%d tool=%s argv=%q", cmd.Process.Pid, filepath.Base(binary), args)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("fixture command: %v\nstdout=%s\nstderr=%s", err, out.Bytes(), stderr.Bytes())
	}
	return out.Bytes()
}

func (x *startupCoreFixture) gitRepo(t *testing.T, dir string) string {
	t.Helper()
	x.command(t, "git", "-C", dir, "init", "--quiet")
	x.command(t, "git", "-C", dir, "add", "--all")
	// Synthetic Git objects are fixture inputs, never delivery commits.
	x.command(t, "git", "-C", dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "gpg.format=ssh", "-c", "user.signingkey="+x.signer, "commit", "-S", "--quiet", "-m", "Fixture")
	return strings.TrimSpace(string(x.command(t, "git", "-C", dir, "rev-parse", "HEAD")))
}

func (x *startupCoreFixture) level(t *testing.T, level int) {
	t.Helper()
	doc := map[string]any{"schema_version": 11, "security_posture": "hardened", "allowed_sources": []string{"example.invalid/fixture"}, "skills_root": filepath.Join(x.entry.dir, "skills"), "projects": map[string]any{}, "startup_verification": map[string]int{"level": level}}
	doc["environments"] = map[string]any{"mcp_package_allowlist": []string{"example.invalid/fixture/mcp"}, "source_signers": map[string]any{
		"example.invalid/fixture/context": []any{map[string]string{"type": "ssh", "key": x.publicKey}},
		"example.invalid/fixture/skill":   []any{map[string]string{"type": "ssh", "key": x.publicKey}},
		"example.invalid/fixture/mcp":     []any{map[string]string{"type": "ssh", "key": x.publicKey}},
	}}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, x.config, data, 0600)
}

func (x *startupCoreFixture) readLock(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(x.lock)
	if err != nil {
		t.Fatal(err)
	}
	var lock map[string]any
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	return lock
}

func (x *startupCoreFixture) writeLock(t *testing.T, lock map[string]any) {
	t.Helper()
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, x.lock, data, 0600)
}

func (x *startupCoreFixture) resolve(t *testing.T) {
	t.Helper()
	raw := x.command(t, x.core, "env", "resolve", "codex_cli", "--profile", "startup-fixture", "--repair", "--format", "json")
	f, err := fragment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	x.entry.home = f.Env[fragment.HomeVariable("codex_cli")]
}

// The intact and mismatching cases independently ground all four policy
// outcomes in actual Core stdout/stderr and the production launcher path.
// A successful resolution alone is insufficient: the inert child must start;
// refusal must expose no fragment/child and preserve lock, source and marker.
// Strict snapshot repair may restore the same pinned bytes; it must not replace
// contradictory frozen integrity evidence or manufacture legacy evidence.
func TestStartupCoreDocking(t *testing.T) {
	core := os.Getenv("CURATOR_RUN_CORE_TEST_BINARY")
	if core == "" {
		t.Skip("set CURATOR_RUN_CORE_TEST_BINARY to a separately qualified Core executable")
	}
	if !filepath.IsAbs(core) {
		t.Fatal("CURATOR_RUN_CORE_TEST_BINARY must be absolute")
	}
	for _, shape := range []string{"complete", "mismatch", "contradictory-lock", "legacy-native", "legacy-manifestless", "unsafe", "weakening"} {
		levels := []int{1, 2, 3, 4}
		if shape == "weakening" {
			levels = []int{3}
		}
		for _, level := range levels {
			t.Run(fmt.Sprintf("%s/level%d", shape, level), func(t *testing.T) {
				x := newStartupCoreFixture(t, core, shape == "legacy-native", strings.HasPrefix(shape, "legacy-"), false)
				x.level(t, level)
				allow := shape == "complete" || shape == "legacy-native" || shape == "mismatch" && level != 2 || shape == "legacy-manifestless" && level <= 2
				if shape == "mismatch" {
					writeFixture(t, x.skill, []byte("Contradicting frozen reference.\n"), 0600)
				}
				if shape == "contradictory-lock" {
					lock := x.readLock(t)
					evidence := lock["skill_integrity"].([]any)[0].(map[string]any)
					manifest := evidence["manifest"].(map[string]any)
					manifest["content_sha256"] = "sha256:" + strings.Repeat("0", 64)
					raw, err := json.Marshal(manifest)
					if err != nil {
						t.Fatal(err)
					}
					value, err := fragment.ParseJSON(raw)
					if err != nil {
						t.Fatal(err)
					}
					evidence["manifest_sha256"] = fmt.Sprintf("sha256:%x", sha256.Sum256(fragment.Canonical(value)))
					x.writeLock(t, lock)
					// An existing home must not be repaired from contradictory evidence.
				}
				if shape == "unsafe" {
					if err := os.Symlink("README.md", filepath.Join(filepath.Dir(x.skill), "forbidden-link")); err != nil {
						t.Fatal(err)
					}
				}
				if shape == "weakening" {
					// All levels share a machine baseline3; floor2 is forbidden.
					x.level(t, 3)
					lock := x.readLock(t)
					lock["startup_verification_level"] = 2
					x.writeLock(t, lock)
				}
				paths := []string{x.lock, x.skill, filepath.Join(x.entry.home, ".agent-environment.json")}
				before := map[string][]byte{}
				modes := map[string]os.FileMode{}
				for _, path := range paths {
					data, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					before[path] = data
					info, err := os.Stat(path)
					if err != nil {
						t.Fatal(err)
					}
					modes[path] = info.Mode()
				}
				code, out, stderr := x.entry.run()
				if allow {
					if code != 0 || len(out) == 0 || x.entry.builds != 1 {
						t.Fatalf("expected inert child, code=%d stdout=%s stderr=%s", code, out, stderr)
					}
					if _, err := os.Stat(filepath.Join(x.entry.dir, "started")); err != nil {
						t.Fatalf("inert child absent: %v", err)
					}
					var child childCapture
					if err := json.Unmarshal(out, &child); err != nil || child.WorkDir != x.entry.dir {
						t.Fatalf("invalid inert-child capture: %v", err)
					}
				} else {
					if code != 1 || len(out) != 0 || !bytes.Contains(stderr, []byte("curator-run: resolve_")) {
						t.Fatalf("expected typed resolver refusal, code=%d stdout=%s stderr=%s", code, out, stderr)
					}
					x.entry.assertNoChild(t)
					if x.entry.builds != 0 {
						t.Fatal("Core refusal reached the launch builder")
					}
				}
				if (shape == "mismatch" && level <= 2 || shape == "contradictory-lock" || shape == "legacy-manifestless") && !bytes.Contains(stderr, []byte("startup_verification_")) {
					t.Fatalf("actual Core startup diagnostic not transported: %s", stderr)
				}
				if allow && level == 4 && !bytes.Contains(stderr, []byte("verification_centre_unavailable")) {
					t.Fatalf("local3 fallback warning not transported: %s", stderr)
				}
				if shape == "contradictory-lock" && level == 1 && !bytes.Contains(stderr, []byte("environment_home_stale: required home cannot be repaired from mismatching evidence")) {
					t.Fatalf("wrong mismatching-evidence home refusal: %s", stderr)
				}
				if shape == "weakening" && !bytes.Contains(stderr, []byte("startup_verification_weakening")) {
					t.Fatalf("wrong floor-weakening refusal: %s", stderr)
				}
				if shape == "unsafe" && (!bytes.Contains(stderr, []byte("environment_store_untrusted")) || !bytes.Contains(stderr, []byte("failed link_safety check:")) || !bytes.Contains(stderr, []byte("forbidden-link"))) {
					t.Fatalf("wrong unsafe-boundary refusal: %s", stderr)
				}
				if (shape == "complete" || shape == "legacy-native") && level < 4 && bytes.Contains(stderr, []byte("startup_verification_")) {
					t.Fatalf("intact source gained a startup warning: %s", stderr)
				}
				if shape == "unsafe" {
					got, err := os.Readlink(filepath.Join(filepath.Dir(x.skill), "forbidden-link"))
					if err != nil || got != "README.md" {
						t.Fatalf("refused startup changed unsafe fixture: %v", err)
					}
				}
				for path, want := range before {
					if shape == "mismatch" && level >= 3 && path == x.skill {
						want = []byte("Frozen reference.\n")
					}
					got, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(want, got) {
						t.Fatalf("startup changed fixture evidence %s: %v", filepath.Base(path), err)
					}
					info, err := os.Stat(path)
					if err != nil || (shape != "mismatch" || level < 3 || path != x.skill) && info.Mode() != modes[path] {
						t.Fatalf("startup changed fixture permissions %s: %v", filepath.Base(path), err)
					}
				}
			})
		}
	}
}

// For context and MCP snapshots, an intact positive starts the inert child at
// each level. An outside-snapshot hardlink then has to refuse before the actual
// launcher builder/child, preserving the full fixture state and both identities.
// This Unix docking witness does not qualify Windows syscalls or a live runtime.
func TestStartupCoreHardlinkDocking(t *testing.T) {
	core := os.Getenv("CURATOR_RUN_CORE_TEST_BINARY")
	if core == "" {
		t.Skip("set CURATOR_RUN_CORE_TEST_BINARY to a separately qualified Core executable")
	}
	if !filepath.IsAbs(core) {
		t.Fatal("CURATOR_RUN_CORE_TEST_BINARY must be absolute")
	}
	for _, kind := range []string{"context", "mcp"} {
		for level := 1; level <= 4; level++ {
			t.Run(fmt.Sprintf("%s/level%d", kind, level), func(t *testing.T) {
				x := newStartupCoreFixture(t, core, false, false, kind == "mcp")
				x.level(t, level)
				state := filepath.Dir(x.config)
				intact := startupCoreStateDigest(t, state)
				code, out, stderr := x.entry.run()
				if code != 0 || len(out) == 0 || x.entry.builds != 1 {
					t.Fatalf("intact positive did not start child: code=%d stderr=%s", code, stderr)
				}
				started := filepath.Join(x.entry.dir, "started")
				if _, err := os.Stat(started); err != nil {
					t.Fatalf("positive child absent: %v", err)
				}
				if level == 4 && !bytes.Contains(stderr, []byte("verification_centre_unavailable")) {
					t.Fatalf("positive fallback warning absent: %s", stderr)
				}
				if got := startupCoreStateDigest(t, state); got != intact {
					t.Fatal("intact positive changed fixture state")
				}
				t.Log("intact positive started inert child and preserved fixture state")
				if err := os.Remove(started); err != nil {
					t.Fatal(err)
				}
				x.entry.builds = 0
				payload := x.context
				if kind == "mcp" {
					payload = x.mcp
				}
				plant := filepath.Join(x.entry.dir, "outside-snapshot-hardlink")
				if err := os.Link(payload, plant); err != nil {
					t.Fatal(err)
				}
				before, err := os.Lstat(payload)
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(payload)
				if err != nil {
					t.Fatal(err)
				}
				planted := startupCoreStateDigest(t, state)
				code, out, stderr = x.entry.run()
				if code != 1 || len(out) != 0 || x.entry.builds != 0 ||
					!bytes.Contains(stderr, []byte("curator-run: resolve_")) ||
					!bytes.Contains(stderr, []byte("environment_store_untrusted")) ||
					!bytes.Contains(stderr, []byte("failed single_link check:")) ||
					!bytes.Contains(stderr, []byte(payload)) {
					t.Errorf("expected named single_link refusal before builder/child: code=%d stdout=%s stderr=%s", code, out, stderr)
				}
				x.entry.assertNoChild(t)
				if got := startupCoreStateDigest(t, state); got != planted {
					t.Error("hardlink refusal changed store, lock, home, marker or other fixture state")
				}
				for _, path := range []string{payload, plant} {
					after, err := os.Lstat(path)
					if err != nil || !os.SameFile(before, after) || after.Mode() != before.Mode() {
						t.Errorf("hardlink identity or permissions changed: %v", err)
					}
					got, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(want, got) {
						t.Errorf("hardlink bytes changed: %v", err)
					}
				}
			})
		}
	}
}

func startupCoreStateDigest(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		var data []byte
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			data = []byte(link)
		case info.Mode().IsRegular():
			data, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		case info.IsDir():
		default:
			return fmt.Errorf("unexpected fixture file type at %s", relative)
		}
		_, _ = fmt.Fprintf(h, "%d:%s:%d:%d:", len(relative), relative, info.Mode(), len(data))
		_, _ = h.Write(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
