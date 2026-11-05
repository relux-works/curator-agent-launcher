package fragment

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type boundedResolveRunner struct {
	t                 *testing.T
	ordinary, bounded int
	wantDeadline      bool
	stdout            []byte
}

func (r *boundedResolveRunner) Run(ctx context.Context, _ string, _ []string, _ string, _ []string, _ io.Writer) ([]byte, int, error) {
	r.ordinary++
	if _, ok := ctx.Deadline(); ok != r.wantDeadline {
		r.t.Fatalf("ordinary deadline=%v want %v", ok, r.wantDeadline)
	}
	return r.stdout, 0, nil
}

func (r *boundedResolveRunner) RunBounded(ctx context.Context, _ string, _ []string, _ string, _ []string, _ io.Writer) ([]byte, int, error) {
	r.bounded++
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 15*time.Second {
		r.t.Fatal("nonrepair metadata has no bounded deadline")
	}
	return r.stdout, 0, nil
}

// Nonrepair selects the bounded process/drain route and a finite deadline.
// Legacy repair keeps its original route and inherits only caller deadlines.
// This is a dispatch/context control, not a subprocess-cleanup witness.
func TestVerifyOnlyBoundedRunner(t *testing.T) {
	for _, verify := range []bool{false, true} {
		for _, callerDeadline := range []bool{false, true} {
			ctx := context.Background()
			if callerDeadline {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Second)
				defer cancel()
			}
			runner := &boundedResolveRunner{t: t, stdout: []byte(piFragment(t)), wantDeadline: callerDeadline}
			f, err := NewWithRunner("fixture", runner).Resolve(ctx, Request{EnvID: "pi", VerifyOnly: verify})
			if err != nil || f == nil {
				t.Fatalf("valid bounded route refused: %v", err)
			}
			if verify && (runner.bounded != 1 || runner.ordinary != 0) || !verify && (runner.bounded != 0 || runner.ordinary != 1) {
				t.Fatalf("verify=%v calls ordinary=%d bounded=%d", verify, runner.ordinary, runner.bounded)
			}
		}
	}
}

// Verification must omit repair, retain cwd/env and return the existing valid
// fragment; legacy Request still asks for repair through its unchanged default.
func TestVerifyOnlyResolution(t *testing.T) {
	req := Request{EnvID: "pi", Profile: "companyA", ProfileSet: true, VerifyOnly: true, Dir: "/fixture/work", Env: []string{"PATH=/fixture/bin"}}
	want := []string{"env", "resolve", "pi", "--profile", "companyA", "--format", "json"}
	if strings.Join(req.Argv(), "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("verify argv: %q", req.Argv())
	}
	fr := &fakeRunner{stdout: piFragment(t)}
	f, re, _ := resolveWith(t, fr, req)
	if f == nil || re != nil || fr.dir != req.Dir || strings.Join(fr.env, ",") != "PATH=/fixture/bin" || strings.Join(fr.argv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("verify failed: %v %v %+v", f, re, fr)
	}
	legacy := req
	legacy.VerifyOnly = false
	if !strings.Contains(strings.Join(legacy.Argv(), " "), " --repair ") {
		t.Fatal("legacy repair default changed")
	}
}

// Stale verification refuses even with valid stdout; it never retries repair.
// Legacy impossible-under-repair stale keeps its existing invocation-failed code.
func TestVerifyOnlyStaleRefusal(t *testing.T) {
	for _, verify := range []bool{false, true} {
		fr := &fakeRunner{stdout: piFragment(t), stderr: "curator: environment_home_stale: drifted\n", exit: 1}
		f, re, _ := resolveWith(t, fr, Request{EnvID: "pi", VerifyOnly: verify})
		want := CodeInvocationFailed
		if verify {
			want = CodeHomeStale
		}
		if f != nil || re == nil || re.Code != want || re.CuratorCode != "environment_home_stale" || re.ExitCode != 1 || fr.calls != 1 {
			t.Fatalf("verify=%v: %v %+v", verify, f, re)
		}
		for _, arg := range fr.argv {
			if verify && arg == "--repair" {
				t.Fatal("stale verification requested repair")
			}
		}
	}
}
