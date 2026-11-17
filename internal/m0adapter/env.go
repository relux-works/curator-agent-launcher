package m0adapter

import "strings"

// fixedEnvNames is the manager's fixed helper environment (spec §4). The
// adapter admits exactly these names, passes the environment to the runner
// unchanged and never clears or adds a variable to gain admission.
var fixedEnvNames = [...]string{"HOME", "USER", "LOGNAME", "PATH", "LANG", "TMPDIR"}

const (
	envHome   = 0
	envTmpdir = 5
)

// checkEnv validates env without modifying it. Refusal order: malformed,
// unexpected or duplicate name, missing name, deny marker, HOME != H, TMPDIR
// outside H.
func checkEnv(env []string, lit Literals) error {
	var seen [len(fixedEnvNames)]bool
	var values [len(fixedEnvNames)]string
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			return refuse(ReasonEnvMalformed)
		}
		idx := -1
		for i, n := range fixedEnvNames {
			if kv[:eq] == n {
				idx = i
				break
			}
		}
		if idx < 0 {
			return refuse(ReasonEnvUnexpected)
		}
		if seen[idx] {
			return refuse(ReasonEnvDuplicate)
		}
		seen[idx] = true
		values[idx] = kv[eq+1:]
	}
	for _, ok := range seen {
		if !ok {
			return refuse(ReasonEnvMissing)
		}
	}
	for i, v := range values {
		if lit.deniedMarker(fixedEnvNames[i]) || lit.deniedMarker(v) {
			return refuse(ReasonEnvDenied)
		}
	}
	if values[envHome] != lit.Home {
		return refuse(ReasonEnvHomeMismatch)
	}
	if t := values[envTmpdir]; t != lit.Home && !strings.HasPrefix(t, lit.Home+"/") {
		return refuse(ReasonEnvTmpdir)
	}
	return nil
}
