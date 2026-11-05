package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/protocolchild"
)

type protocolMetadataResolver struct {
	resolve func(context.Context, fragment.Request) (*fragment.Fragment, error)
}

func (r protocolMetadataResolver) Resolve(ctx context.Context, req fragment.Request) (*fragment.Fragment, error) {
	return r.resolve(ctx, req)
}

func protocolMetadataFixture(t *testing.T) ([]string, []string, *fragment.Fragment, string) {
	t.Helper()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f, err := fragment.Parse([]byte(fragmentLineFor(fragment.EnvCodexCLI)))
	if err != nil {
		t.Fatal(err)
	}
	args := []string{protocolchild.Command, "--runtime=codex", "--kind=app-server", "--profile=" + f.Profile.Name,
		"--expected-lock-sha256=" + f.Profile.LockSHA256, "--expected-fragment-sha256=" + strings.TrimPrefix(f.Digest, fragment.DigestPrefix),
		"--workspace=" + workspace, `--model-json="gpt-6-astra"`, `--effort-json="low"`, "--permissions=native",
		"--service-tier-json=null", "--weights-json=null", "--credential-source-json=null", "--child-env-json={}", "--native-tail-json=[]",
		"--operation-id=12345678-1234-4123-8123-123456789abc", "--lifecycle=create", "--member-a2a-agent-json=null", "--member-a2a-config-dir-json=null"}
	return args, []string{"HOME=" + workspace, "PATH=" + workspace}, f, workspace
}

// Metadata preparation uses exactly one nonrepair Core request in the selected
// physical workspace with the checked baseline. Member selectors never reach
// that subprocess. No actual Core/native/provider process runs in this fixture.
func TestProtocolMetadataNonrepairRequest(t *testing.T) {
	args, baseline, f, workspace := protocolMetadataFixture(t)
	args[17] = `--member-a2a-agent-json="fixture.member"`
	args[18] = `--member-a2a-config-dir-json="` + workspace + `"`
	calls := 0
	deps := launchDeps{workdir: func() (string, error) { return workspace, nil }, environ: func() []string { return baseline }}
	deps.resolver = protocolMetadataResolver{resolve: func(_ context.Context, req fragment.Request) (*fragment.Fragment, error) {
		calls++
		wantArgv := []string{"env", "resolve", "codex_cli", "--profile", f.Profile.Name, "--format", "json"}
		if !req.VerifyOnly || req.Dir != workspace || !reflect.DeepEqual(req.Argv(), wantArgv) || !reflect.DeepEqual(req.Env, baseline) {
			t.Fatalf("metadata request changed: %+v argv=%v", req, req.Argv())
		}
		if req.Stderr != io.Discard {
			t.Fatal("raw Core diagnostics could escape the sanitized boundary")
		}
		return f, nil
	}}
	got, err := resolveProtocolMetadata(context.Background(), args, deps)
	if err != nil || got == nil || calls != 1 || got.fragment != f || got.workspace != workspace {
		t.Fatalf("valid metadata preparation refused: got=%+v err=%v calls=%d", got, err, calls)
	}
	baseline[0] = "HOME=/fixture/changed"
	if got.baseline[0] == baseline[0] {
		t.Fatal("metadata snapshot aliases caller baseline")
	}
}

// Closed-grammar, contaminated-baseline, cancellation and workspace failures
// stop before Core metadata. Adjacent valid preparation is covered above.
func TestProtocolMetadataEarlyRefusals(t *testing.T) {
	for _, kind := range []string{"grammar", "foreign-home", "foreign-identity", "cancelled", "workspace-mismatch", "workspace-unavailable"} {
		t.Run(kind, func(t *testing.T) {
			args, baseline, _, workspace := protocolMetadataFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wantCode := "protocol_child_stale"
			switch kind {
			case "grammar":
				args = append(args, "--unexpected=true")
				wantCode = protocolchild.CodeUsage
			case "foreign-home":
				baseline = append(baseline, "CODEX_HOME=/fixture/foreign")
				wantCode = protocolchild.CodeEnvironmentRefused
			case "foreign-identity":
				baseline = append(baseline, "A2A_AGENT=fixture.other")
				wantCode = protocolchild.CodeEnvironmentRefused
			case "cancelled":
				cancel()
				wantCode = "protocol_child_cancelled"
			case "workspace-mismatch":
				workspace = t.TempDir()
			}
			deps := launchDeps{environ: func() []string { return baseline }, workdir: func() (string, error) {
				if kind == "workspace-unavailable" {
					return "", errors.New("private-fixture-detail")
				}
				return workspace, nil
			}, resolver: protocolMetadataResolver{resolve: func(context.Context, fragment.Request) (*fragment.Fragment, error) {
				t.Fatal("refused input reached Core metadata")
				return nil, nil
			}}}
			got, err := resolveProtocolMetadata(ctx, args, deps)
			assertProtocolMetadataRefusal(t, got, err, wantCode)
		})
	}
}

// Each retained profile/lock/fragment expectation is checked independently.
// A stale or failed Core result never repairs, retries or yields partial inputs.
func TestProtocolMetadataResolvedRefusals(t *testing.T) {
	for _, kind := range []string{"profile", "lock", "fragment", "core-stale", "core-failed", "nil-fragment", "cancel-after-resolve"} {
		t.Run(kind, func(t *testing.T) {
			args, baseline, f, workspace := protocolMetadataFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wantCode := "protocol_child_stale"
			calls := 0
			deps := launchDeps{environ: func() []string { return baseline }, workdir: func() (string, error) { return workspace, nil }}
			deps.resolver = protocolMetadataResolver{resolve: func(context.Context, fragment.Request) (*fragment.Fragment, error) {
				calls++
				switch kind {
				case "profile":
					f.Profile.Name = "different"
				case "lock":
					f.Profile.LockSHA256 = strings.Repeat("a", 64)
				case "fragment":
					f.Digest = fragment.DigestPrefix + strings.Repeat("b", 64)
				case "core-stale":
					return nil, &fragment.ResolveError{Code: fragment.CodeHomeStale, Detail: "private-fixture-detail"}
				case "core-failed":
					wantCode = "protocol_child_resolve_refused"
					return nil, errors.New("private-fixture-detail")
				case "nil-fragment":
					return nil, nil
				case "cancel-after-resolve":
					cancel()
					wantCode = "protocol_child_cancelled"
				}
				return f, nil
			}}
			got, err := resolveProtocolMetadata(ctx, args, deps)
			assertProtocolMetadataRefusal(t, got, err, wantCode)
			if calls != 1 {
				t.Fatalf("metadata requests=%d want exactly 1", calls)
			}
		})
	}
}

func assertProtocolMetadataRefusal(t *testing.T, got *protocolMetadata, err error, code string) {
	t.Helper()
	var refusal *protocolchild.Error
	if got != nil || !errors.As(err, &refusal) || refusal.Code != code {
		t.Fatalf("got=%+v err=%v; want %s without partial metadata", got, err, code)
	}
	if strings.Contains(err.Error(), "private-fixture-detail") || strings.Contains(err.Error(), "/fixture/foreign") || strings.Contains(err.Error(), "fixture.other") {
		t.Fatal("metadata refusal leaked input or Core error detail")
	}
}
