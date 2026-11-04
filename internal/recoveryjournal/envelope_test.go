package recoveryjournal

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

func fixtureEnvelope() Envelope {
	return Envelope{Root: "/fixture/project", Session: "environment-20261104T020000Z-123-attempt", Runtime: "codex", AttemptID: strings.Repeat("a", 64), RecoveryProof: strings.Repeat("b", 64), RegistryID: strings.Repeat("c", 64), Boot: "fixture-boot", IssuedNS: "23", ExpiresNS: "120000000023"}
}

// Entropy is independently consumed for attempt and proof; issuance freezes
// the health clock with the exact 120s window. This is not health authentication.
func TestNewEnvelope(t *testing.T) {
	f := IssueFacts{Root: "/fixture/project", Session: "environment-123-attempt", Runtime: "codex", RegistryID: strings.Repeat("c", 64), Boot: "fixture-boot", NowNS: "23"}
	random := append(bytes.Repeat([]byte{0xaa}, 32), bytes.Repeat([]byte{0xbb}, 32)...)
	e, err := newEnvelope(f, bytes.NewReader(random))
	if err != nil || e.AttemptID != strings.Repeat("a", 64) || e.RecoveryProof != strings.Repeat("b", 64) || e.IssuedNS != "23" || e.ExpiresNS != "120000000023" {
		t.Fatal("independent entropy/window projection failed")
	}
	for _, rng := range []*bytes.Reader{bytes.NewReader(nil), bytes.NewReader(bytes.Repeat([]byte{1}, 64))} {
		if _, err := newEnvelope(f, rng); !errors.Is(err, ErrEnvelope) {
			t.Fatal("failed or identical entropy accepted")
		}
	}
	f.NowNS = strconv.FormatUint(math.MaxUint64, 10)
	if _, err := newEnvelope(f, bytes.NewReader(random)); !errors.Is(err, ErrEnvelope) {
		t.Fatal("issuance overflow accepted")
	}
	if e, err := NewEnvelope(IssueFacts{Root: "/fixture/project", Session: "valid-session", Runtime: "codex", RegistryID: f.RegistryID, Boot: f.Boot, NowNS: "0"}); err != nil || e.Validate() != nil {
		t.Fatal("OS entropy positive failed")
	}
}

// Existing valid registration forms stay valid; invalid routing, identity and
// canonical decimal boundaries refuse without printing private fields.
func TestEnvelopePredicatesAndRedaction(t *testing.T) {
	valid := fixtureEnvelope()
	if valid.Validate() != nil {
		t.Fatal("valid envelope refused")
	}
	for _, edit := range []func(*Envelope){
		func(e *Envelope) { e.Root = "relative" }, func(e *Envelope) { e.Session = "contains space" },
		func(e *Envelope) { e.Runtime = "CODEX" }, func(e *Envelope) { e.AttemptID = strings.Repeat("A", 64) },
		func(e *Envelope) { e.RecoveryProof = e.AttemptID }, func(e *Envelope) { e.RegistryID = "" },
		func(e *Envelope) { e.Boot = "" }, func(e *Envelope) { e.IssuedNS = "023" },
		func(e *Envelope) { e.ExpiresNS = "120000000024" }, func(e *Envelope) { e.IssuedNS = "+23" },
		func(e *Envelope) { e.ExpiresNS = "18446744073709551616" },
	} {
		e := valid
		edit(&e)
		if err := e.Validate(); !errors.Is(err, ErrEnvelope) || strings.Contains(err.Error(), e.Root) {
			t.Fatal("invalid envelope or error disclosure")
		}
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(format, valid) + fmt.Sprintf(format, &valid) + fmt.Sprintf(format, Prepared{valid})
		if strings.Contains(out, valid.RecoveryProof) || strings.Contains(out, valid.Root) || strings.Contains(out, valid.RegistryID) {
			t.Fatal("diagnostic disclosed private envelope")
		}
	}
}
