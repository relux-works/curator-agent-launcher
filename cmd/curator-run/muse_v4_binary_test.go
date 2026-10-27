//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

func frontV4(t *testing.T, f *frontFixture, environment string, root bool) *fragment.Fragment {
	t.Helper()
	m := map[string]any{"fragment": fragment.IdentityV4, "environment": environment, "profile": map[string]any{"name": "sample", "lock_sha256": strings.Repeat("1", 64)}, "precedence": map[string]any{"winner": "higher-weight", "placement": "winner-last"}, "permissions": map[string]any{"mode": "native", "source": "profile", "locked": false}}
	if environment == fragment.EnvMuse {
		env := map[string]string{"TBH_CREDENTIAL_BACKEND": "file"}
		for _, kind := range []string{"config", "data", "state", "cache"} {
			env["XDG_"+strings.ToUpper(kind)+"_HOME"] = filepath.Join(f.root, "managed", kind)
		}
		m["env"] = env
		m["credential_store"] = map[string]any{"selection": "file"}
		if root {
			m["root_context"] = map[string]any{"exclusive": true}
		}
	} else {
		m["env"] = map[string]string{fragment.HomeVariable(environment): filepath.Join(f.root, "managed")}
	}
	raw := frontJSON(m)
	writeFixture(t, filepath.Join(f.root, "fragment.json"), raw, 0600)
	frag, err := fragment.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	f.managerConfig.Profiles["sample"] = fakemanager.Profile{LockSHA256: frag.Profile.LockSHA256, FragmentDigest: frag.Digest}
	return frag
}
func museBinaryArgs() []string {
	return []string{"muse", "--model", "muse-wonder-1.0-contributor", "--effort", "xhigh", "--permissions", "native"}
}
func readNative(t *testing.T, f *frontFixture) childCapture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, "native.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c childCapture
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// Real binary + real module + local provider double prove exact interactive
// argv and the backend/HOME invariant; refusals must have no provider effect.
// The public CLI has no exec/dry-run launch form; those are tested below at
// the real module construction boundary, not represented as end-to-end proof.
func TestMuseV4Binary(t *testing.T) {
	bin := frontBuild(t)
	for _, root := range []bool{false, true} {
		for _, mode := range []string{"native", "yolo"} {
			t.Run(mode+map[bool]string{false: "-without-root", true: "-root"}[root], func(t *testing.T) {
				f := newFront(t, bin)
				frag := frontV4(t, f, fragment.EnvMuse, root)
				f.env = append(f.env, "TBH_CREDENTIAL_BACKEND=keychain")
				args := museBinaryArgs()
				args[len(args)-1] = mode
				args = append(args, "--native", "--", "hello")
				code, _, stderr := f.command(bin.launcher, args, nil, false, false, 12*time.Second)
				f.require(t, code, 0, stderr, "", nil, 0, true)
				c := readNative(t, f)
				want := museArgv(f.root, agentic.PermissionModeNative)
				if root {
					want = append(want, "-c", "context.foreign_personal_skills=false")
				}
				if mode == "yolo" {
					want = append(want, "--yolo")
				}
				want = append(want, "hello")
				if !slices.Equal(c.Argv, want) {
					t.Fatalf("argv=%q want=%q", c.Argv, want)
				}
				env := map[string]string{}
				for _, e := range c.Env {
					k, v, _ := strings.Cut(e, "=")
					env[k] = v
				}
				if env["HOME"] != f.root || env["TBH_CREDENTIAL_BACKEND"] != "file" {
					t.Fatalf("wrong home/backend: %v", env)
				}
				for k, v := range frag.Env {
					if env[k] != v {
						t.Fatalf("lost %s", k)
					}
				}
			})
		}
	}
	for _, tail := range [][]string{{"exec", "hello"}, {"serve"}, {"--model=other", "serve"}, {"-c", "context.foreign_personal_skills=true"}, {"--config=context.foreign_personal_skills=false"}, {"--no-foreign-personal-context"}} {
		t.Run("refuse/"+strings.Join(tail, "_"), func(t *testing.T) {
			f := newFront(t, bin)
			frontV4(t, f, fragment.EnvMuse, true)
			args := append(museBinaryArgs(), "--native", "--")
			args = append(args, tail...)
			code, _, stderr := f.command(bin.launcher, args, nil, false, false, 12*time.Second)
			f.require(t, code, 1, stderr, "plan_refused", nil, 0, false)
			if tail[0] == "exec" || slices.Contains(tail, "serve") {
				form := "serve"
				if tail[0] == "exec" {
					form = "exec"
				}
				if !strings.Contains(stderr, "form="+form) {
					t.Fatal(stderr)
				}
			}
		})
	}
	// No fact means no inferred exclusion and the existing native tail survives.
	for _, form := range []string{"exec", "serve"} {
		t.Run("absent-fact/"+form, func(t *testing.T) {
			f := newFront(t, bin)
			frontV4(t, f, fragment.EnvMuse, false)
			code, _, stderr := f.command(bin.launcher, append(museBinaryArgs(), "--native", "--", form), nil, false, false, 12*time.Second)
			f.require(t, code, 0, stderr, "", nil, 0, true)
			want := append(museArgv(f.root, agentic.PermissionModeNative), form)
			if !slices.Equal(readNative(t, f).Argv, want) {
				t.Fatal("fact inferred or tail rewritten")
			}
		})
	}
	// Whole-fragment failures must stop before manager contact or provider exec.
	for _, bad := range []string{"unknown-member", "missing-backend", "duplicate-root", "v3-with-v4"} {
		t.Run("invalid/"+bad, func(t *testing.T) {
			f := newFront(t, bin)
			frontV4(t, f, fragment.EnvMuse, true)
			path := filepath.Join(f.root, "fragment.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			switch bad {
			case "unknown-member":
				value["future"] = true
			case "missing-backend":
				delete(value["env"].(map[string]any), "TBH_CREDENTIAL_BACKEND")
			case "v3-with-v4":
				value["fragment"] = fragment.IdentityV3
			}
			raw = frontJSON(value)
			if bad == "duplicate-root" {
				raw = []byte(strings.Replace(string(raw), `"exclusive":true`, `"exclusive":true,"exclusive":false`, 1))
			}
			writeFixture(t, path, raw, 0600)
			code, _, stderr := f.command(bin.launcher, append(museBinaryArgs(), "--native"), nil, false, false, 12*time.Second)
			want, diagnostic := 1, "resolve_fragment_invalid"
			if bad == "v3-with-v4" {
				want, diagnostic = 2, "fragment_version_unsupported"
			}
			f.require(t, code, want, stderr, diagnostic, nil, 0, false)
		})
	}
	for _, rev := range []string{fragment.Identity, fragment.IdentityV2, fragment.IdentityV3} {
		t.Run("upgrade/"+rev, func(t *testing.T) {
			f := newFront(t, bin)
			frontV4(t, f, fragment.EnvMuse, false)
			path := filepath.Join(f.root, "fragment.json")
			raw, _ := os.ReadFile(path)
			var m map[string]any
			json.Unmarshal(raw, &m)
			m["fragment"] = rev
			delete(m, "credential_store")
			delete(m["env"].(map[string]any), "TBH_CREDENTIAL_BACKEND")
			if rev == fragment.Identity {
				delete(m, "permissions")
			}
			writeFixture(t, path, frontJSON(m), 0600)
			code, _, stderr := f.command(bin.launcher, append(museBinaryArgs(), "--native"), nil, false, false, 12*time.Second)
			f.require(t, code, 2, stderr, "fragment_version_unsupported", nil, 0, false)
			want := "curator-run: fragment_version_unsupported: Fragment version unsupported. environment=muse required=launch-env-fragment-v4\n"
			if stderr != want {
				t.Fatalf("wrong exact upgrade refusal: %q", stderr)
			}
		})
	}
}

// The pinned module refuses unsupported file/keychain policy before execution,
// preserving config.toml exactly. Native and absent policy controls launch.
// Positive -c mapping belongs to the released module-consumer follow-up.
func TestCodexV4BinaryCredentialStore(t *testing.T) {
	bin := frontBuild(t)
	for _, selection := range []string{"absent", "native", "file", "keychain"} {
		t.Run(selection, func(t *testing.T) {
			f := newFront(t, bin)
			frontV4(t, f, fragment.EnvCodexCLI, false)
			path := filepath.Join(f.root, "fragment.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			if selection != "absent" {
				store := map[string]any{"selection": selection}
				if selection == "file" || selection == "keychain" {
					value := "file"
					if selection == "keychain" {
						value = "keyring"
					}
					store["config_overrides"] = map[string]string{"cli_auth_credentials_store": value}
				}
				m["credential_store"] = store
			}
			writeFixture(t, path, frontJSON(m), 0600)
			config := filepath.Join(f.root, "managed", "config.toml")
			original := []byte("# preserved\n")
			writeFixture(t, config, original, 0600)
			args := []string{"codex", "--model", "gpt-6-sol", "--effort", "medium", "--permissions", "native", "--native"}
			code, _, stderr := f.command(bin.launcher, args, nil, false, false, 12*time.Second)
			if selection == "file" || selection == "keychain" {
				f.require(t, code, 1, stderr, "plan_refused", nil, 0, false)
				want := "curator-run: plan_refused: spawn-plane admission refused the launch\nplan_refused: spawn plane refused the launch: agentic: building plan for codex: agentic: selected system cannot apply Curator context: codex_cli credential_store \"" + selection + "\" is not applied by this launcher\n"
				if !strings.HasSuffix(stderr, want) {
					t.Fatalf("wrong typed policy refusal: %s", stderr)
				}
			} else {
				f.require(t, code, 0, stderr, "", nil, 0, true)
				for _, arg := range readNative(t, f).Argv {
					if strings.Contains(arg, "cli_auth_credentials_store") {
						t.Fatalf("unexpected policy override: %s", arg)
					}
				}
			}
			after, err := os.ReadFile(config)
			if err != nil || string(after) != string(original) {
				t.Fatalf("config rewritten: %v %q", err, after)
			}
		})
	}
}

// This is the actual tagged module Exec/DryRun boundary, not a public CLI form.
// Both modes carry the explicit root fact and backend into their sealed plan.
func TestMuseV4ExecDryRunModuleBoundary(t *testing.T) {
	f, frag := museFixture(t)
	frag.RootContext = &fragment.RootContext{Exclusive: true}
	writeFixture(t, filepath.Join(f.dir, "muse-version"), []byte("Muse Code 1.4.7 (1.4.7-R6101.1)\n"), 0600)
	req := plan.SpawnRequest(plan.Request{Runtime: "muse", Model: "muse-wonder-1.0-contributor", Effort: "xhigh", ToolRelease: "1.4.7", Context: plan.Context(frag, ""), Home: frag.Home(), WorkDir: f.dir, Env: f.deps.environ()})
	var actual []string
	for _, mode := range []agentic.LaunchMode{agentic.LaunchModeExec, agentic.LaunchModeDryRun} {
		built, err := vendorplugin.BuildLaunchWithEnvironment(context.Background(), f.deps.registry, req, mode)
		if err != nil {
			t.Fatal(err)
		}
		provenance, present := built.Plan.CuratorContextProvenanceSnapshot()
		if !present || provenance.Fragment.RootContext == nil || !provenance.Fragment.RootContext.Exclusive || provenance.Fragment.CredentialStore == nil || provenance.Fragment.CredentialStore.Selection != agentic.CuratorCredentialStoreFile {
			t.Fatal("v4 facts lost before sealing")
		}
		args := built.Plan.Argv
		if len(args) == 0 || args[0] != "exec" || slices.Contains(args, "context.foreign_personal_skills=false") || !slices.Contains(args, "--no-foreign-personal-context") {
			t.Fatalf("wrong %s mapping: %q", mode, args)
		}
		if mode == agentic.LaunchModeExec {
			actual = args
		} else {
			// DryRun's existing assignment placeholder is the sole permitted delta.
			want := append(slices.Clone(actual), "--prompt-file", "<prompt-file>")
			if !reflect.DeepEqual(args, want) {
				t.Fatalf("dry-run=%q exec=%q", args, actual)
			}
		}
		if !slices.Contains(built.Plan.Env, "TBH_CREDENTIAL_BACKEND=file") {
			t.Fatal("lost file backend")
		}
	}
	f.assertNoChild(t)
}

// The delegated request's existing profile digest binds the complete v4
// fragment. The no-op fake witnesses routing and digest fences, not a real
// manager's Muse process composition or its content verification.
func TestMuseV4HostedBinary(t *testing.T) {
	bin := frontBuild(t)
	for _, tc := range []struct {
		name       string
		ops        []string
		missingCap string
		stale      bool
		want       int
		calls      []string
	}{
		{name: "qualified", ops: []string{"launch", "attach"}, want: 1, calls: []string{"describe", "launch"}},
		{name: "missing-runtime", want: 6, calls: []string{"describe"}},
		{name: "no-attach", ops: []string{"launch"}, want: 6, calls: []string{"describe"}},
		{name: "no-launch", ops: []string{"attach"}, want: 6, calls: []string{"describe"}},
		{name: "no-attach-cap", ops: []string{"launch", "attach"}, missingCap: "attach/1", want: 6, calls: []string{"describe"}},
		{name: "no-launch-cap", ops: []string{"launch", "attach"}, missingCap: "launch-delegated/1", want: 6, calls: []string{"describe"}},
		{name: "root-fact-drift", ops: []string{"launch", "attach"}, stale: true, want: 16, calls: []string{"describe", "launch"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFront(t, bin)
			if tc.ops != nil {
				f.managerConfig = fakemanager.MuseConfig()
				f.managerConfig.Runtimes[fragment.EnvMuse] = append(slices.Clone(tc.ops), "status", "cancel")
			}
			current := frontV4(t, f, fragment.EnvMuse, false)
			// Same bytes produce the same digest. Only adding root_context changes
			// it; the manager's stale pin below differs on that fact alone.
			raw, err := os.ReadFile(filepath.Join(f.root, "fragment.json"))
			if err != nil {
				t.Fatal(err)
			}
			same, err := fragment.Parse(raw)
			if err != nil || same.Digest != current.Digest {
				t.Fatal("unstable control digest", err)
			}
			withRoot := frontV4(t, f, fragment.EnvMuse, true)
			if withRoot.Digest == current.Digest {
				t.Fatal("root_context did not change fragment digest")
			}
			if tc.stale {
				f.managerConfig.Profiles["sample"] = fakemanager.Profile{LockSHA256: current.Profile.LockSHA256, FragmentDigest: current.Digest}
			}
			if tc.missingCap != "" {
				f.managerConfig.Capabilities = slices.DeleteFunc(f.managerConfig.Capabilities, func(s string) bool { return s == tc.missingCap })
			}
			f.reset()
			f.policy("auto", true)
			args := append(museBinaryArgs(), "--hosted", "--", "hello")
			code, _, stderr := f.command(bin.launcher, args, nil, true, false, 12*time.Second)
			diagnostic := ""
			if tc.name == "qualified" {
				diagnostic = "process_incarnation_unavailable"
			}
			if tc.want == 6 {
				diagnostic = "session_host_provider_unsupported"
			}
			if tc.stale {
				diagnostic = "policy_refused"
			}
			starts := 0
			if tc.name == "qualified" {
				starts = 1
			}
			f.require(t, code, tc.want, stderr, diagnostic, tc.calls, starts, false)
			if tc.name == "qualified" {
				var state fakemanager.State
				if err := json.Unmarshal(f.observe().Records, &state); err != nil {
					t.Fatal(err)
				}
				if len(state.Sessions) != 1 {
					t.Fatalf("sessions=%d", len(state.Sessions))
				}
				for _, session := range state.Sessions {
					var request map[string]any
					if err := json.Unmarshal(session.Plan, &request); err != nil {
						t.Fatal(err)
					}
					profile := request["profile"].(map[string]any)
					if profile["fragment_digest"] != withRoot.Digest || request["runtime"] != fragment.EnvMuse {
						t.Fatal("lost v4 digest binding")
					}
					for _, key := range []string{"root_context", "credential_store"} {
						if _, ok := request[key]; ok {
							t.Fatal("closed request widened:", key)
						}
					}
				}
			}
		})
	}
}

// Freeze the old v3 composition projection independently of the new launch
// refusal: the parser and composer still reproduce every historical golden.
func TestMuseV3CompositionGoldens(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode agentic.PermissionMode
	}{{"native", agentic.PermissionModeNative}, {"yolo", agentic.PermissionModeYolo}, {"yolo-alias", agentic.PermissionModeYolo}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			raw, err := os.ReadFile("../../internal/fragment/testdata/v3/muse.json")
			if err != nil {
				t.Fatal(err)
			}
			frag, err := fragment.Parse([]byte(strings.ReplaceAll(string(raw), "/manager/environments/default/muse", filepath.Join(root, "managed"))))
			if err != nil {
				t.Fatal(err)
			}
			p := agentic.Plan{Argv: museArgv(root, tc.mode), Env: []string{"HOME=" + root, "PATH=" + root, "MUSE_NO_AUTO_UPDATE=1"}, WorkDir: root, Stdin: agentic.StdinPayload{Attached: true, Bytes: []byte("parent stdin\n\x00\xff")}}
			v, err := composition.Compose(p, []string{"MUSE_NO_AUTO_UPDATE=1"}, *frag, composition.PromptApplication{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, "pipeline-muse-"+tc.name, childCapture{Argv: v.Argv, Env: v.Env, Stdin: v.RawStdin.Bytes, WorkDir: v.WorkDir}, root)
		})
	}
}

// Werkstatt preparation must forward v4 policy too: native Codex is a
// successful control, while file reaches the module's typed refusal without
// starting a manager session or executing a native provider.
func TestCodexV4WerkstattPreparation(t *testing.T) {
	bin := frontBuild(t)
	for _, file := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "file-refusal"}[file], func(t *testing.T) {
			w := newWS(t, bin)
			frontV4(t, w.frontFixture, fragment.EnvCodexCLI, false)
			w.record["runtime"], w.record["model"], w.record["effort"] = "codex_cli", "gpt-6-sol", "medium"
			w.write()
			if file {
				path := filepath.Join(w.root, "fragment.json")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var value map[string]any
				if err := json.Unmarshal(raw, &value); err != nil {
					t.Fatal(err)
				}
				value["credential_store"] = map[string]any{"selection": "file", "config_overrides": map[string]string{"cli_auth_credentials_store": "file"}}
				writeFixture(t, path, frontJSON(value), 0600)
			}
			w.reset()
			if file {
				w.runWS("up", 1, `codex_cli credential_store "file"`)
				w.expectStarts(0)
			} else {
				w.runWS("up", 1, "process_incarnation_unavailable")
				w.expectStarts(1)
			}
			if _, err := os.Stat(filepath.Join(w.root, "native-called")); !os.IsNotExist(err) {
				t.Fatal("native provider executed")
			}
		})
	}
}
