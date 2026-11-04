package recoveryjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
)

const TerminalSecretVersion = "netproxy-recovery-secret/2"
const retentionIntervalNS uint64 = 7 * 24 * 60 * 60 * 1000000000

// PruneObservation records only checked durable removals. An error may leave
// SecretRemoved true and ReceiptRemoved false; that orphan receipt is retained.
type PruneObservation struct{ SecretRemoved, ReceiptRemoved bool }

// This metadata remains private and carries no independent service authority.
// Its anchor must be sampled only after checked durable receipt publication.
type receiptRetention struct {
	ReceiptSHA256 string `json:"receipt_sha256"`
	Boot          string `json:"boot"`
	AnchorNS      string `json:"anchor_ns"`
}

type terminalRecord struct {
	secretRecord
	Retention receiptRetention `json:"receipt_retention"`
}

// awakeObservation is kernel awake uptime in nanoseconds, paired with the
// kernel boot UUID. Wall time and suspend-inclusive clocks are not substitutes.
type awakeObservation struct {
	Boot string
	NS   uint64
}

func bootUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (r receiptRetention) valid() bool {
	_, ok := decimal(r.AnchorNS)
	return ok && digest(r.ReceiptSHA256) && bootUUID(r.Boot)
}

func decodeRetention(raw []byte) (*receiptRetention, bool) {
	f, ok := closedObject(raw, "receipt_sha256", "boot", "anchor_ns")
	if !ok {
		return nil, false
	}
	for _, value := range f {
		var s string
		if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &s) != nil {
			return nil, false
		}
	}
	var r receiptRetention
	if json.Unmarshal(raw, &r) != nil || !r.valid() {
		return nil, false
	}
	return &r, true
}

// nextRetention is a pure policy helper, not a durability certificate. The
// caller must validate exact receipt/envelope correlation and complete receipt
// durability BEFORE sampling now, then durably publish the returned secret/2.
// An existing same-boot anchor is immutable. A new boot restarts the interval.
func nextRetention(prior *receiptRetention, receipt []byte, now awakeObservation) (receiptRetention, error) {
	if _, ok := decodeReceipt(receipt); !ok || !bootUUID(now.Boot) {
		return receiptRetention{}, ErrLocalEvidence
	}
	hash := sha256.Sum256(receipt)
	r := receiptRetention{hex.EncodeToString(hash[:]), now.Boot, strconv.FormatUint(now.NS, 10)}
	if prior != nil {
		if !prior.valid() || prior.ReceiptSHA256 != r.ReceiptSHA256 {
			return receiptRetention{}, ErrLocalEvidence
		}
		if prior.Boot == now.Boot {
			anchor, _ := decimal(prior.AnchorNS)
			if now.NS < anchor {
				return receiptRetention{}, ErrLocalEvidence
			}
			return *prior, nil
		}
	}
	return r, nil
}

// An age predicate alone never authorizes deletion. Exact receipt hashing,
// terminal durability, guarded removal and checked directory sync remain
// mandatory. Unknown or regressing observations receive no age credit.
func retentionElapsed(r receiptRetention, now awakeObservation) bool {
	if !r.valid() || now.Boot != r.Boot {
		return false
	}
	anchor, _ := decimal(r.AnchorNS)
	return now.NS >= anchor && now.NS-anchor >= retentionIntervalNS
}
