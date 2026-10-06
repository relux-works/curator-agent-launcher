package cli_test

import (
	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"strings"
	"testing"
)

func TestNetworkPolicyFlagGrammar(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, tc := range []struct{ flag, mode, pin string }{{"optimistic", "", ""}, {"strict", "strict", ""}, {"pinned:" + digest, "pinned", "sha256-" + digest}} {
		got, e := cli.Parse([]string{"claude_code", "--network-policy", tc.flag}, cli.Options{})
		if e != nil || got.NetworkMode != tc.mode || got.NetworkPin != tc.pin {
			t.Fatalf("policy flag %s not preserved", tc.flag)
		}
	}
}
func TestNetworkPolicyInvalidRefuses(t *testing.T) {
	for _, flag := range []string{"other", "pinned", "pinned:" + strings.Repeat("A", 64), "pinned:" + strings.Repeat("a", 63)} {
		if _, e := cli.Parse([]string{"claude_code", "--network-policy", flag}, cli.Options{}); e == nil {
			t.Fatalf("invalid policy %s admitted", flag)
		}
	}
}
func TestNetworkPolicyRepeatedRefuses(t *testing.T) {
	if _, e := cli.Parse([]string{"claude_code", "--network-policy", "strict", "--network-policy", "optimistic"}, cli.Options{}); e == nil {
		t.Fatal("repeated policy flag admitted")
	}
}
