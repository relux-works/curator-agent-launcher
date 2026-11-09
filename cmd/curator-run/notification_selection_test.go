//go:build darwin || linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNotificationMember(t *testing.T, f *frontFixture) {
	t.Helper()
	r := expectedRecord(f.root)
	r.Schema = "werkstatt-record-v1"
	r.ID = "example"
	r.Project = "sample"
	r.Role = "developer"
	r.Identity = "example.dev"
	r.Room = "#example"
	r.Orchestrator = "example.orch"
	r.Sandbox = "none"
	r.TrustClass = "standard"
	if err := os.MkdirAll(filepath.Join(f.config, "werkstatt"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(f.config, "werkstatt", "example.json"), frontJSON(r), 0600)
}

// Actual selected Host metadata cannot fall through to native execution or an
// unselected manager. Real launcher binaries must refuse without a manager
// call, native child or Werkstatt write; absent/off controls still run natively.
func TestSelectedNotificationRouteRefusalBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, tc := range []struct {
		name, mode, registry, backend, diagnostic string
		args                                      []string
		code                                      int
		native                                    bool
	}{
		{"native-flag", "auto", "invalid", "host", "selected Host notification requires hosted execution", []string{"--native"}, 1, false},
		{"untracked-flag", "auto", "invalid", "host", "selected Host notification requires hosted execution", []string{"--untracked"}, 1, false},
		{"native-mode", "native", "invalid", "host", "selected Host notification requires hosted execution", nil, 1, false},
		{"resume-native", "auto", "invalid", "host", "selected Host notification requires hosted execution", []string{"--native", "resume"}, 1, false},
		{"empty-manager", "auto", "empty", "host", "session_host_missing", []string{"--detach"}, 1, false},
		{"unselected-manager", "auto", "unselected", "host", "session_host_missing", []string{"--detach"}, 1, false},
		{"absent-control", "auto", "invalid", "", "", []string{"--native"}, 0, true},
		{"off-control", "auto", "invalid", "off", "", []string{"--native"}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(t, bin)
			f.policy(tc.mode, false)
			f.registry(tc.registry)
			extensions := map[string]any{}
			if tc.backend != "" {
				n := map[string]any{"backend": tc.backend}
				if tc.backend == "host" {
					n["contract"] = "curator-host-notification/1"
					n["workflow_reference"] = "work-1"
					n["workflow_sha256"] = strings.Repeat("a", 64)
				}
				extensions["notification"] = n
			}
			writeFixture(t, filepath.Join(f.root, "selected-config.json"), frontJSON(map[string]any{
				"schema": "curator.launcher-config/2", "config_schema": 8,
				"config_path": "/fixture/selected.json", "extensions": extensions,
			}), 0600)
			args := append([]string{}, tc.args...)
			if tc.backend == "host" && (tc.name == "empty-manager" || tc.name == "unselected-manager") {
				writeNotificationMember(t, f)
				args = append(args, "--member", "example")
			}
			code, stdout, stderr := f.invoke(false, false, args...)
			f.require(t, code, tc.code, stderr, tc.diagnostic, nil, 0, tc.native)
			if !tc.native && stdout != "" {
				t.Fatal("refused notification launch published output")
			}
		})
	}
}

// The real selected projection must reach the capability gate before any lease
// or native/hosted child starts. Absent/off controls traverse the same legacy
// manager route; the fixture's later incarnation refusal is not Host admission.
func TestSelectedNotificationCapabilityBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, backend := range []string{"host", "off", ""} {
		name := backend
		if name == "" {
			name = "absent"
		}
		t.Run(name, func(t *testing.T) {
			f := newFront(t, bin)
			extensions := map[string]any{}
			if backend != "" {
				selection := map[string]any{"backend": backend}
				if backend == "host" {
					selection["contract"] = "curator-host-notification/1"
					selection["workflow_reference"] = "work-1"
					selection["workflow_sha256"] = strings.Repeat("a", 64)
				}
				extensions["notification"] = selection
			}
			writeFixture(t, filepath.Join(f.root, "selected-config.json"), frontJSON(map[string]any{
				"schema": "curator.launcher-config/2", "config_schema": 8,
				"config_path": "/fixture/selected.json", "extensions": extensions,
			}), 0600)
			args := []string{"--detach"}
			if backend == "host" {
				writeNotificationMember(t, f)
				args = append(args, "--member", "example")
			}
			code, _, stderr := f.invoke(false, false, args...)
			if backend == "host" {
				f.require(t, code, 6, stderr, "capability=curator-host-notification/1", []string{"describe"}, 0, false)
				raw, err := os.ReadFile(filepath.Join(f.root, "lease-trace.jsonl"))
				if err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if len(raw) != 0 {
					t.Fatalf("capability refusal touched home lease: %s", raw)
				}
			} else {
				f.require(t, code, 1, stderr, "process_incarnation_unavailable", []string{"describe", "launch"}, 1, false)
			}
		})
	}
}

// The real station entry prepares Host intent and requires its capability
// before any launch/lease. This manager intentionally cannot execute Host.
func TestNotificationStationCapabilityBinary(t *testing.T) {
	bin := frontBuild(t)
	w := newWS(t, bin)
	f := w.frontFixture
	n := selectedNotification()
	writeFixture(t, filepath.Join(f.root, "selected-config.json"), frontJSON(map[string]any{"schema": "curator.launcher-config/2", "config_schema": 8, "config_path": "/fixture/selected.json", "extensions": map[string]any{"notification": map[string]any{"backend": n.Backend, "contract": n.Contract, "workflow_reference": n.WorkflowReference, "workflow_sha256": n.WorkflowSHA256}}}), 0600)
	w.record["notification"] = expectedRecord(f.root).Notification
	w.record["profile_lock_sha256"] = strings.Repeat("1", 64)
	w.write()
	code, _, stderr := w.invokeWS("up")
	if code != 6 || !strings.Contains(stderr, "capability=curator-host-notification/1") {
		t.Fatalf("station omitted Host request: %d %s", code, stderr)
	}
	if o := w.observe(); o.Starts != 0 {
		t.Fatal("missing capability started fixture")
	}
	raw, err := os.ReadFile(filepath.Join(f.root, "lease-trace.jsonl"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(raw) != 0 {
		t.Fatal("missing capability touched home lease")
	}
}

// Incomplete Host member selection refuses before registry/RPC/lease/native
// effects, even if the registry is invalid. Disabled controls above stay valid.
func TestNotificationMissingMemberBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, id := range []string{"", "missing"} {
		t.Run("member-"+id, func(t *testing.T) {
			f := newFront(t, bin)
			f.registry("invalid")
			n := selectedNotification()
			writeFixture(t, filepath.Join(f.root, "selected-config.json"), frontJSON(map[string]any{"schema": "curator.launcher-config/2", "config_schema": 8, "config_path": "/fixture/selected.json", "extensions": map[string]any{"notification": map[string]any{"backend": n.Backend, "contract": n.Contract, "workflow_reference": n.WorkflowReference, "workflow_sha256": n.WorkflowSHA256}}}), 0600)
			args := []string{"--detach"}
			diagnostic := "requires explicit --member"
			if id != "" {
				args = append(args, "--member", id)
				diagnostic = "werkstatt_not_found"
			}
			code, _, stderr := f.invoke(false, false, args...)
			f.require(t, code, 1, stderr, diagnostic, nil, 0, false)
			raw, err := os.ReadFile(filepath.Join(f.root, "lease-trace.jsonl"))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if len(raw) != 0 {
				t.Fatal("missing selection touched lease")
			}
		})
	}
}
