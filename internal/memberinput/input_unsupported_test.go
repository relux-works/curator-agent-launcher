//go:build !darwin && !linux

package memberinput

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Unsupported builds refuse without even needing valid handles; real fixture
// bytes, open state, mode and offset stay unchanged. Not a portable custody claim.
func TestUnsupportedRefusesWithoutInputEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	original := []byte("fixture")
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		t.Fatal(e)
	}
	if _, e := f.Seek(2, 0); e != nil {
		t.Fatal(e)
	}
	p, err := ReadPair(f, f)
	if p != nil || err == nil || err.Error() != "bootstrap/descriptor_invalid" {
		t.Fatal("unsupported primitive did not explicitly refuse")
	}
	pos, e := f.Seek(0, 1)
	if e != nil || pos != 2 {
		t.Fatal("caller file closed or offset changed")
	}
	after, e := f.Stat()
	if e != nil || after.Mode() != before.Mode() {
		t.Fatal("caller mode/open state changed")
	}
	raw, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(raw, original) {
		t.Fatal("caller content changed")
	}
	p, err = ReadPair(nil, nil)
	if p != nil || err == nil || err.Error() != "bootstrap/descriptor_invalid" {
		t.Fatal("unsupported nil handles were touched or accepted")
	}
}
