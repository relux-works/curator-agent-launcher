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
		{"isolation off in gateway", `{"restrictions":{"enforce_web_search_mcp_isolation":{"allowed":false}}}`, "", false},
		{"isolation wrong type", `{"restrictions":{"enforce_web_search_mcp_isolation":{"allowed":1}}}`, "restrictions.enforce_web_search_mcp_isolation.allowed", false},
		{"connector wrong type", `{"restrictions":{"allow_connector_file_downloads":{"allowed":"true"}}}`, "restrictions.allow_connector_file_downloads.allowed", false},
		{"restriction extra member", `{"restrictions":{"allow_connector_file_downloads":{"allowed":true,"SECRET":"SECRET"}}}`, "restrictions.allow_connector_file_downloads.*", false},
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

// Diagnostics enumerate independent schema failures without private content.
func TestPolicyCacheCompleteDiagnostics(t *testing.T) {
	// Independent violations all appear; supplied names and values never do.
	unsafe := `{"PRIVATE_ROOT":"PRIVATE_VALUE","restrictions":{"PRIVATE_KEY":{},"allow_connector_file_downloads":{"allowed":"PRIVATE_VALUE","PRIVATE_EXTRA":0},"enforce_web_search_mcp_isolation":{"allowed":1}},"defaults":{"remote_control_at_startup":true,"PRIVATE_DEFAULT":0},"compliance_taints":["PRIVATE_VALUE"],"monitoring_notice":false}`
	want := []string{"$.*", "restrictions.*", "restrictions.allow_connector_file_downloads.allowed", "restrictions.allow_connector_file_downloads.*", "restrictions.enforce_web_search_mcp_isolation.allowed", "defaults.remote_control_at_startup", "defaults.*", "compliance_taints", "monitoring_notice"}
	got := policyCachePath([]byte(unsafe), false)
	for _, path := range want {
		if !strings.Contains(got, path) {
			t.Errorf("missing diagnostic %q in %q", path, got)
		}
	}
	if strings.Contains(got, "PRIVATE_") {
		t.Fatalf("private member or value disclosed: %q", got)
	}
	if got := policyCachePath([]byte(`{"defaults":{"remote_control_at_startup":true},"monitoring_notice":false}`), false); !strings.Contains(got, "restrictions") || !strings.Contains(got, "defaults.remote_control_at_startup") || !strings.Contains(got, "monitoring_notice") {
		t.Fatalf("missing required field hid neighboring violations: %q", got)
	}
	// The independently grounded gateway subset admits either isolation bool.
	for _, doc := range []string{`{"restrictions":{}}`, `{"restrictions":{"enforce_web_search_mcp_isolation":{"allowed":false}}}`, `{"restrictions":{"enforce_web_search_mcp_isolation":{"allowed":true}}}`} {
		if got := policyCachePath([]byte(doc), false); got != "" {
			t.Errorf("valid control refused: %q", got)
		}
	}
}

// policyWithRestrictions renders a policy-limits document in the shape the
// server emits: the given restriction entries (raw JSON values by name, in the
// given order) beside the admitted defaults, taints and notice.
func policyWithRestrictions(entries ...[2]string) string {
	var members []string
	for _, entry := range entries {
		members = append(members, `"`+entry[0]+`":`+entry[1])
	}
	return `{"restrictions":{` + strings.Join(members, ",") + `},"defaults":{"remote_control_at_startup":false},"compliance_taints":[],"monitoring_notice":null}`
}

// jsonEscapedAt returns name with the byte at index i written as a JSON
// unicode escape (a backslash, the letter u and four hex digits), so that the
// text differs from name and decodes to it.
func jsonEscapedAt(name string, i int) string {
	const hex = "0123456789abcdef"
	c := name[i]
	return name[:i] + string(rune(0x5c)) + "u00" + string(hex[c>>4]) + string(hex[c&0xf]) + name[i+1:]
}

// Claim: the restriction-name allowlist is exact and closed. The four
// admitted names pass with either boolean, alone and together in the shape
// the server emits; a native name without an artifact check, a near miss of
// an admitted name and a looser entry shape for a newly admitted name refuse
// with the paths the contract already defines, and a weakening value beside
// the admitted names still refuses. This pins the launcher's check only: that
// an admitted value is inert in the runtime rests on the qualification
// records, not on this test.
func TestPolicyCacheRestrictionNames(t *testing.T) {
	admitted := []string{"allow_connector_file_downloads", "allow_connector_file_uploads", "allow_computer_use", "enforce_web_search_mcp_isolation"}
	added := []string{"allow_connector_file_uploads", "allow_computer_use"}
	entry := func(allowed bool) string {
		return map[bool]string{false: `{"allowed":false}`, true: `{"allowed":true}`}[allowed]
	}
	all := func(allowed bool) [][2]string {
		var entries [][2]string
		for _, name := range admitted {
			entries = append(entries, [2]string{name, entry(allowed)})
		}
		return entries
	}
	check := func(t *testing.T, doc, want string) {
		t.Helper()
		got := policyCachePath([]byte(doc), false)
		if got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if strings.Contains(got, "PRIVATE_") {
			t.Fatalf("private member or value disclosed: %q", got)
		}
	}

	// Compatibility: every form the server emits or has emitted is admitted.
	t.Run("server shape/all true", func(t *testing.T) { check(t, policyWithRestrictions(all(true)...), "") })
	t.Run("server shape/all false", func(t *testing.T) { check(t, policyWithRestrictions(all(false)...), "") })
	t.Run("earlier two-name shape", func(t *testing.T) {
		check(t, policyWithRestrictions([2]string{"allow_connector_file_downloads", entry(true)}, [2]string{"enforce_web_search_mcp_isolation", entry(true)}), "")
	})
	for _, name := range admitted {
		for _, allowed := range []bool{false, true} {
			t.Run("alone/"+name+"/"+map[bool]string{false: "false", true: "true"}[allowed], func(t *testing.T) {
				check(t, policyWithRestrictions([2]string{name, entry(allowed)}), "")
				check(t, `{"restrictions":{"`+name+`":`+entry(allowed)+`}}`, "")
			})
		}
	}

	// A name is admitted by an artifact check, never by its spelling. The
	// first group are real native restriction names that have no check; the
	// second are near misses of the admitted names.
	for _, name := range []string{
		"allow_linked_computer", "allow_remote_control", "allow_remote_sessions", "allow_cobalt_plinth_alkanet",
		"allow_gmail_file_references", "allow_tool_search", "require_connector_block_on_plugin_servers",
		"allow_computer", "allow_computer_use_2", "allow_computer_use ", " allow_computer_use", "Allow_Computer_Use",
		"ALLOW_COMPUTER_USE", "allow-computer-use", "allow_connector_file_upload", "allow_connector_file_uploads_",
		"allow_connector_file_uploadS", "enforce_computer_use", "PRIVATE_NAME",
	} {
		t.Run("unlisted/"+name, func(t *testing.T) {
			check(t, policyWithRestrictions(append(all(true), [2]string{name, entry(true)})...), "restrictions.*")
			check(t, policyWithRestrictions(append(all(false), [2]string{name, entry(false)})...), "restrictions.*")
			check(t, policyWithRestrictions([2]string{name, entry(true)}), "restrictions.*")
		})
	}

	// A newly admitted name gets no looser entry than the existing two.
	for _, name := range added {
		for _, tc := range []struct{ shape, value, suffix string }{
			{"string", `{"allowed":"true"}`, ".allowed"},
			{"number", `{"allowed":1}`, ".allowed"},
			{"null allowed", `{"allowed":null}`, ".allowed"},
			{"nested allowed", `{"allowed":{"allowed":true}}`, ".allowed"},
			{"missing allowed", `{}`, ".allowed"},
			{"null entry", `null`, ".allowed"},
			{"bool entry", `true`, ".allowed"},
			{"array entry", `[{"allowed":true}]`, ".allowed"},
			{"extra member", `{"allowed":true,"PRIVATE_EXTRA":"PRIVATE_VALUE"}`, ".*"},
			{"case variant member", `{"Allowed":true}`, ".*, restrictions." + name + ".allowed"},
		} {
			t.Run("shape/"+name+"/"+tc.shape, func(t *testing.T) {
				want := "restrictions." + name + tc.suffix
				check(t, policyWithRestrictions([2]string{name, tc.value}), want)
				// The other admitted entries do not mask the malformed one.
				var entries [][2]string
				for _, other := range admitted {
					if other != name {
						entries = append(entries, [2]string{other, entry(true)})
					}
				}
				check(t, policyWithRestrictions(append(entries, [2]string{name, tc.value})...), want)
			})
		}
		// The strict parser refuses a repeated member before the allowlist
		// is consulted.
		for _, tc := range []struct{ kind, doc string }{
			{"member", `{"restrictions":{"` + name + `":{"allowed":false,"allowed":true}}}`},
			{"entry", `{"restrictions":{"` + name + `":{"allowed":false},"` + name + `":{"allowed":true}}}`},
		} {
			t.Run("duplicate/"+name+"/"+tc.kind, func(t *testing.T) { check(t, tc.doc, "$") })
		}
		// A key is compared after JSON escape decoding. The same name
		// spelled with one escaped byte is a repeat beside the plain key, in
		// either order, and alone it is the admitted name. The guard keeps
		// these rows honest: the fixture must differ from the plain name.
		for _, at := range []struct {
			label string
			index int
		}{{"first byte", 0}, {"last byte", len(name) - 1}} {
			escaped := jsonEscapedAt(name, at.index)
			if escaped == name || strings.Contains(escaped, name) || len(escaped) != len(name)+5 {
				t.Fatalf("fixture %q is not an escaped spelling of %q", escaped, name)
			}
			t.Run("duplicate/"+name+"/escaped entry/"+at.label, func(t *testing.T) {
				check(t, `{"restrictions":{"`+name+`":{"allowed":false},"`+escaped+`":{"allowed":true}}}`, "$")
				check(t, `{"restrictions":{"`+escaped+`":{"allowed":false},"`+name+`":{"allowed":true}}}`, "$")
			})
			t.Run("escaped spelling/"+name+"/"+at.label, func(t *testing.T) {
				check(t, `{"restrictions":{"`+escaped+`":{"allowed":true}}}`, "")
				check(t, `{"restrictions":{"`+escaped+`":{"allowed":"true"}}}`, "restrictions."+name+".allowed")
			})
		}
		// Decoding is exact: an escape that decodes to a near miss of the
		// name stays an unlisted name.
		last := len(name) - 1
		nearMiss := jsonEscapedAt(name[:last]+strings.ToUpper(name[last:]), last)
		t.Run("escaped near miss/"+name, func(t *testing.T) {
			check(t, `{"restrictions":{"`+nearMiss+`":{"allowed":true}}}`, "restrictions.*")
		})
	}
	// An escaped spelling does not admit a name that has no artifact check.
	t.Run("escaped unlisted name", func(t *testing.T) {
		check(t, `{"restrictions":{"`+jsonEscapedAt("allow_linked_computer", 0)+`":{"allowed":true}}}`, "restrictions.*")
	})

	// Values that could weaken a session stay refused beside the admitted
	// names, and independent violations are still all reported.
	server := policyWithRestrictions(all(true)...)
	for _, tc := range []struct{ name, old, replacement, want string }{
		{"startup remote control", `"remote_control_at_startup":false`, `"remote_control_at_startup":true`, "defaults.remote_control_at_startup"},
		{"another default", `"remote_control_at_startup":false`, `"remote_control_at_startup":false,"PRIVATE_DEFAULT":true`, "defaults.*"},
		{"taint", `"compliance_taints":[]`, `"compliance_taints":["hipaa"]`, "compliance_taints"},
		{"notice object", `"monitoring_notice":null`, `"monitoring_notice":{"text":"PRIVATE_VALUE"}`, "monitoring_notice"},
		{"core skills", `"monitoring_notice":null`, `"monitoring_notice":null,"core_skills":[]`, "$.*"},
		{"minimum version", `"monitoring_notice":null`, `"monitoring_notice":null,"upcoming_minimum_version":null`, "$.*"},
		{"unlisted name, startup remote control and unknown root", `}},"defaults":{"remote_control_at_startup":false}`, `},"allow_linked_computer":{"allowed":true}},"defaults":{"remote_control_at_startup":true},"PRIVATE_ROOT":"PRIVATE_VALUE"`, "$.*, defaults.remote_control_at_startup, restrictions.*"},
	} {
		t.Run("weakening/"+tc.name, func(t *testing.T) {
			doc := strings.Replace(server, tc.old, tc.replacement, 1)
			if doc == server {
				t.Fatal("fixture replacement did not apply")
			}
			check(t, doc, tc.want)
		})
	}
	// A populated remote-settings cache is refused whatever it names.
	if got := policyCachePath([]byte(server), true); got != "$.*" {
		t.Fatalf("remote cache path = %q, want %q", got, "$.*")
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

// Claim: admitting two more restriction names leaves the companion rule as it
// was. Beside a cache that holds all four admitted names, each of the six
// companions is admitted as a regular file up to the size limit and refused
// as a link, a dangling link, a directory, a fifo or an oversize file; the
// refusal names that companion and nothing in the admitted cache, and the
// companion is never opened. Metadata only: this says nothing about what a
// companion contains.
func TestPolicyCacheSidecarsBesideAdmittedNames(t *testing.T) {
	policy := policyWithRestrictions(
		[2]string{"allow_connector_file_downloads", `{"allowed":true}`}, [2]string{"allow_connector_file_uploads", `{"allowed":true}`},
		[2]string{"allow_computer_use", `{"allowed":false}`}, [2]string{"enforce_web_search_mcp_isolation", `{"allowed":true}`})
	for _, file := range []string{
		"policy-limits.json.stamp.json", "policy-limits.json.signature.json", "policy-limits.json.signature-iat.json",
		"remote-settings.json.signature.json", "remote-settings.json.signature-iat.json", "remote-settings.json.synced-at",
	} {
		for _, kind := range []string{"regular", "at-limit", "oversize", "symlink", "dangling", "directory", "fifo"} {
			t.Run(file+"/"+kind, func(t *testing.T) {
				home := t.TempDir()
				for name, doc := range map[string]string{"policy-limits.json": policy, "remote-settings.json": `{}`} {
					if err := os.WriteFile(filepath.Join(home, name), []byte(doc), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				path := filepath.Join(home, file)
				if err := os.WriteFile(path, []byte("SECRET"), 0o600); err != nil {
					t.Fatal(err)
				}
				var err error
				switch kind {
				case "at-limit":
					err = os.Truncate(path, maxPolicySidecarBytes)
				case "oversize":
					err = os.Truncate(path, maxPolicySidecarBytes+1)
				case "symlink", "dangling":
					target := path + ".target"
					if err = os.Rename(path, target); err == nil {
						if kind == "dangling" {
							target += ".absent"
						}
						err = os.Symlink(target, path)
					}
				case "directory":
					if err = os.Remove(path); err == nil {
						err = os.Mkdir(path, 0o700)
					}
				case "fifo":
					if err = os.Remove(path); err == nil {
						err = syscall.Mkfifo(path, 0o600)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				original := openPolicyCache
				defer func() { openPolicyCache = original }()
				openPolicyCache = func(opened string) (*os.File, error) {
					if filepath.Base(opened) != "policy-limits.json" && filepath.Base(opened) != "remote-settings.json" {
						t.Errorf("companion opened: %s", filepath.Base(opened))
					}
					return original(opened)
				}
				err = checkPolicyCaches(newGuardClock(context.Background(), time.Minute), "test", home)
				if kind == "regular" || kind == "at-limit" {
					if err != nil {
						t.Fatalf("admitted companion refused: %v", err)
					}
					return
				}
				if err == nil || !strings.HasSuffix(err.Error(), "server policy caches are not admitted: "+file+" at $") {
					t.Fatalf("refusal = %v, want only %s at $", err, file)
				}
				if strings.Contains(err.Error(), "SECRET") {
					t.Fatalf("companion content disclosed: %v", err)
				}
			})
		}
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
