package m0adapter

import (
	"io"
	"strings"
	"testing"
)

type environmentReader struct {
	io.Reader
	closeErr error
}

func (r environmentReader) Close() error { return r.closeErr }

type failedEnvironmentRead struct{}

func (failedEnvironmentRead) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestRawEnvironmentRead proves exact ordered entries survive and incomplete, unreadable or over-bound blocks refuse without fallback.
func TestRawEnvironmentRead(t *testing.T) {
	base := strings.Join(fixedEnv(testHome), "\x00") + "\x00"
	for _, c := range []struct {
		name, block       string
		readErr, closeErr bool
		want              Reason
	}{
		{"unterminated", strings.TrimSuffix(base, "\x00"), false, false, ReasonEnvMalformed},
		{"over-bound", strings.Repeat("x", maxEnvironmentBytes+1), false, false, reasonEnvLimit},
		{"entry-bound", strings.Repeat("\x00", maxEnvironmentEntries+1), false, false, reasonEnvLimit},
		{"incomplete", "", true, false, reasonEnvRead},
		{"close-failure", base, false, true, reasonEnvRead},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			good, err := readEnvironmentWith(func() (io.ReadCloser, error) { calls++; return io.NopCloser(strings.NewReader(base)), nil })
			if err != nil || calls != 1 || strings.Join(good, "\x00")+"\x00" != base {
				t.Fatalf("raw positive: %v", err)
			}
			_, err = readEnvironmentWith(func() (io.ReadCloser, error) {
				var r io.Reader = strings.NewReader(c.block)
				if c.readErr {
					r = failedEnvironmentRead{}
				}
				var closeErr error
				if c.closeErr {
					closeErr = io.ErrClosedPipe
				}
				return environmentReader{r, closeErr}, nil
			})
			requireReason(t, err, c.want)
		})
	}
	for _, extra := range []string{"HOME=/other", ""} {
		block := base + extra + "\x00"
		entries, err := readEnvironmentWith(func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(block)), nil })
		if err != nil || len(entries) != 7 || entries[6] != extra {
			t.Fatalf("raw entry hidden: %v", err)
		}
	}
	_, err := readEnvironmentWith(func() (io.ReadCloser, error) { return nil, io.ErrClosedPipe })
	requireReason(t, err, reasonEnvRead)
}
