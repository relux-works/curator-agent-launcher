// Profile-bound transcript conversion is an external CLI operation. Only the
// verified target UUID crosses back into the normal native launch pipeline.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/doorbell"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

type conversionSource struct {
	Schema        int
	Profile, Lock string
	Path          string `json:"source_log"`
	executable    string
}

func conversionProvider(id string) string {
	switch id {
	case fragment.EnvClaudeCode:
		return "claude"
	case fragment.EnvCodexCLI:
		return "codex"
	default:
		return id
	}
}

func conversionCommand(ctx context.Context, binary string, stderr io.Writer, result any, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("agent-session-convert %s: %w", args[0], err)
	}
	return json.Unmarshal(out, result)
}

func locateConversion(ctx context.Context, inv cli.Invocation, stderr io.Writer) (*conversionSource, error) {
	binary, err := exec.LookPath("agent-session-convert")
	wd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return nil, cwdErr
	}
	real, pathErr := filepath.EvalSymlinks(binary)
	physicalWD, wdErr := filepath.EvalSymlinks(wd)
	if err != nil || pathErr != nil || wdErr != nil || !filepath.IsAbs(binary) ||
		insideCheckout(wd, binary) || insideCheckout(physicalWD, real) {
		return nil, errors.New("install the agent-session-transfer command on the operator PATH")
	}
	args := []string{"locate", "--from", conversionProvider(inv.From), "--session-id", inv.SourceID}
	if inv.ProfileSet {
		args = append(args, "--source-profile", inv.Profile)
	}
	var s conversionSource
	if err := conversionCommand(ctx, real, stderr, &s, args...); err != nil {
		return nil, err
	}
	if s.Schema != 1 || !filepath.IsAbs(s.Path) || s.Profile == "" || len(s.Lock) != 64 || (inv.ProfileSet && inv.Profile != s.Profile) {
		return nil, errors.New("invalid converter source binding")
	}
	s.executable = real
	return &s, nil
}

var convertedUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func publishConversion(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, wd string, src *conversionSource, stderr io.Writer) (string, error) {
	wd, err := filepath.EvalSymlinks(wd)
	if err != nil {
		return "", err
	}
	state, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	parent := filepath.Join(state, "swarma", "session-conversions")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	key, _ := json.Marshal([]string{inv.From, inv.SourceID, inv.EnvID, src.Profile, src.Lock, wd, frag.Home()})
	bundle := filepath.Join(parent, fmt.Sprintf("%x", sha256.Sum256(key)))
	if _, err := os.Lstat(bundle); os.IsNotExist(err) && inv.A2A != "" {
		if err := requireReleasedIdentity(ctx, inv.A2A, wd, stderr); err != nil {
			return "", err
		}
	}
	// The converter claims the bundle with an exclusive mkdir. On retry it
	// can return the identical completed session or refuse an uncertain write.
	fmt.Fprintf(stderr, "curator-run: conversion bundle: %s\n", bundle)
	provider := conversionProvider(inv.EnvID)
	var r struct {
		State, Workspace string
		ID               string `json:"target_id"`
		Log              string `json:"target_log"`
		Provider         string `json:"target_provider"`
		Destination      struct{ Kind, Profile, Lock, Home string }
	}
	err = conversionCommand(ctx, src.executable, stderr, &r, "convert", "--from", conversionProvider(inv.From),
		"--to", provider, "--source-log", src.Path, "--expect-source-id", inv.SourceID,
		"--target-profile", src.Profile, "--expect-lock", src.Lock, "--expect-home", frag.Home(),
		"--workspace", wd, "--out-dir", bundle, "--apply", "--reuse-published")
	if err != nil {
		return "", err
	}
	d := r.Destination
	if r.State != "published" || r.Workspace != wd || !convertedUUID.MatchString(r.ID) || r.Provider != provider ||
		d.Kind != "curator" || d.Profile != src.Profile || d.Lock != src.Lock || d.Home != frag.Home() {
		return "", errors.New("converter receipt does not match resolved launch")
	}
	path, err := filepath.EvalSymlinks(r.Log)
	store := map[string]string{"codex": "sessions", "muse": "data/muse/sessions", "claude": "projects"}[provider]
	rel, relErr := filepath.Rel(filepath.Join(frag.Home(), store), path)
	if err != nil || relErr != nil || !filepath.IsLocal(rel) || rel == "." || path != r.Log ||
		!strings.Contains(filepath.ToSlash(rel), r.ID) {
		return "", errors.New("converter returned a session outside the managed store or with a different identity")
	}
	fmt.Fprintf(stderr, "curator-run: converted session: %s\n", r.ID)
	return r.ID, nil
}

func insideCheckout(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}

func providerResume(env, id string) []string {
	if env == fragment.EnvClaudeCode {
		return []string{"--resume", id}
	}
	return []string{"resume", id}
}

// A live or indeterminate old notification owner blocks a new publication.
// SessionStart still performs the authoritative lease claim inside the pane.
func requireReleasedIdentity(ctx context.Context, id, cwd string, stderr io.Writer) error {
	installer := doorbell.Installer{Environment: os.Environ(), InvocationDirectory: cwd, Diagnostics: stderr}
	exe, err := installer.Resolve(cwd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	native, err := exec.CommandContext(ctx, exe, "--as", id, "runtime", "status").Output()
	if err != nil || strings.TrimSpace(string(native)) != id+" unavailable: unbound" {
		return errors.New("a2a native binding active or unknown; reconcile the source before conversion")
	}
	terminal, err := exec.CommandContext(ctx, exe, "--as", id, "bind", "status").Output()
	if err == nil {
		for _, line := range strings.Split(string(terminal), "\n") {
			if strings.HasPrefix(line, id+" unbound;") {
				return nil
			}
			if strings.HasPrefix(line, id+" generation=") && strings.Contains(line, " stopped=true outcome=") {
				return nil
			}
		}
	}
	return errors.New("a2a terminal binding active or unknown; reconcile the source before conversion")
}
