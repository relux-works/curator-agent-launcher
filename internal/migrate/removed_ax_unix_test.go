//go:build unix

package migrate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A FIFO leftover is listed without a blocking open; apply refuses before
// touching either the FIFO or a neighboring regular policy.
func TestRemoveAXFIFO(t *testing.T) {
	l := fixture(t)
	path := filepath.Join(l.LegacyUser, "ax.json")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	put(t, l.LegacySystem, "ax.json", "neighbor")
	for _, dry := range []bool{true, false} {
		var out bytes.Buffer
		err := Run(l, dry, &out, Options{CanWriteSystem: allowed})
		if dry && err != nil || !dry && (err == nil || !strings.Contains(err.Error(), "remove it manually")) {
			t.Fatal("wrong FIFO outcome", err)
		}
		if !strings.Contains(out.String(), "remove manually (non-regular file; not followed): "+path) {
			t.Fatal(out.String())
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
			t.Fatal("FIFO changed", err)
		}
		if get(t, l.LegacySystem, "ax.json") != "neighbor" {
			t.Fatal("neighbor changed")
		}
	}
}
