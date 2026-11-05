package main

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/protocolchild"
)

// protocolMetadata is a private checked-input snapshot, not an executable plan
// or evidence of Host operation admission. No child environment is exported.
type protocolMetadata struct {
	selectors *protocolchild.Selectors
	baseline  []string
	fragment  *fragment.Fragment
	workspace string
	argv      []string
}

// resolveProtocolMetadata prepares the closed backend's input/profile seam.
// The frontend remains on its repair path. This helper neither dispatches a
// CLI mode nor starts a native process, selects a Host or repairs a profile.
func resolveProtocolMetadata(ctx context.Context, args []string, deps launchDeps) (*protocolMetadata, error) {
	refuse := func(code string) (*protocolMetadata, error) {
		return nil, &protocolchild.Error{Code: code, Detail: "protocol child metadata preparation refused"}
	}
	s, err := protocolchild.Parse(args)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	environ := deps.environ
	if environ == nil {
		environ = os.Environ
	}
	baseline, err := protocolchild.EntryEnvironment(environ())
	if err != nil {
		return nil, err
	}
	workdir := deps.workdir
	if workdir == nil {
		workdir = os.Getwd
	}
	wd, err := workdir()
	if err != nil || !filepath.IsAbs(wd) {
		return refuse("protocol_child_stale")
	}
	physical, err := filepath.EvalSymlinks(wd)
	if err != nil || physical != s.Workspace {
		return refuse("protocol_child_stale")
	}
	selected, err := filepath.EvalSymlinks(s.Workspace)
	if err != nil || selected != s.Workspace {
		return refuse("protocol_child_stale")
	}
	if deps.resolver == nil {
		return refuse("protocol_child_resolve_refused")
	}
	f, err := deps.resolver.Resolve(ctx, fragment.Request{
		EnvID: fragment.EnvCodexCLI, Profile: s.Profile, ProfileSet: true,
		VerifyOnly: true, Dir: s.Workspace, Env: append([]string{}, baseline...), Stderr: io.Discard,
	})
	if ctx.Err() != nil {
		return refuse("protocol_child_cancelled")
	}
	if err != nil {
		if resolved, ok := fragment.IsResolve(err); ok && resolved.Code == fragment.CodeHomeStale {
			return refuse("protocol_child_stale")
		}
		return refuse("protocol_child_resolve_refused")
	}
	if f == nil || fragment.CheckLaunchRevision(f) != nil || f.Environment != fragment.EnvCodexCLI ||
		f.Profile.Name != s.Profile || f.Profile.LockSHA256 != s.LockSHA256 || f.Digest != fragment.DigestPrefix+s.FragmentSHA256 {
		return refuse("protocol_child_stale")
	}
	return &protocolMetadata{selectors: s, baseline: baseline, fragment: f, workspace: s.Workspace, argv: append([]string{}, args...)}, nil
}
