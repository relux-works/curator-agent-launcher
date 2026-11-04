package cli

// GatewayRecoveryCommand selects one public attempt, never a path, proof,
// endpoint, profile or runtime. It has no native argument tail.
type GatewayRecoveryCommand struct{ Attempt string }

const GatewayRecoveryHelp = `usage: curator-run gateway recover --attempt <64 lowercase hex>

Recover the original saved gateway attempt and commit local cleanup evidence.
Only the named original entry is read. No runtime is launched or registered.
The command accepts no proof, token, path, endpoint or profile override.
Service confirmation and local receipt/retention durability are separate facts.
The fixed operation budget is 30 seconds; uncertainty retains recovery evidence.
`

func parseGatewayRecovery(args []string) (Invocation, error) {
	if len(args) != 4 || args[0] != "gateway" || args[1] != "recover" || args[2] != "--attempt" || !publicAttempt(args[3]) {
		return Invocation{}, usageErr("expected gateway recover --attempt <64 lowercase hex>")
	}
	return Invocation{GatewayRecovery: &GatewayRecoveryCommand{Attempt: args[3]}}, nil
}

func publicAttempt(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
