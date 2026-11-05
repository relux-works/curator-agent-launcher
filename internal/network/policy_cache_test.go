package network

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These verdicts follow the closed five-path contract, not native parser
// equivalence. Native acceptance alone never establishes gateway safety.
func TestPolicyCacheClosedSubset(t *testing.T) {
	for _, tc := range []struct {
		name, doc, path string
		remote          bool
	}{
		{"remote empty", ` { } `, "", true},
		{"remote settings", `{"env":{"ANTHROPIC_BASE_URL":"SECRET"}}`, "$.*", true},
		{"minimal policy", `{"restrictions":{}}`, "", false},
		{"empty policy lacks native required restrictions", `{}`, "restrictions", false},
		{"projected shape permissive connector", `{"restrictions":{"allow_connector_file_downloads":{"allowed":true},"enforce_web_search_mcp_isolation":{"allowed":true}},"defaults":{"remote_control_at_startup":false},"compliance_taints":[],"monitoring_notice":null}`, "", false},
		{"connector narrowed", `{"restrictions":{"allow_connector_file_downloads":{"allowed":false}}}`, "", false},
		{"notice string", `{"restrictions":{},"monitoring_notice":"SECRET"}`, "", false},
		{"empty defaults", `{"restrictions":{},"defaults":{}}`, "", false},
		{"startup channel", `{"restrictions":{},"defaults":{"remote_control_at_startup":true}}`, "defaults.remote_control_at_startup", false},
		{"startup wrong type", `{"restrictions":{},"defaults":{"remote_control_at_startup":"false"}}`, "defaults.remote_control_at_startup", false},
		{"isolation off", `{"restrictions":{"enforce_web_search_mcp_isolation":{"allowed":false}}}`, "restrictions.enforce_web_search_mcp_isolation.allowed", false},
		{"isolation wrong type", `{"restrictions":{"enforce_web_search_mcp_isolation":{"allowed":1}}}`, "restrictions.enforce_web_search_mcp_isolation.allowed", false},
		{"connector wrong type", `{"restrictions":{"allow_connector_file_downloads":{"allowed":"true"}}}`, "restrictions.allow_connector_file_downloads.allowed", false},
		{"restriction extra member", `{"restrictions":{"allow_connector_file_downloads":{"allowed":true,"SECRET":"SECRET"}}}`, "restrictions.allow_connector_file_downloads.allowed", false},
		{"restriction missing allowed", `{"restrictions":{"allow_connector_file_downloads":{}}}`, "restrictions.allow_connector_file_downloads.allowed", false},
		{"restriction nonobject", `{"restrictions":[]}`, "restrictions", false},
		{"unknown restriction", `{"restrictions":{"allow_SECRET":{"allowed":true}}}`, "restrictions.*", false},
		{"prefix lookalike", `{"restrictions":{"enforce_SECRET":{"allowed":true}}}`, "restrictions.*", false},
		{"unknown defaults", `{"restrictions":{},"defaults":{"SECRET":false}}`, "defaults.*", false},
		{"defaults null", `{"restrictions":{},"defaults":null}`, "defaults", false},
		{"taint not empty", `{"restrictions":{},"compliance_taints":["hipaa"]}`, "compliance_taints", false},
		{"taint wrong type", `{"restrictions":{},"compliance_taints":null}`, "compliance_taints", false},
		{"native notice object outside admitted subset", `{"restrictions":{},"monitoring_notice":{"text":"SECRET"}}`, "monitoring_notice", false},
		{"notice wrong type", `{"restrictions":{},"monitoring_notice":false}`, "monitoring_notice", false},
		{"core skills outside subset", `{"restrictions":{},"core_skills":[]}`, "$.*", false},
		{"unknown root", `{"restrictions":{},"SECRET":"SECRET"}`, "$.*", false},
		{"duplicate root", `{"restrictions":{},"restrictions":{}}`, "$", false},
		{"escaped duplicate", `{"restrictions":{},"\u0072estrictions":{}}`, "$", false},
		{"nested duplicate", `{"restrictions":{"allow_connector_file_downloads":{"allowed":false,"allowed":true}}}`, "$", false},
		{"trailing document", `{"restrictions":{}} {}`, "$", false},
		{"malformed", `{SECRET`, "$", false},
		{"null", `null`, "$", false},
		{"array", `[]`, "$", false},
		{"bad unicode", `{"restrictions":{},"monitoring_notice":"\ud800"}`, "$", false},
		{"bad UTF8", string([]byte{'{', '"', 0xff, '"', ':', '0', '}'}), "$", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := policyCachePath([]byte(tc.doc), tc.remote); got != tc.path {
				t.Fatalf("path = %q, want %q", got, tc.path)
			}
		})
	}
}

// The cache reader refuses special files and links before opening them,
// catches a regular-file/symlink replacement at open, and preserves bytes.
func TestPolicyCacheNoFollowReader(t *testing.T) {
	for _, kind := range []string{"regular", "missing", "symlink", "dangling", "directory", "fifo", "unreadable", "oversize", "replaced-file", "replaced-link", "rewrite-on-read", "replace-on-read"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cache")
			write := func(p, doc string) {
				t.Helper()
				if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			write(path, "{}")
			original := openPolicyCache
			defer func() { openPolicyCache = original }()
			originalRead := readSettingsChunk
			defer func() { readSettingsChunk = originalRead }()
			opened := false
			openPolicyCache = func(p string) (*os.File, error) {
				opened = true
				if kind == "replaced-file" || kind == "replaced-link" {
					target := p + ".new"
					write(target, "{}")
					if err := os.Remove(p); err != nil {
						t.Fatal(err)
					}
					if kind == "replaced-link" {
						if err := os.Symlink(target, p); err != nil {
							t.Fatal(err)
						}
					} else if err := os.Rename(target, p); err != nil {
						t.Fatal(err)
					}
				}
				f, err := original(p)
				if kind == "replaced-link" && !errors.Is(err, syscall.ELOOP) {
					if f != nil {
						f.Close()
					}
					t.Fatalf("link replacement did not receive the kernel no-follow refusal: %v", err)
				}
				return f, err
			}
			if kind == "rewrite-on-read" || kind == "replace-on-read" {
				once := false
				readSettingsChunk = func(f *os.File, b []byte) (int, error) {
					n, err := originalRead(f, b)
					if !once {
						once = true
						if kind == "replace-on-read" {
							write(path+".new", "{}")
							if err := os.Rename(path+".new", path); err != nil {
								t.Fatal(err)
							}
						} else {
							write(path, "{ }")
						}
					}
					return n, err
				}
			}
			switch kind {
			case "missing":
				os.Remove(path)
			case "symlink", "dangling":
				os.Remove(path)
				target := path + ".target"
				if kind == "symlink" {
					write(target, "{}")
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				os.Remove(path)
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				os.Remove(path)
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.Truncate(path, maxSettingsBytes+1); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readPolicyCache(newGuardClock(context.Background(), time.Minute), "test", path)
			if kind == "regular" {
				if err != nil || string(got) != "{}" || !opened {
					t.Fatalf("control: %q %v opened=%v", got, err, opened)
				}
			} else if kind == "missing" {
				if !errors.Is(err, errAbsent) || opened {
					t.Fatalf("missing: %v opened=%v", err, opened)
				}
			} else if err == nil || (opened && !strings.HasPrefix(kind, "replaced-") && !strings.HasSuffix(kind, "-on-read")) {
				t.Fatalf("unsafe file: %v opened=%v", err, opened)
			}
		})
	}
}

// Sidecars may be present without policy bodies. They are metadata-only:
// arbitrary content is never read, trusted, printed or used as policy.
func TestPolicyCacheSidecarsMetadataOnly(t *testing.T) {
	for _, file := range []string{
		"policy-limits.json.stamp.json", "policy-limits.json.signature.json", "policy-limits.json.signature-iat.json",
		"remote-settings.json.signature.json", "remote-settings.json.signature-iat.json", "remote-settings.json.synced-at",
	} {
		t.Run(file, func(t *testing.T) {
			for _, kind := range []string{"regular", "symlink", "dangling", "directory", "fifo", "oversize"} {
				t.Run(kind, func(t *testing.T) {
					home := t.TempDir()
					path := filepath.Join(home, file)
					if err := os.WriteFile(path, []byte("SECRET"), 0o600); err != nil {
						t.Fatal(err)
					}
					switch kind {
					case "symlink", "dangling":
						os.Rename(path, path+".target")
						target := path + ".target"
						if kind == "dangling" {
							target += ".absent"
						}
						if err := os.Symlink(target, path); err != nil {
							t.Fatal(err)
						}
					case "directory":
						os.Remove(path)
						if err := os.Mkdir(path, 0o700); err != nil {
							t.Fatal(err)
						}
					case "fifo":
						os.Remove(path)
						if err := syscall.Mkfifo(path, 0o600); err != nil {
							t.Fatal(err)
						}
					case "oversize":
						if err := os.Truncate(path, maxPolicySidecarBytes+1); err != nil {
							t.Fatal(err)
						}
					}
					original := openPolicyCache
					defer func() { openPolicyCache = original }()
					openPolicyCache = func(string) (*os.File, error) { t.Fatal("sidecar opened"); return nil, nil }
					err := checkPolicyCaches(newGuardClock(context.Background(), time.Minute), "test", home)
					if (err == nil) != (kind == "regular") || (err != nil && strings.Contains(err.Error(), "SECRET")) {
						t.Fatalf("sidecar: %v", err)
					}
				})
			}
		})
	}
}

// Cache reads share the settings guard's interruption/deadline contract;
// cancellation during an open cannot reach parsing or gateway effects.
func TestPolicyCacheStopsOnCancelAndBudget(t *testing.T) {
	for _, kind := range []string{"live", "cancel-before", "cancel-open", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, "remote-settings.json"), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			clock := newGuardClock(ctx, time.Minute)
			if kind == "cancel-before" {
				cancel()
			}
			if kind == "deadline" {
				clock.deadline = time.Now().Add(-time.Second)
			}
			original := openPolicyCache
			defer func() { openPolicyCache = original }()
			if kind == "cancel-open" {
				openPolicyCache = func(path string) (*os.File, error) { cancel(); return original(path) }
			}
			err := checkPolicyCaches(clock, "test", home)
			if kind == "live" {
				if err != nil {
					t.Fatalf("live control: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), map[bool]string{true: "time budget", false: "interrupted"}[kind == "deadline"]) {
				t.Fatalf("stop: %v", err)
			}
		})
	}
}
