package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/diagnostics"
	"github.com/relux-works/curator-agent-launcher/internal/recoveryjournal"
)

const gatewayRecoveryBudget = 30 * time.Second

type gatewayRecoveryResult struct {
	confirmed bool
	outcome   string
	committed bool
}
type gatewayRecoverFunc func(context.Context, []string, string) (gatewayRecoveryResult, error)

func runGatewayRecovery(ctx context.Context, args []string, stdout, stderr io.Writer, deps launchDeps) int {
	inv, err := cli.Parse(args)
	if err != nil || inv.GatewayRecovery == nil {
		_ = diagnostics.Emit(stderr, diagnostics.CodeUsage, "expected gateway recover --attempt <64 lowercase hex>")
		fmt.Fprint(stderr, cli.GatewayRecoveryHelp)
		return diagnostics.ExitUsage
	}
	getEnv := deps.environ
	if getEnv == nil {
		getEnv = os.Environ
	}
	parent := append([]string(nil), getEnv()...)
	recover := deps.gatewayRecover
	if recover == nil {
		recover = recoverOriginalGateway
	}
	bounded, cancel := context.WithTimeout(ctx, gatewayRecoveryBudget)
	defer cancel()
	result, err := recover(bounded, parent, inv.GatewayRecovery.Attempt)
	validOutcome := result.outcome == "revoked" || result.outcome == "cancelled_before_issue"
	if err == nil && result.confirmed && result.committed && validOutcome {
		line := fmt.Sprintf("attempt=%s service_result=%s receipt=committed\n", inv.GatewayRecovery.Attempt, result.outcome)
		if written, err := io.WriteString(stdout, line); err == nil && written == len(line) {
			return 0
		}
		err = recoveryjournal.ErrLocalEvidence
	}
	if err == nil {
		if result.confirmed && validOutcome {
			err = recoveryjournal.ErrLocalEvidence
		} else {
			err = recoveryjournal.ErrRecoveryUnknown
		}
	}
	code := recoveryjournal.FailureCode(err)
	detail := "gateway recovery unconfirmed; original recovery evidence retained"
	if code == recoveryjournal.CodeStateInvalid {
		detail = "original gateway recovery state unavailable or invalid; no replacement selected"
	}
	if code == recoveryjournal.CodeLocalEvidence {
		detail = "gateway local evidence failed; original recovery envelope retained"
		if result.confirmed && validOutcome {
			detail = "gateway service confirmed " + result.outcome + "; local evidence failed; original recovery envelope retained"
		}
	}
	_ = diagnostics.Emit(stderr, code, detail)
	return diagnostics.ExitOperational
}

func recoverOriginalGateway(ctx context.Context, parent []string, attempt string) (result gatewayRecoveryResult, failure error) {
	b, err := recoveryjournal.DiscoverBinding(ctx, parent, attempt)
	if err != nil {
		return result, err
	}
	n, err := recoveryjournal.OpenExisting(b)
	if err != nil {
		return result, err
	}
	defer func() {
		if n.Close() != nil {
			result.committed = false
			failure = recoveryjournal.ErrLocalEvidence
		}
	}()
	observation, err := n.RecoverExact(ctx, attempt, b, "recover")
	result.confirmed, result.outcome = observation.Confirmed, observation.Outcome
	if observation.Receipt != nil {
		if observation.Receipt.AttemptID() != attempt || observation.Receipt.Outcome() != observation.Outcome {
			return result, recoveryjournal.ErrLocalEvidence
		}
		result.committed = true
	}
	return result, err
}
