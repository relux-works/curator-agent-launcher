//go:build linux

package m0adapter

import (
	"os"
	"strings"
	"testing"
)

func promptEvidence(rep runnerReport, want string) string {
	if !rep.EOF || string(rep.Stdin) != want {
		return "prompt_bytes"
	}
	return ""
}

func environmentEvidence(got, want []string) string {
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		return "environment_changed"
	}
	return ""
}

func environmentControlInstances() []controlInstance {
	var out []controlInstance
	for _, mutation := range []string{"env-add-home", "env-add-model", "env-add-endpoint", "env-rewrite-path"} {
		out = append(out, controlInstance{mutation, func(t *testing.T) {
			home := t.TempDir()
			env := fixedEnv(home)
			env[3] = "PATH=/example/tools:/usr/bin:/bin"
			r := runChild(t, childSpec{useHome: home, env: env})
			rep := runnerOutput(t, r)
			if r.exit != 0 || environmentEvidence(rep.Env, env) != "" {
				t.Fatal("unchanged environment refused")
			}
			// Qualification judges the real runner block after each exec-time mutation,
			// not an unrelated incoming-variable refusal.
			badRun := runChild(t, childSpec{useHome: home, env: env, behavior: mutation})
			bad := runnerOutput(t, badRun)
			if badRun.exit != 0 || environmentEvidence(bad.Env, env) != "environment_changed" {
				t.Fatal("environment mutation accepted by qualification")
			}
		}})
	}
	return out
}

func runnerSelectionInstances() []controlInstance {
	return []controlInstance{
		{"entry", func(t *testing.T) {
			requireRunnerPositive(t)
			doc := replaceOnce(t, specimen(t), `"input": {`, `"input": {"runner": "run-claude",`)
			requireRefusal(t, runChild(t, childSpec{plan: []byte(doc)}), ReasonEntryUnknown)
		}},
		{"environment", func(t *testing.T) {
			requireRunnerPositive(t)
			home := t.TempDir()
			requireRefusal(t, runChild(t, childSpec{useHome: home, env: append(fixedEnv(home), "RUNNER=run-claude")}), ReasonEnvUnexpected)
		}},
		{"arguments", runnerArgumentsInstance},
		{"fallback", func(t *testing.T) {
			requireRunnerPositive(t)
			requireRefusal(t, runChild(t, childSpec{behavior: "claude-unpinned"}), ReasonRunnerVariantPending)
		}},
	}
}

func requiredLiteralInstances() []controlInstance {
	var out []controlInstance
	for _, behavior := range []string{"keymaker-missing", "prefix-missing", "origin-missing"} {
		out = append(out, controlInstance{behavior, func(t *testing.T) {
			requireRunnerPositive(t)
			requireRefusal(t, runChild(t, childSpec{behavior: behavior}), ReasonLiteralMissing)
		}})
	}
	return out
}

func pipeStepInstances() []controlInstance {
	var out []controlInstance
	for _, c := range []struct {
		name string
		want Reason
	}{
		{"plan-close-fails", ReasonFDClose}, {"pipe2-fails", ReasonStdinPipe}, {"setfl-fails", ReasonStdinWrite},
		{"write-close-fails", ReasonStdinWrite}, {"dup-fails", ReasonStdinPipe},
	} {
		out = append(out, controlInstance{c.name, func(t *testing.T) {
			requireRunnerPositive(t)
			requireRefusal(t, runChild(t, childSpec{behavior: c.name}), c.want)
		}})
	}
	return out
}

func adapterInstanceTests() map[string][]controlInstance {
	out := map[string][]controlInstance{}
	for id, run := range adapterControlTests() {
		out[id] = []controlInstance{{id, run}}
	}
	for _, c := range grammarNegatives() {
		if c.control != "" && forbiddenCoverage[c.control].test != "" {
			out[c.control] = grammarInstances(c.control)
		}
	}
	for id, run := range map[string]func(*testing.T){
		"stdin-pipe-blocking-write":        TestPromptPipeNonblocking,
		"stdin-pipe-partial-write":         TestPromptPipePartialWrite,
		"stdin-pipe-capacity-raise":        TestPromptPipeNoCapacityRaise,
		"stdin-pipe-write-end-inherited":   TestPromptPipeInheritedWriter,
		"stdin-pipe-second-descriptor":     TestPromptPipeSecondDescriptor,
		"stdin-fd0-read-write":             TestPromptPipeReadOnly,
		"stdin-pipe-capacity-below-length": TestPromptPipeCapacity,
		"stdin-pipe-getpipesz-failure":     TestPromptPipeGetSizeFailure,
		"prompt-decoded-size":              TestProcessPromptDecodedSize,
		"runner-variant-unpinned":          TestRunClaudeRefusesUntilBrokerContractPinned,
		"runner-literal-unknown":           TestUnknownRunnerLiteralRefuses,
		"stdin-pipe-eintr-retry-bound":     TestPromptPipeEINTRBound,
	} {
		out[id] = []controlInstance{{id, run}}
	}
	out["environment"] = environmentControlInstances()
	out["runner-variant-selection"] = runnerSelectionInstances()
	out["denied-literal-member-missing"] = requiredLiteralInstances()
	out["stdin-pipe-step-failure-ignored"] = pipeStepInstances()
	out["adapter-denied-literal-occurrence"] = deniedOccurrenceInstances()
	return out
}

// TestPromptQualificationNegatives proves the observation checker accepts exact bytes/EOF and rejects home substitution and replay for their stated reason.
func TestPromptQualificationNegatives(t *testing.T) {
	for _, id := range []string{"home-prompt", "prompt-replay"} {
		t.Run(id, adapterControlTests()[id])
	}
}

// TestNoCapacitySyscall proves production syntax contains no pipe-capacity-raise operation beside a real-pipe capacity observation.
func TestNoCapacitySyscall(t *testing.T) {
	// The behavioral capacity check is in TestProcessNativePipeProperties;
	// this source check also detects direct capacity syscalls outside the seam.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "1031") || strings.Contains(string(b), "F_SETPIPE_SZ") {
			t.Fatal("pipe_capacity_raise")
		}
	}
}
