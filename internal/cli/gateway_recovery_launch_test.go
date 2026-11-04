package cli

import (
	"strings"
	"testing"
)

// The accepted opt-in is one bare caller flag; native suffixes remain opaque
// and absence preserves legacy selection. Grammar never carries proof values.
func TestGatewayRecoveryLaunchGrammar(t *testing.T) {
	for _, args := range [][]string{{"claude_code", "--native", "--gateway-recovery"}, {"--gateway-recovery", "claude_code", "--network", "egress-a", "--", "-p", "hello"}} {
		inv, err := Parse(args)
		if err != nil || !inv.GatewayRecoveryEnabled || inv.GatewayRecovery != nil {
			t.Fatal("bare native recovery selection refused")
		}
	}
	for _, args := range [][]string{{"claude_code", "--gateway-recovery=true"}, {"claude_code", "--gateway-recovery=false"}, {"claude_code", "--gateway-recovery="}, {"claude_code", "--gateway-recovery", "--gateway-recovery"}, {"claude_code", "--gateway-recovery", "--hosted"}, {"claude_code", "--hosted", "--gateway-recovery"}, {"claude_code", "--gateway-recovery=DO_NOT_ECHO"}} {
		if _, err := Parse(args); !IsUsage(err) || strings.Contains(err.Error(), "DO_NOT_ECHO") {
			t.Fatal("RECOVERY FLAG ALARM: forbidden grammar admitted or value leaked")
		}
	}
	inv, err := Parse([]string{"claude_code", "--", "--gateway-recovery"})
	if err != nil || inv.GatewayRecoveryEnabled || len(inv.Native) != 1 || inv.Native[0] != "--gateway-recovery" {
		t.Fatal("native recovery-looking suffix reinterpreted")
	}
	inv, err = Parse([]string{"claude_code", "--native"})
	if err != nil || inv.GatewayRecoveryEnabled {
		t.Fatal("absent opt-in changed legacy selection")
	}
}
