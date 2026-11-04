package recoveryjournal

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"math"
	"strconv"

	"github.com/relux-works/curator-agent-launcher/internal/netproxy"
)

const issuanceWindowNS = netproxy.IssuanceWindowNS

var ErrEnvelope = netproxy.ErrRecoveryEnvelope

// IssueFacts must be obtained from the admitted control/2 health and original
// registration tuple. This type authenticates neither their source nor health.
type IssueFacts struct {
	Root, Session, Runtime, RegistryID, Boot, NowNS string
}

// Envelope is the frozen control/2 issuance request. RecoveryProof is a
// capability: serialize only into its private journal or authenticated control
// request, never into diagnostics, a receipt, child input or a runtime config.
type Envelope = netproxy.RecoveryEnvelope

func NewEnvelope(f IssueFacts) (Envelope, error) { return newEnvelope(f, rand.Reader) }

func newEnvelope(f IssueFacts, rng io.Reader) (Envelope, error) {
	issued, ok := decimal(f.NowNS)
	if !ok || issued > math.MaxUint64-issuanceWindowNS {
		return Envelope{}, ErrEnvelope
	}
	var random [64]byte
	if _, err := io.ReadFull(rng, random[:]); err != nil {
		return Envelope{}, ErrEnvelope
	}
	e := Envelope{
		Root: f.Root, Session: f.Session, Runtime: f.Runtime,
		RegistryID: f.RegistryID, Boot: f.Boot, IssuedNS: f.NowNS,
		ExpiresNS: strconv.FormatUint(issued+issuanceWindowNS, 10),
		AttemptID: hex.EncodeToString(random[:32]), RecoveryProof: hex.EncodeToString(random[32:]),
	}
	if err := e.Validate(); err != nil {
		return Envelope{}, ErrEnvelope
	}
	return e, nil
}

func decimal(s string) (uint64, bool) {
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil && strconv.FormatUint(n, 10) == s
}

const SecretVersion = "netproxy-recovery-secret/1"
const MaxRecord = 16 << 10
const MaxAttempts = 4096

// The caller contract bounds each of 4096 private records at 16 KiB. The
// service's independent 16 MiB snapshot cap is not a caller journal cap.
const MaxSecretBytes = MaxAttempts * MaxRecord

type secretRecord struct {
	Version       string   `json:"version"`
	Envelope      Envelope `json:"envelope"`
	StateBase     string   `json:"state_base"`
	ProfileDigest string   `json:"profile_digest"`
	ControlSocket string   `json:"control_endpoint"`
	LaunchID      *string  `json:"launch_id"`
	State         string   `json:"state"`
	LastOutcome   string   `json:"last_outcome"`
}

// Prepared exists only after file and parent-directory durability succeed.
// It is not proof that a service request was made or that a capability ended.
type Prepared struct{ envelope Envelope }

func (Prepared) String() string       { return "<prepared private recovery entry>" }
func (Prepared) GoString() string     { return "<prepared private recovery entry>" }
func (p *Prepared) AttemptID() string { return p.envelope.AttemptID }
