package network

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
	"github.com/relux-works/curator-agent-launcher/internal/recoveryjournal"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
	"github.com/relux-works/curator-network-profiles/pkg/resolve"
)

// Only the existing admitted filtering-gateway preparation calls this seam.
// It neither creates missing directories nor admits a new runtime/profile.
func registerRecoveryGateway(ctx context.Context, req Request, selected *resolve.Result, legacy *netproxy.Client, request netproxy.RegisterRequest) (netproxy.Registration, *netproxy.Lease, error) {
	attempt := ""
	refuse := func() (netproxy.Registration, *netproxy.Lease, error) {
		detail := "gateway recovery preparation or issuance unconfirmed; original evidence retained; no downgrade"
		if attempt != "" {
			detail = "attempt=" + attempt + " " + detail
		}
		return netproxy.Registration{}, nil, refusal.New(refusal.CodeProxyUnreachable, selected.Profile.Name, detail)
	}
	b, err := recoveryjournal.ResolveCaller(req.ParentEnv, selected.Digest, selected.Profile.Control)
	if err != nil || recoveryjournal.CheckCatalog(req.ParentEnv, b) != nil {
		return refuse()
	}
	n, err := recoveryjournal.OpenExisting(b)
	if err != nil {
		var missing *recoveryjournal.MissingDirectories
		if errors.As(err, &missing) {
			if labels, projectionErr := missing.PreflightLabels(b); projectionErr == nil {
				return netproxy.Registration{}, nil, refusal.New(refusal.CodeProxyUnreachable, selected.Profile.Name,
					"gateway recovery state incomplete; proposed_directories=["+strings.Join(labels, ",")+"]; creation_mode=0700; STATE is the captured state base; proposal only, no creation performed")
			}
		}
		return refuse()
	}
	owned := false
	defer func() {
		if !owned {
			_ = n.Close()
		}
	}()
	c, err := netproxy.NewRecovery(selected.Profile.Control)
	if err != nil {
		return refuse()
	}
	bounded, cancel := context.WithTimeout(ctx, netproxy.RecoveryCompletionBudget)
	defer cancel()
	h, err := c.Health(bounded)
	if err != nil || !h.Ready {
		return refuse()
	}
	e, err := recoveryjournal.NewEnvelope(recoveryjournal.IssueFacts{Root: request.Root, Session: request.Session, Runtime: request.Runtime, RegistryID: h.RegistryID, Boot: h.Boot, NowNS: h.NowNS})
	if err != nil {
		return refuse()
	}
	// Issue can fail after the private entry is published. Preserve its public
	// selector on every later refusal without exposing the proof or token.
	attempt = e.AttemptID
	// Completion consumes the same original envelope through its guarded exact
	// entry. No callback receives the data-plane token or creates a generation.
	complete := func(ctx context.Context) error {
		observation, failure := n.RecoverExact(ctx, e.AttemptID, b, "finish")
		if n.Close() != nil {
			failure = recoveryjournal.ErrLocalEvidence
		}
		return emitRecoveryCompletion(req.Alert, e.AttemptID, observation, failure)
	}
	requested := time.Now()
	issued, err := n.Issue(bounded, e, b)
	if err != nil {
		if errors.Is(err, recoveryjournal.ErrIssueUnknown) {
			// A lost or cancelled register reply never permits another issue.
			// Reconcile once under a fresh bounded cleanup context even if the
			// launch was interrupted, preserving the exact original entry.
			cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), netproxy.RecoveryCompletionBudget)
			_ = complete(cleanup)
			stop()
		}
		return refuse()
	}
	lease, err := netproxy.NewRecoveryLease(legacy, issued.Registration, req.Alert, requested, complete)
	if err != nil {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), netproxy.RecoveryCompletionBudget)
		_ = complete(cleanup)
		stop()
		return refuse()
	}
	owned = true
	return issued.Registration, lease, nil
}

// Emit only public correlation and fixed verdicts. A service observation is
// distinct from committed local evidence, including failed result output.
func emitRecoveryCompletion(dst io.Writer, attempt string, result recoveryjournal.CleanupObservation, failure error) error {
	if dst == nil {
		dst = io.Discard
	}
	if failure == nil && result.Confirmed && result.Receipt != nil && result.Receipt.AttemptID() == attempt && result.Receipt.Outcome() == result.Outcome {
		line := fmt.Sprintf("curator-run: gateway recovery: attempt=%s service_result=%s receipt=committed\n", attempt, result.Outcome)
		if n, err := io.WriteString(dst, line); err == nil && n == len(line) {
			return nil
		}
		failure = recoveryjournal.ErrLocalEvidence
	}
	if failure == nil {
		failure = recoveryjournal.ErrLocalEvidence
	}
	detail := "gateway recovery unconfirmed; original evidence retained"
	if result.Confirmed {
		detail = "gateway service confirmed; local evidence failed; original evidence retained"
	}
	_, _ = fmt.Fprintf(dst, "curator-run: %s: attempt=%s %s\n", recoveryjournal.FailureCode(failure), attempt, detail)
	return failure
}
