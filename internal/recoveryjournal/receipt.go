package recoveryjournal

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
)

const ReceiptVersion = "netproxy-cleanup-receipt/1"

// ReceiptCommit is returned only after the receipt file and directory syncs.
// It exposes no path, capability, raw service reply or private journal entry.
type ReceiptCommit struct{ attempt, outcome string }

func (*ReceiptCommit) String() string      { return "<committed cleanup receipt>" }
func (*ReceiptCommit) GoString() string    { return "<committed cleanup receipt>" }
func (r *ReceiptCommit) AttemptID() string { return r.attempt }
func (r *ReceiptCommit) Outcome() string   { return r.outcome }

// CleanupObservation keeps service confirmation distinct from local evidence.
// On local_evidence_failed, Confirmed and Outcome may be set, but Receipt is nil.
type CleanupObservation struct {
	Confirmed bool
	Outcome   string
	Receipt   *ReceiptCommit
}

type receiptRecord struct {
	Version        string  `json:"version"`
	RegistryID     string  `json:"registry_id"`
	AttemptID      string  `json:"attempt_id"`
	LaunchID       *string `json:"launch_id"`
	ProfileDigest  string  `json:"profile_digest"`
	Session        string  `json:"registration_session"`
	Runtime        string  `json:"registration_runtime"`
	ServiceVersion string  `json:"service_version"`
	ServiceBoot    string  `json:"service_boot"`
	Operation      string  `json:"operation"`
	Result         string  `json:"service_result"`
	Status         int     `json:"service_status"`
	ObservedUTC    string  `json:"observed_confirmation_utc"`
	Durable        bool    `json:"service_durable"`
}

func validOperation(operation string) bool { return operation == "recover" || operation == "finish" }

func newReceipt(e Envelope, b Binding, launch *string, outcome, boot, operation string, now time.Time) receiptRecord {
	return receiptRecord{ReceiptVersion, e.RegistryID, e.AttemptID, launch, b.profile, e.Session, e.Runtime, netproxy.RecoveryAPI, boot, operation, outcome, 200, now.UTC().Format(time.RFC3339Nano), true}
}

func encodeReceipt(r receiptRecord) ([]byte, bool) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, false
	}
	_, ok := decodeReceipt(raw)
	return raw, ok
}

// The receipt is a closed public observation, not authority. Its UTC timestamp
// predates publication and must never be used as a post-commit retention anchor.
func decodeReceipt(raw []byte) (receiptRecord, bool) {
	f, ok := closedObject(raw, "version", "registry_id", "attempt_id", "launch_id", "profile_digest", "registration_session", "registration_runtime", "service_version", "service_boot", "operation", "service_result", "service_status", "observed_confirmation_utc", "service_durable")
	if !ok {
		return receiptRecord{}, false
	}
	for _, key := range []string{"version", "registry_id", "attempt_id", "profile_digest", "registration_session", "registration_runtime", "service_version", "service_boot", "operation", "service_result", "observed_confirmation_utc"} {
		var value string
		if bytes.Equal(bytes.TrimSpace(f[key]), []byte("null")) || json.Unmarshal(f[key], &value) != nil {
			return receiptRecord{}, false
		}
	}
	var r receiptRecord
	if json.Unmarshal(raw, &r) != nil {
		return receiptRecord{}, false
	}
	stamp, err := time.Parse(time.RFC3339Nano, r.ObservedUTC)
	if r.Version != ReceiptVersion || !digest(r.RegistryID) || !digest(r.AttemptID) || !digest(r.ProfileDigest) || r.ServiceVersion != netproxy.RecoveryAPI || r.ServiceBoot == "" || !validOperation(r.Operation) || r.Status != 200 || !r.Durable || err != nil || !strings.HasSuffix(r.ObservedUTC, "Z") || stamp.UTC().Format(time.RFC3339Nano) != r.ObservedUTC {
		return receiptRecord{}, false
	}
	// Reuse the owning public registration correlation predicates; no project
	// root is serialized into the receipt or accepted as receipt authority.
	if (netproxy.RegisterRequest{Root: "/", Session: r.Session, Runtime: r.Runtime}).Validate() != nil {
		return receiptRecord{}, false
	}
	switch r.Result {
	case "revoked":
		if r.LaunchID == nil || !digest(*r.LaunchID) {
			return receiptRecord{}, false
		}
	case "cancelled_before_issue":
		if !bytes.Equal(bytes.TrimSpace(f["launch_id"]), []byte("null")) {
			return receiptRecord{}, false
		}
	default:
		return receiptRecord{}, false
	}
	return r, true
}

// A later authenticated call may observe a new boot, operation and UTC. A
// matching immutable earlier receipt can be re-synced without rewriting it.
func sameReceiptOutcome(got, want receiptRecord) bool {
	got.ServiceBoot = want.ServiceBoot
	got.Operation = want.Operation
	got.ObservedUTC = want.ObservedUTC
	return reflect.DeepEqual(got, want)
}
