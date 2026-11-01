//go:build darwin

package claudeauth

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"
)

type boundedSecret struct {
	bytes    []byte
	exceeded bool
}

func (b *boundedSecret) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > maxBytes-len(b.bytes) {
		b.exceeded = true
		p = p[:maxBytes-len(b.bytes)]
	}
	b.bytes = append(b.bytes, p...)
	return n, nil
}

func readKeychain(ctx context.Context, service, account string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, ErrRefused
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", service, "-a", account, "-w")
	// Retain the real home, without transmitting ambient provider credentials.
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home}
	out := &boundedSecret{}
	cmd.Stdout, cmd.Stderr = out, io.Discard
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil || out.exceeded || ctx.Err() != nil {
		clear(out.bytes)
		return nil, ErrRefused
	}
	return out.bytes, nil
}
