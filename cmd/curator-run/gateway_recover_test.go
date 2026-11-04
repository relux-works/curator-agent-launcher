package main

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/recoveryjournal"
)

// Read only the public selector an operator receives, never the private journal
// or the service fixture. Repeated diagnostics must name the same attempt.
func publicRecoveryAttempt(t *testing.T, diagnostic []byte) string {
	t.Helper()
	var attempt string
	for _, field := range strings.Fields(string(diagnostic)) {
		if !strings.HasPrefix(field, "attempt=") {
			continue
		}
		id := strings.TrimPrefix(field, "attempt=")
		if raw, err := hex.DecodeString(id); err != nil || len(raw) != 32 || hex.EncodeToString(raw) != id || (attempt != "" && attempt != id) {
			t.Fatal("RECOVERY SELECTOR ALARM: invalid or ambiguous public attempt")
		}
		attempt = id
	}
	if attempt == "" {
		t.Fatal("RECOVERY SELECTOR ALARM: failed operation omitted public attempt")
	}
	return attempt
}

func forbiddenGatewayDeps(t *testing.T) launchDeps {
	t.Helper()
	return launchDeps{configLocations: func() (configfile.Locations, error) {
		t.Error("GATEWAY DISPATCH ALARM: ordinary config callback reached")
		return configfile.Locations{}, errors.New("forbidden")
	}, workdir: func() (string, error) {
		t.Error("GATEWAY DISPATCH ALARM: workdir callback reached")
		return "", errors.New("forbidden")
	}, environ: func() []string { t.Error("GATEWAY EFFECT ALARM: invalid/help command read environment"); return nil }, gatewayRecover: func(context.Context, []string, string) (gatewayRecoveryResult, error) {
		t.Error("GATEWAY EFFECT ALARM: invalid/help command attempted recovery")
		return gatewayRecoveryResult{}, errors.New("forbidden")
	}}
}

// Both static help and invalid grammar precede all ordinary config/fragment/
// manager/native routing and the exact recovery/environment boundaries.
func TestGatewayRecoveryEarlyHelpAndRefusal(t *testing.T) {
	attempt := strings.Repeat("a", 64)
	for _, args := range [][]string{{"gateway", "--help"}, {"gateway", "recover", "--proof", "PRIVATE_SENTINEL", "--help"}, {"gateway"}, {"gateway", "recover", "--attempt", attempt, "--proof", "PRIVATE_SENTINEL"}, {"gateway", "recover", "--attempt", "PRIVATE_SENTINEL"}, {"gateway", "list"}} {
		var out, errOut strings.Builder
		got := run(context.Background(), args, &out, &errOut, forbiddenGatewayDeps(t))
		_, help := cli.TopicHelp(args)
		if help {
			if got != 0 || out.String() != cli.GatewayRecoveryHelp || errOut.Len() != 0 {
				t.Fatal("static help did not bypass broken configuration")
			}
		} else if got != 2 || out.Len() != 0 || !strings.Contains(errOut.String(), "curator-run: usage:") {
			t.Fatal("invalid command did not produce usage refusal")
		}
		if strings.Contains(out.String()+errOut.String(), "PRIVATE_SENTINEL") {
			t.Fatal("gateway grammar/help leaked input")
		}
	}
}

// Exact dispatch copies the captured original inputs, carries one bounded
// context and attempt, and never consults normal launch configuration.
func TestGatewayRecoveryExactDispatch(t *testing.T) {
	for _, outcome := range []string{"revoked", "cancelled_before_issue"} {
		t.Run(outcome, func(t *testing.T) {
			deps := forbiddenGatewayDeps(t)
			env := []string{"HOME=/original", "XDG_STATE_HOME=/original/state"}
			calls, envCalls := 0, 0
			deps.environ = func() []string { envCalls++; return env }
			attempt := strings.Repeat("a", 64)
			deps.gatewayRecover = func(ctx context.Context, parent []string, id string) (gatewayRecoveryResult, error) {
				calls++
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 30*time.Second || id != attempt || !reflect.DeepEqual(parent, env) {
					t.Error("wrong original recovery inputs/budget")
				}
				parent[0] = "changed-copy"
				return gatewayRecoveryResult{true, outcome, true}, nil
			}
			var out, errOut strings.Builder
			got := run(context.Background(), []string{"gateway", "recover", "--attempt", attempt}, &out, &errOut, deps)
			if got != 0 || calls != 1 || envCalls != 1 || env[0] != "HOME=/original" || out.String() != "attempt="+attempt+" service_result="+outcome+" receipt=committed\n" || errOut.Len() != 0 {
				t.Fatal("exact recovery did not use sole early boundary")
			}
		})
	}
}

// Only service confirmation AND committed local evidence yield success. Fixed
// diagnostics preserve their distinction and never print arbitrary backend text.
func TestGatewayRecoveryOutcomeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    gatewayRecoveryResult
		err       error
		code      string
		confirmed bool
	}{
		{"unknown", gatewayRecoveryResult{}, recoveryjournal.ErrRecoveryUnknown, recoveryjournal.CodeUnconfirmed, false},
		{"state", gatewayRecoveryResult{}, recoveryjournal.ErrRead, recoveryjournal.CodeStateInvalid, false},
		{"local", gatewayRecoveryResult{true, "revoked", false}, recoveryjournal.ErrLocalEvidence, recoveryjournal.CodeLocalEvidence, true},
		{"missing-pointer", gatewayRecoveryResult{true, "revoked", false}, nil, recoveryjournal.CodeLocalEvidence, true},
		{"forged-pointer", gatewayRecoveryResult{false, "revoked", true}, nil, recoveryjournal.CodeUnconfirmed, false},
		{"bad-outcome", gatewayRecoveryResult{true, "PRIVATE_SENTINEL", true}, nil, recoveryjournal.CodeUnconfirmed, false},
		{"raw-error", gatewayRecoveryResult{}, errors.New("PRIVATE_SENTINEL token=/private"), recoveryjournal.CodeUnconfirmed, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := forbiddenGatewayDeps(t)
			deps.environ = func() []string { return []string{"HOME=/fixture"} }
			deps.gatewayRecover = func(context.Context, []string, string) (gatewayRecoveryResult, error) { return tc.result, tc.err }
			var out, errOut strings.Builder
			got := run(context.Background(), []string{"gateway", "recover", "--attempt", strings.Repeat("a", 64)}, &out, &errOut, deps)
			if got != 1 || out.Len() != 0 || !strings.HasPrefix(errOut.String(), "curator-run: "+tc.code+":") || strings.Contains(errOut.String(), "PRIVATE_SENTINEL") || strings.Contains(errOut.String(), "/private") {
				t.Fatal("GATEWAY OUTCOME ALARM: uncertified recovery succeeded or leaked")
			}
			if strings.Contains(errOut.String(), "service confirmed") != tc.confirmed {
				t.Fatal("service observation confused with local persistence")
			}
			if publicRecoveryAttempt(t, []byte(errOut.String())) != strings.Repeat("a", 64) {
				t.Fatal("failure diagnostic changed the selected public attempt")
			}
		})
	}
}

type recoveryBrokenWriter struct{ short bool }

func (w recoveryBrokenWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) / 2, nil
	}
	return 0, io.ErrClosedPipe
}

// Lost/short result output is a nonzero local-evidence failure even when the
// underlying service and durable journal operations have already succeeded.
func TestGatewayRecoveryOutputFailure(t *testing.T) {
	for _, short := range []bool{false, true} {
		deps := forbiddenGatewayDeps(t)
		deps.environ = func() []string { return nil }
		deps.gatewayRecover = func(context.Context, []string, string) (gatewayRecoveryResult, error) {
			return gatewayRecoveryResult{true, "revoked", true}, nil
		}
		var errOut strings.Builder
		if got := run(context.Background(), []string{"gateway", "recover", "--attempt", strings.Repeat("a", 64)}, recoveryBrokenWriter{short}, &errOut, deps); got != 1 || !strings.Contains(errOut.String(), recoveryjournal.CodeLocalEvidence) {
			t.Fatal("lost output certified command success")
		}
		if publicRecoveryAttempt(t, []byte(errOut.String())) != strings.Repeat("a", 64) {
			t.Fatal("lost output omitted the original recovery selector")
		}
	}
}
