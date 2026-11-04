package recoveryjournal

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func receiptFixture() receiptRecord {
	e := fixtureEnvelope()
	launch := strings.Repeat("d", 64)
	return newReceipt(e, Binding{profile: strings.Repeat("a", 64)}, &launch, "revoked", "observed-boot", "recover", time.Date(2026, 11, 4, 3, 30, 0, 0, time.UTC))
}

// Both legal outcomes have exact closed public schemas. No authority/private
// routing is present; aliases, duplicate keys, false durability and bad types
// are rejected independently of the atomic writer.
func TestReceiptClosedPublicSchema(t *testing.T) {
	valid := receiptFixture()
	raw, ok := encodeReceipt(valid)
	if !ok {
		t.Fatal("valid receipt rejected")
	}
	for _, forbidden := range []string{fixtureEnvelope().RecoveryProof, fixtureEnvelope().Root, "control_endpoint", "state_base", "recovery_proof", "token", "receipt_committed"} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatal("receipt contains forbidden authority or self-persistence claim")
		}
	}
	cancelled := valid
	cancelled.LaunchID = nil
	cancelled.Result = "cancelled_before_issue"
	if _, ok := encodeReceipt(cancelled); !ok {
		t.Fatal("valid no-issuance receipt rejected")
	}
	for name, edit := range map[string]func(map[string]any){
		"unknown":       func(f map[string]any) { f["root"] = "/private" },
		"missing":       func(f map[string]any) { delete(f, "registry_id") },
		"null-string":   func(f map[string]any) { f["service_boot"] = nil },
		"false":         func(f map[string]any) { f["service_durable"] = false },
		"status-string": func(f map[string]any) { f["service_status"] = "200" },
		"wrong-status":  func(f map[string]any) { f["service_status"] = 204 },
		"operation":     func(f map[string]any) { f["operation"] = "issue" },
		"launch-null":   func(f map[string]any) { f["launch_id"] = nil },
		"outcome":       func(f map[string]any) { f["service_result"] = "unknown" },
		"offset":        func(f map[string]any) { f["observed_confirmation_utc"] = "2026-11-04T03:30:00+00:00" },
		"alias":         func(f map[string]any) { f["Version"] = f["version"]; delete(f, "version") },
	} {
		t.Run(name, func(t *testing.T) {
			var f map[string]any
			_ = json.Unmarshal(raw, &f)
			edit(f)
			bad, _ := json.Marshal(f)
			if _, ok := decodeReceipt(bad); ok {
				t.Fatal("RECEIPT SCHEMA ALARM: invalid public record accepted")
			}
		})
	}
	for _, bad := range []string{strings.Replace(string(raw), `"service_durable":true`, `"service_durable":false,"service_durable":true`, 1), strings.Replace(string(raw), `"version":`, `"\u0076ersion":"ignored","version":`, 1), string(raw) + "{}", "null"} {
		if _, ok := decodeReceipt([]byte(bad)); ok {
			t.Fatal("RECEIPT SCHEMA ALARM: duplicate or trailing record accepted")
		}
	}
}

// Retry may preserve an earlier observation across boot/operation/time changes,
// but never accepts another attempt, profile, launch or authenticated outcome.
func TestReceiptImmutableOutcome(t *testing.T) {
	want := receiptFixture()
	got := want
	got.ServiceBoot = "new-boot"
	got.Operation = "finish"
	got.ObservedUTC = "2026-11-05T00:00:00Z"
	if !sameReceiptOutcome(got, want) {
		t.Fatal("legitimate prior receipt refused")
	}
	for _, edit := range []func(*receiptRecord){func(r *receiptRecord) { r.AttemptID = strings.Repeat("e", 64) }, func(r *receiptRecord) { r.RegistryID = strings.Repeat("e", 64) }, func(r *receiptRecord) { r.ProfileDigest = strings.Repeat("e", 64) }, func(r *receiptRecord) { s := strings.Repeat("e", 64); r.LaunchID = &s }, func(r *receiptRecord) { r.Result = "cancelled_before_issue"; r.LaunchID = nil }, func(r *receiptRecord) { r.Session = "other" }, func(r *receiptRecord) { r.Runtime = "claude" }} {
		bad := got
		edit(&bad)
		if sameReceiptOutcome(bad, want) {
			t.Fatal("foreign receipt outcome accepted")
		}
	}
}
