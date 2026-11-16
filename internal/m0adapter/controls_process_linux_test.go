//go:build linux

package m0adapter

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func requireRunnerPositive(t *testing.T) {
	t.Helper()
	var plan syscall.Stat_t
	r := runChild(t, childSpec{planStats: &plan})
	rep := runnerOutput(t, r)
	if r.exit != 0 || !rep.EOF || string(rep.Stdin) != specimenInput().Prompt || rep.PID != r.pid {
		t.Fatalf("valid runner exit %d", r.exit)
	}
	if len(rep.Inherited) != 3 {
		t.Fatal("runner inherited extra descriptors")
	}
	for _, fd := range rep.Inherited {
		if fd.FD > 2 || fd.Dev == uint64(plan.Dev) && fd.Ino == uint64(plan.Ino) {
			t.Fatal("runner inherited the plan object or an extra descriptor")
		}
	}
}

func addProcessControlTests(cases map[string]func(*testing.T)) map[string]func(*testing.T) {
	// A home-file decoy cannot replace the bound prompt; changed prompt bytes
	// without matching worker-plan identity refuse before pipe creation.
	cases["home-prompt"] = func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(home+"/prompt.txt", []byte("unbound home-file decoy"), 0o600); err != nil {
			t.Fatal(err)
		}
		v := specimenInput()
		v.Prompt = "bytes from bound FD3 only\n"
		doc := variant(t, v)
		r := runChild(t, childSpec{useHome: home, plan: []byte(doc)})
		if rep := runnerOutput(t, r); r.exit != 0 || !rep.EOF || string(rep.Stdin) != v.Prompt {
			t.Fatal("prompt replaced by home input")
		}
		bad := replaceOnce(t, doc, "bytes from bound FD3 only", "substituted home file bytes")
		requireRefusal(t, runChild(t, childSpec{useHome: home, plan: []byte(bad), behavior: "guard-pipe"}), ReasonEntryWorkerPlan)
	}
	// A plan duplicate is refused before exec beside the plan-free runner.
	cases["fd3-left-open"] = func(t *testing.T) {
		requireRunnerPositive(t)
		f, _ := writePlan(t, t.TempDir(), []byte(specimen(t)))
		requireRefusal(t, runChild(t, childSpec{planFile: f, extra: []*os.File{f}}), ReasonFDPlanDuplicate)
	}
	// The runner sees the unchanged six entries, including a non-default PATH;
	// added reserved names refuse rather than being silently cleared.
	cases["environment"] = func(t *testing.T) {
		home := t.TempDir()
		env := fixedEnv(home)
		env[3] = "PATH=/example/tools:/usr/bin:/bin"
		r := runChild(t, childSpec{useHome: home, env: env})
		if rep := runnerOutput(t, r); r.exit != 0 || strings.Join(rep.Env, "\x00") != strings.Join(env, "\x00") {
			t.Fatal("helper environment rewritten")
		}
		requireRefusal(t, runChild(t, childSpec{useHome: home, env: append(env, "CODEX_HOME=x")}), ReasonEnvUnexpected)
	}
	// One exact prompt then EOF is the only delivery; adding LF without a
	// matching worker-plan identity refuses before writing.
	cases["prompt-replay"] = func(t *testing.T) {
		requireRunnerPositive(t)
		doc := replaceOnce(t, specimen(t), specimenPrompt, `"prompt": "Reply with the word example.\n"`)
		requireRefusal(t, runChild(t, childSpec{plan: []byte(doc), behavior: "guard-pipe"}), ReasonEntryWorkerPlan)
	}
	// The artifact literal supplies element 2; entry/env selectors refuse,
	// and an unpinned artifact cannot fall back to the pinned variant.
	cases["runner-variant-selection"] = func(t *testing.T) {
		requireRunnerPositive(t)
		doc := replaceOnce(t, specimen(t), `"input": {`, `"input": {"runner": "run-claude",`)
		requireRefusal(t, runChild(t, childSpec{plan: []byte(doc)}), ReasonEntryUnknown)
		home := t.TempDir()
		requireRefusal(t, runChild(t, childSpec{useHome: home, env: append(fixedEnv(home), "RUNNER=run-claude")}), ReasonEnvUnexpected)
		// The helper's own argv contains test arguments, not a runner selector;
		// the valid runner report above still has literal run-codex at [1].
		requireRefusal(t, runChild(t, childSpec{behavior: "claude-unpinned"}), ReasonRunnerVariantPending)
	}
	// Direct replacement preserves every pinned runner status, never 78.
	cases["status-remap"] = func(t *testing.T) {
		for _, status := range []string{"0", "1", "64", "69", "75", "124", "130"} {
			v := specimenInput()
			v.Account = "status-" + status
			r := runChild(t, childSpec{plan: []byte(variant(t, v))})
			rep := runnerOutput(t, r)
			want, err := strconv.Atoi(status)
			if err != nil || rep.PID != r.pid || len(r.stderr) != 0 || r.exit != want {
				t.Fatal("status remapped")
			}
			if got := rep.Args[3]; got != v.Account {
				t.Fatal("runner selector changed")
			}
			// TestProcessRunnerStatusPassThrough pins each numeric value too.
		}
		requireRefusal(t, runChild(t, childSpec{exe: t.TempDir() + "/absent"}), ReasonExecFailed)
	}
	return cases
}

// TestProcessPromptDecodedSize proves 24576 decoded bytes arrive intact and 24577 refuse before pipe creation or exec.
func TestProcessPromptDecodedSize(t *testing.T) {
	v := specimenInput()
	v.Prompt = strings.Repeat("p", 24576)
	r := runChild(t, childSpec{plan: []byte(variant(t, v))})
	if rep := runnerOutput(t, r); r.exit != 0 || !rep.EOF || !bytes.Equal(rep.Stdin, []byte(v.Prompt)) {
		t.Fatal("24576-byte positive failed")
	}
	v.Prompt += "p"
	doc := []byte(variant(t, v))
	if len(doc) > maxEncoded {
		t.Fatal("encoded size masks decoded bound")
	}
	requireRefusal(t, runChild(t, childSpec{plan: doc, behavior: "guard-pipe"}), ReasonEntryPromptLength)
}

// TestRunClaudeRefusesUntilBrokerContractPinned proves the proposed variant refuses 78 before pipe creation, with no Codex fallback.
func TestRunClaudeRefusesUntilBrokerContractPinned(t *testing.T) {
	requireRunnerPositive(t)
	requireRefusal(t, runChild(t, childSpec{behavior: "claude-unpinned"}), ReasonRunnerVariantPending)
}

// TestUnknownRunnerLiteralRefuses proves run-other refuses 78 before pipe creation beside the pinned run-codex control.
func TestUnknownRunnerLiteralRefuses(t *testing.T) {
	requireRunnerPositive(t)
	requireRefusal(t, runChild(t, childSpec{behavior: "unknown-runner"}), ReasonRunnerVariantPending)
}

// TestRequiredDeniedLiterals proves each absent required member refuses literal_missing before any pipe or exec beside a complete tuple.
func TestRequiredDeniedLiterals(t *testing.T) {
	for _, behavior := range []string{"keymaker-missing", "prefix-missing", "origin-missing"} {
		t.Run(behavior, func(t *testing.T) {
			requireRunnerPositive(t)
			requireRefusal(t, runChild(t, childSpec{behavior: behavior}), ReasonLiteralMissing)
		})
	}
}

// TestPromptPipeInheritedWriter proves a non-CLOEXEC duplicate of the writer above fd 2 refuses fd_sweep beside exact prompt/EOF delivery.
func TestPromptPipeInheritedWriter(t *testing.T) {
	requireRunnerPositive(t)
	requireRefusal(t, runChild(t, childSpec{behavior: "writer-inherited"}), ReasonFDSweep)
}

// TestPromptPipeSecondDescriptor proves pipe identity at fd 1 refuses fd_final even inside 0-2 beside exact delivery.
func TestPromptPipeSecondDescriptor(t *testing.T) {
	requireRunnerPositive(t)
	requireRefusal(t, runChild(t, childSpec{behavior: "second-read-fd1"}), ReasonFDFinal)
}

// TestPromptPipeReadOnly proves an O_RDWR stdin with matching pipe identity refuses fd_final beside the O_RDONLY control.
func TestPromptPipeReadOnly(t *testing.T) {
	requireRunnerPositive(t)
	requireRefusal(t, runChild(t, childSpec{behavior: "stdin-rdwr"}), ReasonFDFinal)
}

// TestProcessPipeStepFailures proves each listed failed syscall refuses its exact 78 code with no exec beside a valid process.
func TestProcessPipeStepFailures(t *testing.T) {
	for _, c := range []struct {
		behavior string
		want     Reason
	}{
		{"plan-close-fails", ReasonFDClose},
		{"pipe2-fails", ReasonStdinPipe},
		{"getsize-fails", ReasonStdinCapacity},
		{"setfl-fails", ReasonStdinWrite},
		{"write-close-fails", ReasonStdinWrite},
		{"dup-fails", ReasonStdinPipe},
	} {
		t.Run(c.behavior, func(t *testing.T) {
			requireRunnerPositive(t)
			requireRefusal(t, runChild(t, childSpec{behavior: c.behavior}), c.want)
		})
	}
}
