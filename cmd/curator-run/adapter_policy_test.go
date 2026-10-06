package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/relux-works/curator-network-profiles/pkg/binding"
	"github.com/relux-works/curator-network-profiles/pkg/probe"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/network"
	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/netprofile"
)

// Tests drive run -> launch -> network.Prepare -> Admit -> Evaluate ->
// BoundIdentity/scope -> Compose -> EmitAdapterProvenance -> Run. Mutations
// replace dependencies at existing seams, never source text. Baseline calls the
// real library and emitter; only a subprocess with OPTION_C_MUTANT weakens one
// member of a rejected class.
func optionCFixture(t *testing.T, tracked bool) *pipelineFixture {
	t.Helper()
	f := printNetworkFixture(t, tracked)
	writeDirectNetworkCatalog(t, f.dir, true)
	f.args = withNetworkFlag(f.args, "direct-a")
	f.deps.prober = forbiddenNetworkProber{t}
	switch os.Getenv("OPTION_C_MUTANT") {
	case "OperatorText-not-shown":
		f.deps.networkEmit = func(w io.Writer, p *adapterprobe.Provenance) error {
			if p == nil || p.BuildID != artifactBuild(t, f) {
				return network.EmitAdapterProvenance(w, p)
			}
			data, _ := json.Marshal(p)
			_, e := io.WriteString(w, "curator-run: adapter-provenance: "+string(data)+"\n")
			return e
		}
	case "provenance-not-stored":
		f.deps.networkEmit = func(w io.Writer, p *adapterprobe.Provenance) error {
			if p == nil || p.BuildID != artifactBuild(t, f) {
				return network.EmitAdapterProvenance(w, p)
			}
			_, e := io.WriteString(w, p.OperatorText()+"\n")
			return e
		}
	case "sensitive-egress-downgrade":
		f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
			if p.Mode == adapterprobe.Optimistic && p.SensitiveEgress {
				p.SensitiveEgress = false
			}
			return adapterprobe.Evaluate(c, r, p)
		}
	case "KnownBadErr-dropped":
		f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
			if u, ok := p.KnownBadErr.(*adapterprobe.Unsupported); ok && u.Reason == "knownbad_unreadable" {
				p.KnownBadErr = nil
			}
			return adapterprobe.Evaluate(c, r, p)
		}
	case "known-bad-member-admitted":
		f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
			if p.KnownBad != nil && p.KnownBad.Contains(strings.TrimPrefix(artifactBuild(t, f), "sha256-")) {
				p.KnownBad = nil
			}
			return adapterprobe.Evaluate(c, r, p)
		}
	case "pin-mismatch-admitted":
		f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
			if p.Mode == adapterprobe.Pinned && p.PinnedBuild == "sha256-"+strings.Repeat("a", 64) {
				p.PinnedBuild = artifactBuild(t, f)
			}
			return adapterprobe.Evaluate(c, r, p)
		}
	case "strict-miss-admitted":
		f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
			if p.Mode == adapterprobe.Strict && !p.SensitiveEgress {
				p.Mode = adapterprobe.Optimistic
			}
			return adapterprobe.Evaluate(c, r, p)
		}
	case "host-build-mismatch-admitted":
		f.deps.networkMatchIdentity = func(a, b binding.AdapterIdentity) bool { a.Build = b.Build; return a == b }
	case "scope-gate-removed":
		// The gate stays present but admits exactly codex exec in addition to Claude.
		f.deps.networkScopeCheck = func(id adapterprobe.Identity) bool {
			return network.ScopeAllowed(id) || id == (adapterprobe.Identity{Adapter: "generic-env-v1", Harness: "codex-cli", Entrypoint: "exec"})
		}
	case "tracked-session-admitted":
		f.deps.networkSessionAllowed = func(tracked, hosted bool) bool { return !hosted }
	}
	return f
}

func adapterRecord(t *testing.T, stderr []byte) adapterprobe.Provenance {
	t.Helper()
	const prefix = "curator-run: adapter-provenance: "
	var p adapterprobe.Provenance
	count := 0
	for _, line := range strings.Split(string(stderr), "\n") {
		if strings.HasPrefix(line, prefix) {
			count++
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &p); err != nil {
				t.Fatal("typed record is not a parseable JSON line")
			}
		}
	}
	if count != 1 || p.Schema != adapterprobe.ProvenanceSchema || p.Outcome != "unqualified_build" || p.Scope != adapterprobe.TransportScope || p.BuildID != "sha256-"+p.BinarySHA256 || p.Recipe != "claude-exec-v1" || p.Adapter != (adapterprobe.Identity{Adapter: "generic-env-v1", Harness: "claude-code", Entrypoint: "exec"}) {
		t.Fatal("full typed record missing or changed")
	}
	return p
}

func TestOptionCOperatorTextVerbatim(t *testing.T) {
	f := optionCFixture(t, false)
	code, _, stderr := f.run()
	if code != 0 {
		t.Fatalf("launch exit=%d", code)
	}
	p := adapterRecord(t, stderr)
	if !bytes.Contains(stderr, []byte(p.OperatorText()+"\n")) {
		t.Fatal("verbatim OperatorText missing on stderr")
	}
}

func TestOptionCProvenanceRecordForSessionWrapper(t *testing.T) {
	f := optionCFixture(t, false)
	code, _, stderr := f.run()
	if code != 0 {
		t.Fatalf("launch exit=%d", code)
	}
	p := adapterRecord(t, stderr)
	if p.BuildID != artifactBuild(t, f) {
		t.Fatal("record identifies another artifact")
	}
	// Persisting the record is the wrapper's responsibility. Launcher creates no
	// session file; a direct wrapper can retain exactly this JSON record.
	raw, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	var stored adapterprobe.Provenance
	if e := json.Unmarshal(raw, &stored); e != nil || stored != p {
		t.Fatal("record cannot round-trip into wrapper session")
	}
}

func TestOptionCSessionRecordGuard(t *testing.T) {
	for _, path := range []string{"tracked", "hosted"} {
		t.Run(path, func(t *testing.T) {
			f := optionCFixture(t, path == "tracked")
			if path == "hosted" {
				insertLauncherArgs(f, "--hosted")
			}
			code, out, stderr := f.run()
			if code == 0 || len(out) != 0 || !bytes.Contains(stderr, []byte("network_scope_unsupported")) {
				t.Fatal("session-owning path admitted network without stored adapter provenance")
			}
			if path == "hosted" && f.builds != 0 {
				t.Fatal("hosted network reached session build without stored adapter provenance")
			}
			f.assertNoChild(t)
		})
	}
}

func TestOptionCProvenanceCannotBeSuppressed(t *testing.T) {
	// Launcher has no suppression flags: quiet/silent/JSON-only at its own
	// flag position are usage errors. Native flags are opaque; every form below
	// launches with both lines on stderr regardless of the child output channel.
	for _, flags := range [][]string{{"--quiet"}, {"--silent"}, {"--output-format", "json"}, {"--json-only"}} {
		t.Run(strings.Join(flags, "-"), func(t *testing.T) {
			f := optionCFixture(t, false)
			f.args = append(f.args, flags...)
			code, _, stderr := f.run()
			if code != 0 {
				t.Fatalf("launch exit=%d", code)
			}
			p := adapterRecord(t, stderr)
			if !bytes.Contains(stderr, []byte(p.OperatorText()+"\n")) {
				t.Fatal("native output flag suppressed OperatorText")
			}
		})
	}
}

func setSensitiveProfile(t *testing.T, f *pipelineFixture) {
	t.Helper()
	doc := directNetworkCatalogTOML + "sensitive_egress = true\n"
	file, e := netprofile.Parse([]byte(doc))
	if e != nil {
		t.Fatal(e)
	}
	profile, _ := file.Lookup("direct-a")
	writeFixture(t, filepath.Join(f.dir, ".curator", "network.toml"), []byte(doc), 0600)
	ledger, _ := json.Marshal(map[string]any{"schema": "relux-network-confirmations-v1", "confirmed": map[string]any{"direct-a": map[string]any{"digest": netprofile.Digest(profile), "confirmed_at": "2026-10-02T00:00:00Z"}}})
	writeFixture(t, filepath.Join(f.dir, ".curator", "network.confirmations.json"), ledger, 0600)
}
func expectPolicyRefusal(t *testing.T, f *pipelineFixture, reason string) {
	t.Helper()
	code, out, stderr := f.run()
	if code != 1 || len(out) != 0 || !bytes.Contains(stderr, []byte("network_scope_unsupported")) || !bytes.Contains(stderr, []byte(reason)) {
		t.Fatalf("expected typed %s refusal; exit=%d", reason, code)
	}
	f.assertNoChild(t)
}
func TestOptionCSensitiveEgressCannotDowngrade(t *testing.T) {
	for _, mode := range []string{"optimistic", "pinned"} {
		t.Run(mode, func(t *testing.T) {
			f := optionCFixture(t, false)
			setSensitiveProfile(t, f)
			flag := mode
			if mode == "pinned" {
				flag += ":" + strings.TrimPrefix(artifactBuild(t, f), "sha256-")
			}
			insertLauncherArgs(f, "--network-policy", flag)
			expectPolicyRefusal(t, f, "strict_miss")
		})
	}
}
func TestOptionCEmptyAllowlistReason(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		t.Run(map[bool]string{false: "strict", true: "sensitive"}[sensitive], func(t *testing.T) {
			f := optionCFixture(t, false)
			if sensitive {
				setSensitiveProfile(t, f)
			} else {
				insertLauncherArgs(f, "--network-policy", "strict")
			}
			expectPolicyRefusal(t, f, "no qualified builds until central qualification (A) ships")
		})
	}
}
func TestOptionCLegacyLabelsRetired(t *testing.T) {
	for _, release := range []string{"2.1.287", "2.1.288"} {
		t.Run(release, func(t *testing.T) {
			f := optionCFixture(t, false)
			withTestRelease(f, release)
			code, _, stderr := f.run()
			if code != 0 {
				t.Fatalf("launch exit=%d", code)
			}
			adapterRecord(t, stderr)
			strict := optionCFixture(t, false)
			withTestRelease(strict, release)
			insertLauncherArgs(strict, "--network-policy", "strict")
			expectPolicyRefusal(t, strict, "strict_miss")
		})
	}
}
func TestOptionCPinnedUnprovenBuild(t *testing.T) {
	f := optionCFixture(t, false)
	insertLauncherArgs(f, "--network-policy", "pinned:"+strings.TrimPrefix(artifactBuild(t, f), "sha256-"))
	code, _, stderr := f.run()
	if code != 0 {
		t.Fatalf("launch exit=%d", code)
	}
	adapterRecord(t, stderr)
}
func TestOptionCPinnedMismatch(t *testing.T) {
	f := optionCFixture(t, false)
	insertLauncherArgs(f, "--network-policy", "pinned:"+strings.Repeat("a", 64))
	expectPolicyRefusal(t, f, "pinned_miss")
}
func TestOptionCKnownBadBuild(t *testing.T) {
	f := optionCFixture(t, false)
	data, _ := json.Marshal(map[string]any{"schema": adapterprobe.KnownBadSchema, "sha256": []string{strings.TrimPrefix(artifactBuild(t, f), "sha256-")}})
	writeFixture(t, filepath.Join(f.dir, ".curator", adapterprobe.KnownBadFileName), data, 0600)
	expectPolicyRefusal(t, f, "known_bad_build")
}
func TestOptionCKnownBadUnreadable(t *testing.T) {
	f := optionCFixture(t, false)
	// A directory is a present-but-unreadable file even under privileged tests.
	if err := os.Mkdir(filepath.Join(f.dir, ".curator", adapterprobe.KnownBadFileName), 0700); err != nil {
		t.Fatal(err)
	}
	expectPolicyRefusal(t, f, "knownbad_unreadable")
}
func TestOptionCExplicitKnownBadMissing(t *testing.T) {
	f := optionCFixture(t, false)
	insertLauncherArgs(f, "--network-known-bad", filepath.Join(f.dir, "missing.json"))
	expectPolicyRefusal(t, f, "knownbad_unreadable")
}
func TestOptionCUnknownVendor(t *testing.T) {
	f := entryFixture(t, "pi", false)
	writeDirectNetworkCatalog(t, f.dir, true)
	f.args = withNetworkFlag(f.args, "direct-a")
	expectPolicyRefusal(t, f, "vendor_line_unsupported")
}
func TestOptionCUnknownAdapter(t *testing.T) {
	f := optionCFixture(t, false)
	f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
		r.Adapter.Adapter = "unknown-adapter"
		return adapterprobe.Evaluate(c, r, p)
	}
	expectPolicyRefusal(t, f, "adapter_unsupported")
}
func TestOptionCQualifiedDecision(t *testing.T) {
	// Future qualification input at the API seam; production has no AllowedBuild.
	f := optionCFixture(t, false)
	f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
		p.Allowlist = []adapterprobe.AllowedBuild{{Adapter: r.Adapter, BuildID: artifactBuild(t, f), Recipe: r.Recipe, Platform: runtime.GOOS, Scope: adapterprobe.TransportScope}}
		return adapterprobe.Evaluate(c, r, p)
	}
	code, _, stderr := f.run()
	if code != 0 || bytes.Contains(stderr, []byte("adapter-provenance:")) {
		t.Fatal("qualified decision did not launch without unqualified provenance")
	}
}
func TestOptionCScopeGate(t *testing.T) {
	f := optionCFixture(t, false)
	// Exercise a real known line and registered exec recipe outside the host's
	// child-scope ceiling, through Prepare. No unknown recipe masks this gate.
	data, e := os.ReadFile(f.binary)
	if e != nil {
		t.Fatal(e)
	}
	req := network.Request{Explicit: "direct-a", ExplicitSet: true, ParentEnv: []string{"HOME=" + f.dir}, Artifact: data, Harness: "codex-cli", Entrypoint: "exec", ScopeCheck: f.deps.networkScopeCheck}
	req.HostIdentity.Adapter = "generic-env-v1"
	req.HostIdentity.Harness = "codex-cli"
	req.HostIdentity.Entrypoint = "exec"
	req.HostIdentity.Build = artifactBuild(t, f)
	if _, e := network.Prepare(context.Background(), req); e == nil {
		t.Fatal("codex exec admitted beyond child-scope ceiling")
	}
}

func TestOptionCMutants(t *testing.T) {
	mutants := []struct{ id, test string }{
		{"known-bad-member-admitted", "TestOptionCKnownBadBuild"},
		{"pin-mismatch-admitted", "TestOptionCPinnedMismatch"},
		{"strict-miss-admitted", "TestOptionCLegacyLabelsRetired"},
		{"host-build-mismatch-admitted", "TestOptionCHostIdentityMismatch"},
		{"OperatorText-not-shown", "TestOptionCOperatorTextVerbatim"},
		{"provenance-not-stored", "TestOptionCProvenanceRecordForSessionWrapper"},
		{"sensitive-egress-downgrade", "TestOptionCSensitiveEgressCannotDowngrade"},
		{"KnownBadErr-dropped", "TestOptionCKnownBadUnreadable"},
		{"scope-gate-removed", "TestOptionCScopeGate"},
		{"tracked-session-admitted", "TestOptionCSessionRecordGuard"},
	}
	for _, m := range mutants {
		t.Run(m.id, func(t *testing.T) {
			cmd := exec.Command("go", "test", ".", "-run", "^"+m.test+"$", "-count=1", "-v")
			for _, e := range os.Environ() {
				if !strings.HasPrefix(e, "OPTION_C_MUTANT=") {
					cmd.Env = append(cmd.Env, e)
				}
			}
			cmd.Env = append(cmd.Env, "OPTION_C_MUTANT="+m.id)
			output, e := cmd.CombinedOutput()
			dir := filepath.Join("..", "..", ".temp", "TASK-261005-yoogtw", "mutants")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, m.id+".log"), output, 0600); err != nil {
				t.Fatal(err)
			}
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 1 || !bytes.Contains(output, []byte("--- FAIL: "+m.test)) {
				t.Fatalf("SURVIVED %s: no named failing test", m.id)
			}
			t.Logf("KILLED %s; %s fails; actual mutant command exit=1 (expected red)", m.id, m.test)
		})
	}
}

func TestOptionCHostIdentityMismatch(t *testing.T) {
	f := optionCFixture(t, false)
	f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
		d, e := adapterprobe.Evaluate(c, r, p)
		d.BinarySHA256 = strings.Repeat("a", 64)
		d.BuildID = "sha256-" + d.BinarySHA256
		d.Provenance.BinarySHA256 = d.BinarySHA256
		d.Provenance.BuildID = d.BuildID
		return d, e
	}
	expectPolicyRefusal(t, f, "decision identity differs from host tuple")
}

type failAdapterWriter struct{}

func (failAdapterWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("Unqualified build:")) || bytes.Contains(p, []byte("adapter-provenance:")) {
		return 0, errors.New("output unavailable")
	}
	return len(p), nil
}
func TestOptionCProvenanceWriteFailureRefuses(t *testing.T) {
	f := optionCFixture(t, false)
	var out bytes.Buffer
	code := run(context.Background(), f.args, &out, failAdapterWriter{}, f.deps)
	if code != 1 || out.Len() != 0 {
		t.Fatal("launch admitted despite missing mandatory operator provenance")
	}
	f.assertNoChild(t)
}
func TestOptionCSuppressionFlagsAreNotLauncherFlags(t *testing.T) {
	for _, flag := range []string{"--quiet", "--silent", "--json-only"} {
		f := optionCFixture(t, false)
		insertLauncherArgs(f, flag)
		code, out, _ := f.run()
		if code != 2 || len(out) != 0 || f.builds != 0 {
			t.Fatal("launcher accepted a provenance suppression flag")
		}
	}
}

type networkProbeFunc func(context.Context, probe.Request) (probe.Result, error)

func (fn networkProbeFunc) Probe(c context.Context, r probe.Request) (probe.Result, error) {
	return fn(c, r)
}
func TestOptionCSnapshotExecutesAdmittedBytes(t *testing.T) {
	f := optionCFixture(t, false)
	endpoint, _ := writeNetworkCatalog(t, f.dir, true)
	for i, a := range f.args {
		if a == "direct-a" {
			f.args[i] = "egress-a"
		}
	}
	build := artifactBuild(t, f)
	f.deps.prober = networkProbeFunc(func(_ context.Context, _ probe.Request) (probe.Result, error) {
		writeFixture(t, f.binary, []byte("#!/bin/sh\nexit 79\n"), 0700)
		return okProbe(endpoint), nil
	})
	code, out, stderr := f.run()
	if code != 0 || len(out) == 0 || adapterRecord(t, stderr).BuildID != build {
		t.Fatal("execution escaped the admitted native snapshot after source replacement")
	}
}
func TestOptionCScriptsRefuse(t *testing.T) {
	f := optionCFixture(t, false)
	inner := f.deps.networkEvaluate
	f.deps.networkEvaluate = func(c context.Context, r adapterprobe.Request, p adapterprobe.Policy) (adapterprobe.Decision, error) {
		r.Artifact = []byte("#!/bin/sh\nexit 0\n")
		if inner != nil {
			return inner(c, r, p)
		}
		return adapterprobe.Evaluate(c, r, p)
	}
	expectPolicyRefusal(t, f, "runtime_closure_unsupported")
}
