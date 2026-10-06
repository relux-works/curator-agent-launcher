package defaults_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
)

func paths(t *testing.T) defaults.Paths {
	t.Helper()
	dir := t.TempDir()
	return defaults.Paths{Machine: filepath.Join(dir, "machine.json"), Operator: filepath.Join(dir, "operator.json")}
}
func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
func doc(entries string, locked bool) string {
	return fmt.Sprintf(`{"schema":"curator-run-defaults-v1","locked":%t,"defaults":%s}`, locked, entries)
}
func member(s string) defaults.Member { return defaults.Member{Value: s, Present: true} }
func resolved(s string, origin defaults.Origin) defaults.ResolvedMember {
	return defaults.ResolvedMember{Member: member(s), Origin: origin}
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	var e *defaults.Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("error = %v, want %s", err, want)
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"syntax": `{`, "trailing": `{} {}`, "root-null": `null`, "root-array": `[]`,
		"missing-schema": `{"defaults":{}}`, "wrong-schema": `{"schema":"v2","defaults":{}}`,
		"schema-null": `{"schema":null,"defaults":{}}`, "missing-defaults": `{"schema":"curator-run-defaults-v1"}`,
		"defaults-null": doc(`null`, false), "defaults-array": doc(`[]`, false),
		"unknown-env": doc(`{"future":{"model":"m"}}`, false),
		"empty-entry": doc(`{"pi":{}}`, false), "null-entry": doc(`{"pi":null}`, false), "array-entry": doc(`{"pi":[]}`, false),
		"unknown-member": doc(`{"pi":{"model":"m","extra":"x"}}`, false),
		"model-null":     doc(`{"pi":{"model":null}}`, false), "effort-null": doc(`{"pi":{"effort":null}}`, false),
		"model-number": doc(`{"pi":{"model":1}}`, false), "effort-bool": doc(`{"pi":{"effort":false}}`, false),
		"duplicate-model":   doc(`{"pi":{"model":"a","model":"b"}}`, false),
		"escaped-duplicate": doc(`{"pi":{"model":"a","\u006dodel":"b"}}`, false),
		"duplicate-env":     doc(`{"pi":{"model":"a"},"pi":{"model":"b"}}`, false),
		"duplicate-root":    `{"schema":"curator-run-defaults-v1","defaults":{},"defaults":{}}`,
		"unknown-sig":       `{"schema":"curator-run-defaults-v1","defaults":{},"sig":"x"}`,
		"locked-null":       `{"schema":"curator-run-defaults-v1","defaults":{},"locked":null}`,
		"locked-string":     `{"schema":"curator-run-defaults-v1","defaults":{},"locked":"true"}`,
		"surrogate":         doc(`{"pi":{"model":"\ud800"}}`, false),
		"utf8":              doc("{\"pi\":{\"model\":\"\xff\"}}", false),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			for _, operator := range []bool{false, true} {
				p := paths(t)
				target := p.Machine
				if operator {
					target = p.Operator
					write(t, p.Machine, doc(`{"pi":{"model":"locked"}}`, true))
				}
				write(t, target, data)
				f, err := defaults.Load(p)
				code(t, err, defaults.CodeInvalid)
				got, err := f.Resolve("pi", defaults.Pair{})
				if err != nil || got != (defaults.Partial{}) {
					t.Fatalf("partial leaked after load failure: %+v %v", got, err)
				}
			}
		})
	}
}

func TestPermissionDefaultsV2MergeAndClosedSchema(t *testing.T) {
	t.Run("v1-rejects-permissions-member", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v1","defaults":{"pi":{"permissions":"yolo"}}}`)
		_, err := defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
	})
	t.Run("v2-rejects-unknown-mode", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"pi":{"permissions":"automatic"}}}`)
		_, err := defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
	})
	t.Run("operator-over-machine-and-machine-lock", func(t *testing.T) {
		p := paths(t)
		write(t, p.Machine, `{"schema":"curator-run-defaults-v2","locked":false,"defaults":{"pi":{"permissions":"native"}}}`)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"pi":{"permissions":"yolo"}}}`)
		f, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.PermissionDefault("pi")
		if err != nil || got != member("yolo") {
			t.Fatalf("permission default = %+v, %v; want operator yolo", got, err)
		}
		write(t, p.Machine, `{"schema":"curator-run-defaults-v2","locked":true,"defaults":{"pi":{"permissions":"native"}}}`)
		f, err = defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err = f.PermissionDefault("pi")
		if err != nil || got != member("native") {
			t.Fatalf("locked permission default = %+v, %v; want machine native", got, err)
		}
		_, err = f.Resolve("pi", defaults.Pair{Permissions: member("native")})
		code(t, err, defaults.CodeUsage)
	})
}

func TestLoadKnownEnvironmentsAndPresence(t *testing.T) {
	p := paths(t)
	// Normative registry set from SPEC §4.2, including the known unsupported ID.
	write(t, p.Operator, `{"schema":"curator-run-defaults-v1","defaults":{"claude_code":{"model":""},"codex_cli":{"effort":""},"pi":{"model":"future-model","effort":"future-effort"},"opencode":{"model":"unadmitted"}}}`)
	f, err := defaults.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for env, want := range map[string]defaults.Partial{
		"claude_code": {Model: resolved("", defaults.OriginOperator)},
		"codex_cli":   {Effort: resolved("", defaults.OriginOperator)},
		"pi":          {Model: resolved("future-model", defaults.OriginOperator), Effort: resolved("future-effort", defaults.OriginOperator)},
		"opencode":    {Model: resolved("unadmitted", defaults.OriginOperator)},
	} {
		got, err := f.Resolve(env, defaults.Pair{})
		if err != nil || got != want {
			t.Errorf("%s: %+v %v want %+v", env, got, err, want)
		}
	}
	_, err = f.Resolve("future", defaults.Pair{})
	code(t, err, defaults.CodeInvalid)
}

// TestLoadRejectsAliasKeys is the reader-side bound on configuration keys:
// defaults.json keys stay the canonical §4.2 ids, and the CLI aliases are
// rejected as unknown environments at load in both files, and refused by
// Resolve. It proves reader rejection only; launch-time persistence is proven
// separately through the production entry point in
// TestProductionAliasPersistedBytesEqual (cmd/curator-run).
func TestLoadRejectsAliasKeys(t *testing.T) {
	for _, alias := range []string{"claude", "codex"} {
		for _, operator := range []bool{false, true} {
			p := paths(t)
			target := p.Machine
			if operator {
				target = p.Operator
			}
			write(t, target, doc(`{"`+alias+`":{"model":"m"}}`, false))
			_, err := defaults.Load(p)
			code(t, err, defaults.CodeInvalid)
			if err == nil || !strings.Contains(err.Error(), "unknown environment") {
				t.Fatalf("%s operator=%v: error %v does not name the unknown environment", alias, operator, err)
			}
		}
		empty := paths(t)
		f, err := defaults.Load(empty)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Resolve(alias, defaults.Pair{}); err == nil {
			t.Fatalf("Resolve(%q) accepted an alias", alias)
		} else {
			code(t, err, defaults.CodeInvalid)
		}
	}
}

func TestResolvePrecedence(t *testing.T) {
	// All 64 member-presence combinations independently select each source.
	for mask := 0; mask < 64; mask++ {
		t.Run(fmt.Sprint(mask), func(t *testing.T) {
			pairs := [3]defaults.Pair{}
			for source := 0; source < 3; source++ {
				if mask&(1<<(source*2)) != 0 {
					pairs[source].Model = member(fmt.Sprintf("m%d", source))
				}
				if mask&(1<<(source*2+1)) != 0 {
					pairs[source].Effort = member(fmt.Sprintf("e%d", source))
				}
			}
			p := paths(t)
			for i, path := range []string{p.Operator, p.Machine} {
				pair := pairs[i+1]
				members := []string{}
				if pair.Model.Present {
					members = append(members, fmt.Sprintf(`"model":%q`, pair.Model.Value))
				}
				if pair.Effort.Present {
					members = append(members, fmt.Sprintf(`"effort":%q`, pair.Effort.Value))
				}
				entries := `{}`
				if len(members) > 0 {
					entries = `{"pi":{` + strings.Join(members, ",") + `}}`
				}
				write(t, path, doc(entries, false))
			}
			f, err := defaults.Load(p)
			if err != nil {
				t.Fatal(err)
			}
			got, err := f.Resolve("pi", pairs[0])
			if err != nil {
				t.Fatal(err)
			}
			var want defaults.Partial
			for i, origin := range []defaults.Origin{defaults.OriginFlag, defaults.OriginOperator, defaults.OriginMachine} {
				if !want.Model.Present && pairs[i].Model.Present {
					want.Model = resolved(pairs[i].Model.Value, origin)
				}
				if !want.Effort.Present && pairs[i].Effort.Present {
					want.Effort = resolved(pairs[i].Effort.Value, origin)
				}
			}
			if got != want {
				t.Fatalf("got %+v want %+v", got, want)
			}
		})
	}
}

func TestResolveLocks(t *testing.T) {
	for _, name := range []string{"model", "effort"} {
		t.Run(name, func(t *testing.T) {
			p := paths(t)
			write(t, p.Machine, doc(fmt.Sprintf(`{"pi":{%q:""}}`, name), true))
			write(t, p.Operator, doc(`{"pi":{"model":"operator","effort":"operator"},"opencode":{"model":"other"}}`, true))
			f, err := defaults.Load(p)
			if err != nil {
				t.Fatal(err)
			}
			flags := defaults.Pair{}
			want := defaults.Partial{}
			if name == "model" {
				flags.Effort = member("")
				want.Model = resolved("", defaults.OriginMachine)
				want.Effort = resolved("", defaults.OriginFlag)
			} else {
				flags.Model = member("")
				want.Effort = resolved("", defaults.OriginMachine)
				want.Model = resolved("", defaults.OriginFlag)
			}
			got, err := f.Resolve("pi", flags)
			if err != nil || got != want {
				t.Fatalf("unset locked member: %+v %v", got, err)
			}
			got, err = f.Resolve("pi", defaults.Pair{})
			if err != nil {
				t.Fatal(err)
			}
			if name == "model" {
				want.Effort = defaults.ResolvedMember{}
			} else {
				want.Model = defaults.ResolvedMember{}
			}
			if got != want {
				t.Fatalf("ignored operator supplied value: %+v", got)
			}
			for _, v := range []string{"", "different"} {
				if name == "model" {
					flags.Model = member(v)
				} else {
					flags.Effort = member(v)
				}
				got, err = f.Resolve("pi", flags)
				code(t, err, defaults.CodeUsage)
				if !strings.Contains(err.Error(), "--"+name) || got != (defaults.Partial{}) {
					t.Fatalf("lock refusal: %+v %v", got, err)
				}
			}
			got, err = f.Resolve("opencode", defaults.Pair{Model: member("flag")})
			if err != nil || got.Model != resolved("flag", defaults.OriginFlag) {
				t.Fatalf("lock spread to unnamed env/operator lock: %+v %v", got, err)
			}
		})
	}
}

func TestLoadFilesystem(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		p := paths(t)
		p.Operator = filepath.Join(p.Operator, "nested", "defaults.json")
		f, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.Resolve("pi", defaults.Pair{})
		if err != nil || got != (defaults.Partial{}) {
			t.Fatalf("%+v %v", got, err)
		}
	})
	for _, kind := range []string{"directory", "broken-link", "broken-parent-link", "link-loop", "read-permission", "parent-not-directory"} {
		t.Run(kind, func(t *testing.T) {
			p := paths(t)
			switch kind {
			case "directory":
				if err := os.Mkdir(p.Operator, 0700); err != nil {
					t.Fatal(err)
				}
			case "broken-link", "broken-parent-link":
				if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), p.Operator); err != nil {
					t.Fatal(err)
				}
				if kind == "broken-parent-link" {
					p.Operator = filepath.Join(p.Operator, "defaults.json")
				}
			case "link-loop":
				if err := os.Symlink(p.Operator, p.Operator); err != nil {
					t.Fatal(err)
				}
			case "read-permission":
				write(t, p.Operator, doc(`{}`, false))
				if err := os.Chmod(p.Operator, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(p.Operator, 0600) })
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permission bits")
				}
			case "parent-not-directory":
				write(t, p.Operator, "x")
				p.Operator = filepath.Join(p.Operator, "defaults.json")
			}
			_, err := defaults.Load(p)
			code(t, err, defaults.CodeInvalid)
		})
	}
	t.Run("linked-file-is-refused", func(t *testing.T) {
		p := paths(t)
		write(t, p.Machine, doc(`{"pi":{"model":"m"}}`, false))
		if err := os.Symlink(p.Machine, p.Operator); err != nil {
			t.Fatal(err)
		}
		_, err := defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
		if !strings.Contains(err.Error(), "symlinked configuration file") {
			t.Fatalf("linked file refusal has no named reason: %v", err)
		}
	})
	t.Run("empty-path", func(t *testing.T) { _, err := defaults.Load(defaults.Paths{}); code(t, err, defaults.CodeInvalid) })
}

func TestConfigPaths(t *testing.T) {
	for _, xdg := range []string{"", "/explicit/config"} {
		got := defaults.ConfigPaths(xdg, "/explicit/home")
		operator := "/explicit/config/curator-run/defaults.json"
		if xdg == "" {
			operator = "/explicit/home/.config/curator-run/defaults.json"
		}
		if got.Machine != "/etc/curator-run/defaults.json" || got.Operator != operator {
			t.Fatalf("%+v", got)
		}
	}
}

func TestResolveExplicitEmptyOverrides(t *testing.T) {
	p := paths(t)
	write(t, p.Machine, doc(`{"pi":{"model":"machine","effort":"machine"}}`, false))
	write(t, p.Operator, doc(`{"pi":{"model":"","effort":""}}`, true))
	f, err := defaults.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Resolve("pi", defaults.Pair{})
	want := defaults.Partial{Model: resolved("", defaults.OriginOperator), Effort: resolved("", defaults.OriginOperator)}
	if err != nil || got != want {
		t.Fatalf("operator empty: %+v %v", got, err)
	}
	got, err = f.Resolve("pi", defaults.Pair{Model: member(""), Effort: member("")})
	want = defaults.Partial{Model: resolved("", defaults.OriginFlag), Effort: resolved("", defaults.OriginFlag)}
	if err != nil || got != want {
		t.Fatalf("flag empty: %+v %v", got, err)
	}
}

func docV3(entries string, locked bool) string {
	return fmt.Sprintf(`{"schema":"curator-run-defaults-v3","locked":%t,"defaults":%s}`, locked, entries)
}

func TestHostDefaultsV3MergeAndClosedSchema(t *testing.T) {
	t.Run("v1-rejects-host-member", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v1","defaults":{"pi":{"host":"hosted"}}}`)
		_, err := defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
	})
	t.Run("v2-rejects-host-member", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v2","defaults":{"pi":{"permissions":"native","host":"hosted"}}}`)
		_, err := defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
	})
	t.Run("v3-rejects-unknown-host", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"pi":{"host":"cloud"}}}`)
		_, err := defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
	})
	t.Run("v3-rejects-non-string-host", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"pi":{"host":true}}}`)
		_, err := defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
	})
	t.Run("v3-rejects-unknown-sibling-member", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"pi":{"host":"native","model":"m","extra":"x"}}}`)
		_, err := defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
	})
	t.Run("v3-keeps-permissions-and-empty-entry-rule", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"pi":{"permissions":"yolo","host":"native"},"codex_cli":{"model":"m"}}}`)
		f, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.Resolve("pi", defaults.Pair{})
		if err != nil || got.Permissions != resolved("yolo", defaults.OriginOperator) {
			t.Fatalf("v3 permissions = %+v, %v", got, err)
		}
		write(t, p.Operator, `{"schema":"curator-run-defaults-v3","defaults":{"pi":{}}}`)
		_, err = defaults.Load(p)
		code(t, err, defaults.CodeInvalid)
		if err == nil || !strings.Contains(err.Error(), "host") {
			t.Fatalf("v3 empty entry error %v does not name host", err)
		}
	})
	t.Run("host-only-entry-loads", func(t *testing.T) {
		p := paths(t)
		write(t, p.Operator, docV3(`{"claude_code":{"host":"hosted"}}`, false))
		f, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.ResolveHost("claude_code", "")
		if err != nil || got.Host != defaults.HostHosted || got.Origin != defaults.OriginHostOperator {
			t.Fatalf("host = %+v, %v; want hosted/operator", got, err)
		}
	})
}

// TestResolveHostPrecedence drives every row of the SPEC §4.8 host table:
// flag wins unlocked; operator beats machine; machine beats the implicit
// native default; a locked machine naming the environment ignores the whole
// operator entry and refuses a flag over its own host, even when equal.
func TestResolveHostPrecedence(t *testing.T) {
	v3 := func(host string) string {
		if host == "" {
			return `{"model":"m"}`
		}
		return fmt.Sprintf(`{"host":%q}`, host)
	}
	for _, tc := range []struct {
		name              string
		machine, operator string
		locked            bool
		flag              string
		wantHost          string
		wantOrigin        defaults.Origin
		wantCode          string
	}{
		{"no-flag-no-default-is-native", "", "", false, "", "native", defaults.OriginHostDefault, ""},
		{"flag-hosted-wins", "native", "native", false, "--hosted", "hosted", defaults.OriginHostFlag, ""},
		{"flag-native-wins", "hosted", "hosted", false, "--native", "native", defaults.OriginHostFlag, ""},
		{"untracked-equals-native", "hosted", "hosted", false, "--untracked", "native", defaults.OriginHostFlag, ""},
		{"operator-beats-machine", "native", "hosted", false, "", "hosted", defaults.OriginHostOperator, ""},
		{"machine-beats-default", "hosted", "", false, "", "hosted", defaults.OriginHostMachine, ""},
		{"machine-native-is-configured", "native", "", false, "", "native", defaults.OriginHostMachine, ""},
		{"operator-native-is-configured", "", "native", false, "", "native", defaults.OriginHostOperator, ""},
		{"locked-ignores-operator", "", "hosted", true, "", "native", defaults.OriginHostDefault, ""},
		{"locked-keeps-machine-host", "hosted", "native", true, "", "hosted", defaults.OriginHostMachine, ""},
		{"locked-flag-without-machine-host-wins", "", "hosted", true, "--hosted", "hosted", defaults.OriginHostFlag, ""},
		{"locked-flag-over-machine-host-refuses", "hosted", "native", true, "--native", "", "", defaults.CodeUsage},
		{"locked-equal-flag-still-refuses", "native", "hosted", true, "--native", "", "", defaults.CodeUsage},
		{"locked-equal-hosted-flag-still-refuses", "hosted", "native", true, "--hosted", "", "", defaults.CodeUsage},
		{"lock-names-other-env-only", "hosted", "native", true, "", "native", defaults.OriginHostOperator, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := paths(t)
			env := "pi"
			if tc.name == "lock-names-other-env-only" {
				write(t, p.Machine, docV3(`{"codex_cli":`+v3(tc.machine)+`}`, tc.locked))
			} else if tc.machine != "" || tc.locked {
				write(t, p.Machine, docV3(`{"pi":`+v3(tc.machine)+`}`, tc.locked))
			}
			if tc.operator != "" {
				write(t, p.Operator, docV3(`{"pi":`+v3(tc.operator)+`}`, false))
			}
			f, err := defaults.Load(p)
			if err != nil {
				t.Fatal(err)
			}
			got, err := f.ResolveHost(env, tc.flag)
			if tc.wantCode != "" {
				code(t, err, tc.wantCode)
				if !strings.Contains(err.Error(), tc.flag) || got != (defaults.ResolvedHost{}) {
					t.Fatalf("lock refusal: %+v %v", got, err)
				}
				return
			}
			if err != nil || got.Host != tc.wantHost || got.Origin != tc.wantOrigin {
				t.Fatalf("host = %+v, %v; want %s/%s", got, err, tc.wantHost, tc.wantOrigin)
			}
		})
	}
}

func TestResolveHostRejectsUnknown(t *testing.T) {
	f, err := defaults.Load(paths(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.ResolveHost("future", ""); err == nil {
		t.Fatal("ResolveHost accepted an unknown environment")
	} else {
		code(t, err, defaults.CodeInvalid)
	}
	if _, err := f.ResolveHost("pi", "--cloud"); err == nil {
		t.Fatal("ResolveHost accepted an unknown host flag")
	} else {
		code(t, err, defaults.CodeInvalid)
	}
}
