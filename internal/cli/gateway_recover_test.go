package cli

import (
	"strings"
	"testing"
)

// Only the exact separated fixed command selects recovery. Malformed or
// expanded argv is a bounded, value-free usage refusal; native tails stay native.
func TestGatewayRecoveryGrammar(t *testing.T) {
	attempt := strings.Repeat("a", 64)
	inv, err := Parse([]string{"gateway", "recover", "--attempt", attempt})
	if err != nil || inv.GatewayRecovery == nil || inv.GatewayRecovery.Attempt != attempt || inv.EnvID != "" || inv.Native != nil {
		t.Fatal("valid exact recovery grammar refused or became launch")
	}
	for _, args := range [][]string{{"gateway"}, {"gateway", "recover"}, {"gateway", "recover", "--attempt"}, {"gateway", "recover", "--attempt", ""}, {"gateway", "recover", "--attempt", strings.Repeat("A", 64)}, {"gateway", "recover", "--attempt", strings.Repeat("a", 63)}, {"gateway", "recover", "--attempt", strings.Repeat("g", 64)}, {"gateway", "recover", "--attempt", attempt, "--attempt", attempt}, {"gateway", "recover", "--attempt", attempt, "extra"}, {"gateway", "recover", "--attempt=" + attempt}, {"gateway", "recover", "--attempt", attempt, "--", "native"}, {"gateway", "list"}, {"--native", "gateway", "recover", "--attempt", attempt}} {
		if got, err := Parse(args); !IsUsage(err) || got.GatewayRecovery != nil {
			t.Fatal("GATEWAY GRAMMAR ALARM: forbidden shape admitted")
		}
	}
	secret := "DO_NOT_ECHO\ncurator-run: forged: token\x00"
	for _, flag := range []string{"--proof", "--token", "--endpoint", "--path", "--profile", "--member", "--unknown"} {
		args := []string{"gateway", "recover", "--attempt", attempt, flag, secret}
		if _, err := Parse(args); !IsUsage(err) || strings.Contains(err.Error(), "DO_NOT_ECHO") || strings.Contains(err.Error(), "forged") {
			t.Fatal("gateway usage leaked values or accepted override")
		}
	}
	for _, value := range []string{secret, "--help", attempt + "\n", attempt + "\x00", strings.Repeat("a", 1<<20)} {
		if _, err := Parse([]string{"gateway", "recover", "--attempt", value}); !IsUsage(err) || strings.Contains(err.Error(), "DO_NOT_ECHO") {
			t.Fatal("invalid selector admitted/leaked")
		}
	}
	inv, err = Parse([]string{"codex", "--", "gateway", "recover", "--attempt", attempt})
	if err != nil || inv.GatewayRecovery != nil || len(inv.Native) != 4 {
		t.Fatal("native tail reinterpreted")
	}
}

// Help is constant even beside forbidden values; it does not interpolate an
// attempt, arbitrary verb or synthetic secret into its output.
func TestGatewayRecoveryStaticHelp(t *testing.T) {
	for _, args := range [][]string{{"gateway", "--help"}, {"gateway", "recover", "-h"}, {"gateway", "recover", "--token", "DO_NOT_ECHO", "--help"}} {
		text, ok := TopicHelp(args)
		if !ok || text != GatewayRecoveryHelp || strings.Contains(text, "DO_NOT_ECHO") {
			t.Fatal("gateway help is not static")
		}
	}
}
