package plan

import (
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/archon-pantheon/pkg/agentic"
)

func TestContextForwardsV7Antigravity(t *testing.T) {
	home := "/manager/environments/default/antigravity"
	frag := &fragment.Fragment{
		Revision:        fragment.IdentityV7,
		Environment:     fragment.EnvAntigravity,
		Profile:         fragment.Profile{Name: "default", LockSHA256: strings.Repeat("a", 64)},
		Precedence:      fragment.Precedence{Winner: "higher-weight", Placement: "winner-last"},
		Permissions:     &fragment.Permissions{Mode: "native", Locked: false, Source: "default"},
		HomeBinding:     &fragment.HomeBinding{Path: home, Transport: "flag"},
		CredentialStore: &fragment.CredentialStore{Selection: "file"},
		Env:             map[string]string{fragment.AntigravityForceFileTokenVariable: fragment.AntigravityForceFileTokenTrue},
		MCP: &fragment.MCP{
			Path:     home + "/" + fragment.AntigravityMCPFile,
			EnvNames: []string{"FIGMA_API_KEY"},
			Channels: []fragment.Channel{{Kind: fragment.KindFile, Filename: fragment.AntigravityMCPFile}},
		},
	}
	ctx := Context(frag, "")
	if ctx == nil {
		t.Fatal("Context returned nil for v7 antigravity")
	}
	if ctx.Revision != agentic.CuratorLaunchFragmentV7 {
		t.Fatalf("Revision = %q, want %q", ctx.Revision, agentic.CuratorLaunchFragmentV7)
	}
	if ctx.Home == nil || ctx.Home.Path != home || ctx.Home.Transport != "flag" {
		t.Fatalf("Home = %+v, want {%s flag}", ctx.Home, home)
	}
	if ctx.CredentialStore == nil || ctx.CredentialStore.Selection != agentic.CuratorCredentialStoreFile {
		t.Fatalf("CredentialStore = %+v, want file", ctx.CredentialStore)
	}
	if ctx.MCP == nil || ctx.MCP.Path != home+"/"+fragment.AntigravityMCPFile {
		t.Fatalf("MCP = %+v", ctx.MCP)
	}
	if err := agentic.ValidateCuratorContext(ctx, home); err != nil {
		t.Fatalf("ValidateCuratorContext rejected projected v7 context: %v", err)
	}
}
