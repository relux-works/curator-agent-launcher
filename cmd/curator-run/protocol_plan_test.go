package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/plan"
	"github.com/relux-works/curator-agent-launcher/internal/protocolchild"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"github.com/relux-works/skill-agents-management/pkg/providerlimits"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

func protocolPlanFixture(t *testing.T) (*pipelineFixture, []string) {
	t.Helper()
	f := entryFixture(t, fragment.EnvCodexCLI)
	f.usePermissionsFragment(t, "native", false, "profile")
	parsed, err := fragment.Parse([]byte(f.resolver.stdout))
	if err != nil {
		t.Fatal(err)
	}
	args, _, _, _ := protocolMetadataFixture(t)
	args[3] = "--profile=" + parsed.Profile.Name
	args[4] = "--expected-lock-sha256=" + parsed.Profile.LockSHA256
	args[5] = "--expected-fragment-sha256=" + strings.TrimPrefix(parsed.Digest, fragment.DigestPrefix)
	args[6] = "--workspace=" + f.dir
	args[8] = `--effort-json="medium"`
	f.deps.environ = func() []string { return []string{"HOME=" + f.dir, "PATH=" + f.dir} }
	f.deps.resolver = protocolMetadataResolver{resolve: func(_ context.Context, req fragment.Request) (*fragment.Fragment, error) {
		if !req.VerifyOnly || req.Dir != f.dir {
			t.Fatal("provider preparation lost the nonrepair workspace boundary")
		}
		return parsed, nil
	}}
	// The version fixture uses a neighboring file, never an extra baseline env
	// name. Provider planning still invokes the pinned owning version parser.
	writeFixture(t, filepath.Join(f.dir, "codex-version"), []byte("codex-cli 0.153.2\n"), 0600)
	f.writeDefaults(t, f.deps.defaults.Machine, `{"schema":"curator-run-defaults-v3","defaults":{"codex_cli":{"model":"gpt-6-astra","effort":"medium"}}}`)
	return f, args
}

func protocolPolicyFixture(t *testing.T, f *pipelineFixture, args []string) *protocolPolicy {
	t.Helper()
	meta, err := resolveProtocolMetadata(context.Background(), args, f.deps)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := resolveProtocolPolicy(context.Background(), meta, f.deps)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

// TMPDIR is optional in the closed entry contract. Both its absence and an
// explicit fixture directory must reach owning finalization without invented
// environment entries. This exercises the real pinned provider, not native exec.
func TestProtocolPlanOptionalTempDir(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "present"}[present], func(t *testing.T) {
			f, args := protocolPlanFixture(t)
			t.Cleanup(func() { f.assertNoChild(t) })
			if present {
				baseline := f.deps.environ()
				f.deps.environ = func() []string {
					return append(append([]string{}, baseline...), "TMPDIR="+f.dir)
				}
			}
			policy := protocolPolicyFixture(t, f, args)
			got, err := composeProtocolPlan(context.Background(), policy)
			if err != nil || got == nil {
				// Report only stage counts and a typed contract fact. No raw plan,
				// environment, private diagnostic or native config enters output.
				basisChecked := f.builds == 1 && f.verdicts == 1
				missingBasis := false
				if basisChecked {
					_, basisErr := agentic.FinalizePlan(f.plan, agentic.FinalizeOverlays{}, nil)
					missingBasis = errors.Is(basisErr, agentic.ErrFinalizeNoSealBasis)
				}
				t.Fatalf("valid optional-TMPDIR plan refused: builds=%d limits=%d basis-checked=%t missing-finalize-basis=%t refusal=%v", f.builds, f.verdicts, basisChecked, missingBasis, err)
			}
			if f.builds != 1 || f.verdicts != 1 {
				t.Fatal("optional baseline caused repeated provider admission")
			}
			if err := got.sealed.VerifyBeforeExec(); err != nil {
				t.Fatal("valid optional-TMPDIR finalized process refused")
			}
			seen := 0
			for _, entry := range got.value.Env {
				if strings.HasPrefix(entry, "TMPDIR=") {
					seen++
					if !present || entry != "TMPDIR="+f.dir {
						t.Fatal("optional TMPDIR was invented or changed")
					}
				}
			}
			if present && seen != 1 || !present && seen != 0 {
				t.Fatal("optional TMPDIR presence changed during composition")
			}
		})
	}
}

// The owning vendor admission sees app-server BEFORE building, and ordinary
// composition consumes that same suffix exactly once. Synthetic version/limits
// fixtures start no runtime; preparation is not a final boundary or admission.
func TestProtocolPlanOwningComposition(t *testing.T) {
	for _, member := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-member", true: "member"}[member], func(t *testing.T) {
			f, args := protocolPlanFixture(t)
			if member {
				args[17] = `--member-a2a-agent-json="fixture.member"`
				args[18] = `--member-a2a-config-dir-json="` + f.dir + `"`
			}
			policy := protocolPolicyFixture(t, f, args)
			got, err := composeProtocolPlan(context.Background(), policy)
			if err != nil || got == nil {
				t.Fatalf("valid owning plan refused: %v", err)
			}
			if f.builds != 1 || f.verdicts != 1 || !reflect.DeepEqual(f.request.NativeArgs, []string{"app-server"}) || f.request.PermissionMode != agentic.PermissionModeNative || f.request.ToolRelease != "0.153.2" || !reflect.DeepEqual(f.request.Env, policy.metadata.baseline) {
				t.Fatalf("owning admission request changed: builds=%d verdicts=%d request=%+v", f.builds, f.verdicts, f.request)
			}
			if got.value.WorkDir != f.dir || got.value.RawStdin.Attached || len(got.value.Argv) == 0 || got.value.Argv[len(got.value.Argv)-1] != "app-server" {
				t.Fatalf("protocol plan shape changed: %+v", got.value)
			}
			count := 0
			for _, arg := range got.value.Argv {
				if arg == "app-server" {
					count++
				}
			}
			if count != 1 || policy.metadata.selectors.CheckMemberEnvironment(got.value.Env) != nil {
				t.Fatal("suffix/member composition changed")
			}
			if err := got.sealed.VerifyBeforeExec(); err != nil {
				t.Fatalf("valid finalized process refused: %v", err)
			}
			if !reflect.DeepEqual(got.value.Env, got.sealed.Env) {
				t.Fatal("execution environment differs from the finalized ordered environment")
			}
			changed := got.sealed
			changed.Env = append(append([]string{}, changed.Env...), "UNADMITTED=changed")
			if !errors.Is(changed.VerifyBeforeExec(), agentic.ErrFinalizedProcessChanged) {
				t.Fatal("finalized plan admitted a changed process")
			}
			f.assertNoChild(t)
		})
	}
}

// Expectations never become permission flags or bypass the profile lock.
// Provider/build/limit refusals preserve a nil result and fixed diagnostics.
// Nearby valid admission/composition is exercised above through the real module.
func TestProtocolPlanRefusals(t *testing.T) {
	for _, kind := range []string{"permission-mismatch", "unknown-yolo-release", "provider-refusal", "limits-unavailable", "limits-unknown", "attached-stdin", "suffix-drift", "foreign-member-overlay", "foreign-member-lookup", "member-link", "cancel-before", "cancel-after-build"} {
		t.Run(kind, func(t *testing.T) {
			f, args := protocolPlanFixture(t)
			code := "protocol_child_plan_refused"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "permission-mismatch":
				args[9] = "--permissions=yolo"
				code = "protocol_child_permission_mismatch"
			case "unknown-yolo-release":
				f.usePermissionsFragment(t, "yolo", false, "profile")
				parsed, err := fragment.Parse([]byte(f.resolver.stdout))
				if err != nil {
					t.Fatal(err)
				}
				f.deps.resolver = protocolMetadataResolver{resolve: func(context.Context, fragment.Request) (*fragment.Fragment, error) { return parsed, nil }}
				args[5] = "--expected-fragment-sha256=" + strings.TrimPrefix(parsed.Digest, fragment.DigestPrefix)
				args[9] = "--permissions=yolo"
				writeFixture(t, filepath.Join(f.dir, "codex-version"), []byte("private-probe-fixture\n"), 0600)
				code = "protocol_child_permission_refused"
			case "limits-unavailable":
				f.deps.availability = func() (plan.AvailabilityFunc, error) { return nil, errors.New("private-limit-fixture") }
			case "limits-unknown":
				f.deps.availability = func() (plan.AvailabilityFunc, error) {
					return func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) {
						return vendorplugin.Availability{}, errors.New("private-limit-fixture")
					}, nil
				}
			case "member-link":
				link := filepath.Join(f.dir, "member-link")
				if err := os.Symlink(f.dir, link); err != nil {
					t.Fatal(err)
				}
				args[17] = `--member-a2a-agent-json="fixture.member"`
				args[18] = `--member-a2a-config-dir-json="` + link + `"`
				code = protocolchild.CodeEnvironmentRefused
			}
			original := f.deps.build
			f.deps.build = func(ctx context.Context, r *vendorplugin.Registry, req vendorplugin.SpawnRequest, mode agentic.LaunchMode) (agentic.PlanWithEnvironment, error) {
				if kind == "provider-refusal" {
					return agentic.PlanWithEnvironment{}, errors.New("private-provider-fixture")
				}
				built, err := original(ctx, r, req, mode)
				if err != nil {
					return built, err
				}
				switch kind {
				case "attached-stdin":
					built.Plan.Stdin.Attached = true
				case "suffix-drift":
					built.Plan.Argv[len(built.Plan.Argv)-1] = "unadmitted-suffix"
				case "foreign-member-overlay":
					built.Plan.Env = append(built.Plan.Env, "A2A_AGENT=fixture.foreign")
				case "cancel-after-build":
					cancel()
				}
				return built, nil
			}
			policy := protocolPolicyFixture(t, f, args)
			if kind == "foreign-member-lookup" {
				policy.metadata.fragment.MCP.EnvNames = append(policy.metadata.fragment.MCP.EnvNames, "A2A_AGENT")
			}
			if kind == "foreign-member-overlay" || kind == "foreign-member-lookup" {
				code = protocolchild.CodeEnvironmentRefused
			}
			if kind == "cancel-before" {
				cancel()
			}
			if strings.HasPrefix(kind, "cancel-") {
				code = "protocol_child_cancelled"
			}
			got, err := composeProtocolPlan(ctx, policy)
			var refusal *protocolchild.Error
			if got != nil || !errors.As(err, &refusal) || refusal.Code != code || strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), "fixture.foreign") {
				t.Fatalf("wrong/unsanitized refusal: got=%+v err=%v want=%s", got, err, code)
			}
			if (kind == "permission-mismatch" || kind == "unknown-yolo-release" || kind == "member-link" || kind == "cancel-before") && f.builds != 0 {
				t.Fatal("early refusal reached provider admission")
			}
			f.assertNoChild(t)
		})
	}
}
