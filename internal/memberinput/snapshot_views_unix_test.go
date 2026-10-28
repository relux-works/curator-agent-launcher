//go:build darwin || linux

package memberinput_test

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/memberinput"
)

type expectedView struct {
	raw    string
	length int
	digest [32]byte
}

// These expectations are frozen from independent literal bytes/lengths/digests,
// before ReadPair or any assignment; no view under test supplies the oracle.
func fixedViews(t *testing.T) (expectedView, expectedView) {
	t.Helper()
	digest := func(s string) (sum [32]byte) {
		b, e := hex.DecodeString(s)
		if e != nil || len(b) != len(sum) {
			t.Fatal("invalid fixed digest")
		}
		copy(sum[:], b)
		return sum
	}
	return expectedView{"abc", 3, digest("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")},
		expectedView{"hello", 5, digest("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")}
}
func assertView(t *testing.T, s *memberinput.Snapshot, want expectedView) {
	t.Helper()
	if s == nil {
		t.Fatal("missing non-nil snapshot view")
	}
	if !bytes.Equal(s.Raw(), []byte(want.raw)) || s.Length() != want.length || s.SHA256() != want.digest {
		t.Fatal("snapshot differs from independent frozen bytes/length/hash")
	}
}
func viewInput(t *testing.T, want expectedView) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input")
	if e := os.WriteFile(path, []byte(want.raw), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(path, 0600); e != nil {
		t.Fatal(e)
	}
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { f.Close() })
	if _, e := f.Seek(17, io.SeekStart); e != nil {
		t.Fatal(e)
	}
	return f
}
func assertInput(t *testing.T, f *os.File, want expectedView) {
	t.Helper()
	info, e := f.Stat()
	if e != nil || info.Mode() != 0600 || info.Size() != int64(want.length) {
		t.Fatal("caller handle closed or metadata changed")
	}
	pos, e := f.Seek(0, io.SeekCurrent)
	if e != nil || pos != 17 {
		t.Fatal("caller offset changed")
	}
	b := make([]byte, want.length+1)
	n, e := f.ReadAt(b, 0)
	if (e != nil && e != io.EOF) || n != want.length || !bytes.Equal(b[:n], []byte(want.raw)) {
		t.Fatal("caller bytes changed")
	}
}
func assertRefusal(t *testing.T, n, a *os.File) {
	t.Helper()
	pair, e := memberinput.ReadPair(n, a)
	if pair != nil || e == nil || e.Error() != "bootstrap/descriptor_invalid" || e.Stage() != "bootstrap" || e.Code() != "descriptor_invalid" {
		t.Fatal("invalid handle must return no pair and exact static descriptor refusal")
	}
}

// Four real exported-API cases target BOTH accessors with zero and cross-value
// replacement. Each checks that the assignment actually took effect in its
// returned view, while fresh/earlier/untouched views match fixed original N/A.
// Raw-copy, valid-pair, refusal and caller-closure controls are adjacent. Ordinary
// hosted Darwin/Linux tests only; not a local red-baseline/native/freeze witness.
func TestSnapshotStructViewsAreIndependent(t *testing.T) {
	for _, target := range []string{"request", "authorization"} {
		for _, replacement := range []string{"zero", "cross"} {
			t.Run(target+"/"+replacement, func(t *testing.T) {
				wantN, wantA := fixedViews(t) // Both independent expectations predate mutations.
				n, a := viewInput(t, wantN), viewInput(t, wantA)
				assertRefusal(t, n, nil)
				assertRefusal(t, nil, a)
				assertInput(t, n, wantN)
				assertInput(t, a, wantA)
				pair, e := memberinput.ReadPair(n, a)
				if e != nil || pair == nil {
					t.Fatal("valid readonly pair refused", e)
				}
				assertView(t, pair.Request(), wantN)
				assertView(t, pair.Authorization(), wantA)
				getTarget, getOther := (*memberinput.UntrustedPair).Request, (*memberinput.UntrustedPair).Authorization
				wantTarget, wantOther := wantN, wantA
				if target == "authorization" {
					getTarget, getOther = getOther, getTarget
					wantTarget, wantOther = wantA, wantN
				}
				earlierTarget, untouchedOther := getTarget(pair), getOther(pair)
				earlierRaw, otherRaw := earlierTarget.Raw(), untouchedOther.Raw()
				mutated, opposite := getTarget(pair), getOther(pair)
				assertView(t, earlierTarget, wantTarget)
				assertView(t, untouchedOther, wantOther)
				assertView(t, mutated, wantTarget)
				assertView(t, opposite, wantOther)
				wantMutated := expectedView{} // Zero struct has zero digest, NOT hash(empty).
				if replacement == "zero" {
					*mutated = memberinput.Snapshot{}
				} else {
					*mutated = *opposite
					wantMutated = wantOther
				}
				assertView(t, mutated, wantMutated) // A skipped assignment cannot pass.
				checkOriginals := func() {
					assertView(t, getTarget(pair), wantTarget)
					assertView(t, earlierTarget, wantTarget)
					assertView(t, getOther(pair), wantOther)
					assertView(t, untouchedOther, wantOther)
					assertView(t, opposite, wantOther)
					assertView(t, pair.Request(), wantN)
					assertView(t, pair.Authorization(), wantA)
					if !bytes.Equal(earlierRaw, []byte(wantTarget.raw)) || !bytes.Equal(otherRaw, []byte(wantOther.raw)) {
						t.Fatal("earlier Raw observation changed")
					}
				}
				checkOriginals()
				// Raw from fresh N, fresh A and an earlier target view must each be copies.
				for _, s := range []*memberinput.Snapshot{pair.Request(), pair.Authorization(), earlierTarget} {
					b := s.Raw()
					b[0] ^= 0xff
				}
				checkOriginals()
				assertView(t, mutated, wantMutated)
				assertInput(t, n, wantN)
				assertInput(t, a, wantA)
				if e := n.Close(); e != nil {
					t.Fatal(e)
				}
				if e := a.Close(); e != nil {
					t.Fatal(e)
				}
				if _, e := n.Stat(); e == nil {
					t.Fatal("request closure control did not fire")
				}
				if _, e := a.Stat(); e == nil {
					t.Fatal("authorization closure control did not fire")
				}
				checkOriginals()
				assertView(t, mutated, wantMutated)
			})
		}
	}
}

// The pre-existing zero pair returns nil observations. This narrow assertion
// preserves that behavior without adding any nil receiver API promise.
func TestZeroPairAccessorsRemainNil(t *testing.T) {
	var pair memberinput.UntrustedPair
	if pair.Request() != nil || pair.Authorization() != nil {
		t.Fatal("zero pair accessor behavior changed")
	}
}
