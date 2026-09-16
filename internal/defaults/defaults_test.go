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
	t.Run("valid-link", func(t *testing.T) {
		p := paths(t)
		write(t, p.Machine, doc(`{"pi":{"model":"m"}}`, false))
		if err := os.Symlink(p.Machine, p.Operator); err != nil {
			t.Fatal(err)
		}
		f, err := defaults.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.Resolve("pi", defaults.Pair{})
		if err != nil || got.Model != resolved("m", defaults.OriginOperator) {
			t.Fatalf("%+v %v", got, err)
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
