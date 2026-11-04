package recoveryjournal

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func terminalFixture(t *testing.T) (Binding, Envelope, []byte, []byte) {
	t.Helper()
	e := fixtureEnvelope()
	b, err := Resolve(Inputs{XDGStateHome: "/fixture/state", ProfileDigest: strings.Repeat("e", 64), ControlSocket: "/fixture/control.sock"})
	if err != nil {
		t.Fatal(err)
	}
	launch := strings.Repeat("d", 64)
	r := newReceipt(e, b, &launch, "revoked", "service-boot", "recover", time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC))
	receipt, ok := encodeReceipt(r)
	if !ok {
		t.Fatal("receipt fixture invalid")
	}
	anchor, err := nextRetention(nil, receipt, awakeObservation{fixtureBoot, 100})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(terminalRecord{secretRecord{TerminalSecretVersion, e, b.base, b.profile, b.socket, &launch, "ended", "revoked"}, anchor})
	if err != nil {
		t.Fatal(err)
	}
	return b, e, raw, receipt
}

const fixtureBoot = "12345678-1234-4234-8234-123456789abc"

// Both closed versions preserve the original proof for authenticated result
// readback. Terminal metadata supplies no new issuance or activation authority.
func TestTerminalSecretClosedSchema(t *testing.T) {
	b, e, raw, _ := terminalFixture(t)
	got, err := decodeEntry(raw, e.AttemptID, b)
	if err != nil || got.envelope != e || got.retention == nil {
		t.Fatal("valid terminal entry refused")
	}
	if binding, err := recordedBinding(raw, e.AttemptID, location{b.base}); err != nil || binding != b {
		t.Fatal("terminal entry lost exact original routing")
	}
	var cancelled map[string]any
	_ = json.Unmarshal(raw, &cancelled)
	cancelled["last_outcome"], cancelled["launch_id"] = "cancelled_before_issue", nil
	cancelledRaw, _ := json.Marshal(cancelled)
	if entry, err := decodeEntry(cancelledRaw, e.AttemptID, b); err != nil || entry.envelope != e || entry.retention == nil {
		t.Fatal("authenticated cancelled-before-issue representation refused")
	}
	prepared, _ := json.Marshal(secretRecord{SecretVersion, e, b.base, b.profile, b.socket, nil, "prepared", "not_observed"})
	if got, err := decodeEntry(prepared, e.AttemptID, b); err != nil || got.retention != nil || got.envelope != e {
		t.Fatal("original prepared format lost")
	}
	for _, kind := range []string{"missing", "extra", "duplicate", "null", "hash", "boot", "anchor", "anchor-number", "state", "outcome", "launch", "proof-as-launch", "v1-with-retention", "v2-without-retention", "v2-prepared"} {
		t.Run(kind, func(t *testing.T) {
			var m map[string]any
			if json.Unmarshal(raw, &m) != nil {
				t.Fatal("fixture")
			}
			a := m["receipt_retention"].(map[string]any)
			switch kind {
			case "missing":
				delete(a, "boot")
			case "extra":
				a["utc"] = "2026-01-01T00:00:00Z"
			case "null":
				m["receipt_retention"] = nil
			case "hash":
				a["receipt_sha256"] = strings.Repeat("A", 64)
			case "boot":
				a["boot"] = "not-a-boot-uuid"
			case "anchor":
				a["anchor_ns"] = "0100"
			case "anchor-number":
				a["anchor_ns"] = 100
			case "state":
				m["state"] = "active"
			case "outcome":
				m["last_outcome"] = "unknown"
			case "launch":
				m["launch_id"] = nil
			case "proof-as-launch":
				m["launch_id"] = e.RecoveryProof
			case "v1-with-retention":
				m["version"] = SecretVersion
			case "v2-without-retention":
				delete(m, "receipt_retention")
			case "v2-prepared":
				m["state"] = "prepared"
				m["last_outcome"] = "not_observed"
				m["launch_id"] = nil
			}
			bad, _ := json.Marshal(m)
			if kind == "duplicate" {
				bad = bytes.Replace(raw, []byte(`"anchor_ns":"100"`), []byte(`"anchor_ns":"99","anchor_ns":"100"`), 1)
			}
			if got, err := decodeEntry(bad, e.AttemptID, b); got != nil || err != ErrRead {
				t.Fatal("TERMINAL SCHEMA ALARM: malformed metadata admitted")
			}
		})
	}
}

// Expected ages come from the accepted seven-awake-day interval, independently
// of wall UTC. Same-boot retry preserves age; a new boot restarts conservatively.
func TestRetentionAgeAndRetry(t *testing.T) {
	_, _, _, receipt := terminalFixture(t)
	first, err := nextRetention(nil, receipt, awakeObservation{fixtureBoot, 100})
	if err != nil || first.AnchorNS != "100" {
		t.Fatal("first anchor")
	}
	retry, err := nextRetention(&first, receipt, awakeObservation{fixtureBoot, 500})
	if err != nil || retry != first {
		t.Fatal("RETENTION AGE ALARM: same-boot retry reset age")
	}
	for _, tc := range []struct {
		now  uint64
		want bool
	}{{100, false}, {100 + 604800000000000 - 1, false}, {100 + 604800000000000, true}, {100 + 604800000000001, true}} {
		if got := retentionElapsed(first, awakeObservation{fixtureBoot, tc.now}); got != tc.want {
			t.Fatal("RETENTION INTERVAL ALARM: wrong awake interval")
		}
	}
	otherBoot := "22345678-1234-4234-8234-123456789abc"
	restarted, err := nextRetention(&first, receipt, awakeObservation{otherBoot, 9})
	if err != nil || restarted.Boot != otherBoot || restarted.AnchorNS != "9" || retentionElapsed(first, awakeObservation{otherBoot, 999999999999999}) {
		t.Fatal("new boot inherited old age")
	}
	// UTC is observation only: even a huge valid wall jump changes no elapsed decision.
	var r receiptRecord
	_ = json.Unmarshal(receipt, &r)
	r.ObservedUTC = "2099-01-01T00:00:00Z"
	jumped, _ := encodeReceipt(r)
	if _, err := nextRetention(&first, jumped, awakeObservation{fixtureBoot, 500}); err != ErrLocalEvidence {
		t.Fatal("changed immutable receipt silently rebound")
	}
}

// Unknown/hash-mismatched/regressing clock facts prohibit age credit without
// mutating the supplied anchor. Nearby canonical facts above remain valid.
func TestRetentionRefusals(t *testing.T) {
	_, _, _, receipt := terminalFixture(t)
	first, _ := nextRetention(nil, receipt, awakeObservation{fixtureBoot, 100})
	for _, kind := range []string{"regress", "empty-boot", "bad-boot", "bad-hash", "bad-anchor", "bad-json", "foreign-json", "empty-receipt"} {
		t.Run(kind, func(t *testing.T) {
			prior, raw, now := first, append([]byte(nil), receipt...), awakeObservation{fixtureBoot, 500}
			switch kind {
			case "regress":
				now.NS = 99
			case "empty-boot":
				now.Boot = ""
			case "bad-boot":
				now.Boot = "UPPERCASE"
			case "bad-hash":
				prior.ReceiptSHA256 = strings.Repeat("f", 64)
			case "bad-anchor":
				prior.AnchorNS = "0100"
			case "bad-json":
				raw = []byte("{}")
			case "foreign-json":
				raw = []byte(`{"token":"forbidden"}`)
			case "empty-receipt":
				raw = nil
			}
			before := prior
			if _, err := nextRetention(&prior, raw, now); err != ErrLocalEvidence || prior != before {
				t.Fatal("RETENTION REFUSAL ALARM: uncertain state gained age")
			}
			if retentionElapsed(prior, now) {
				t.Fatal("invalid/young state eligible")
			}
		})
	}
}
