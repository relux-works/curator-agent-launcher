//go:build !darwin && !linux

package memberobjects

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Source-only unsupported route: refuse without input use. This test is NOT
// selected by the current Darwin/Linux hosted matrix; no platform pass claimed.
func TestUnsupportedNoInputAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned")
	raw := []byte("unchanged")
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, e := f.Seek(3, 0); e != nil {
		t.Fatal(e)
	}
	before, e := f.Stat()
	if e != nil {
		t.Fatal(e)
	}
	for _, input := range []*os.File{f, nil} {
		c, e := Acquire(input, []Reference{{"bad", ^uint64(0), "invalid"}})
		if c != nil || e == nil || e.Error() != "bootstrap/descriptor_invalid" {
			t.Fatal("unsupported route touched declaration/handle policy")
		}
	}
	pos, e := f.Seek(0, 1)
	if e != nil || pos != 3 {
		t.Fatal("caller offset/open state changed")
	}
	after, e := f.Stat()
	if e != nil || after.Mode() != before.Mode() || after.Size() != before.Size() {
		t.Fatal("caller metadata changed")
	}
	b, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(b, raw) {
		t.Fatal("input changed")
	}
}
