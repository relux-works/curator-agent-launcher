//go:build linux

package m0adapter

import "syscall"

// Run executes the pipeline in spec order and returns a reason only when it
// refuses before exec. On a successful exec it does not return: the process
// is the runner and its status is the runner's.
//
//  1. durable C seam (before any decoding);
//  2. installation literals;
//  3. entry descriptor inventory (0-3 only, no plan duplicate);
//  4. bounded FD3 read, then FD3 and every duplicate closed and verified;
//  5. closed grammar, CCJ-1 and worker_plan consistency;
//  6. fixed environment, paths, qualified clock, T and argv;
//  7. one-shot stdin pipe at fd 0;
//  8. qualified clock re-read (T only lowered) and final descriptor check;
//  9. execve(E, argv, unchanged environment).
func Run(cfg Config) Reason {
	return runWithSyscalls(cfg, nativePromptSyscalls(), syscall.Close)
}

// runWithSyscalls keeps fault injection local to a test invocation.
func runWithSyscalls(cfg Config, ops promptSyscalls, closeFD func(int) error) Reason {
	if err := cfg.WaitForC(); err != nil {
		return reasonOf(err)
	}
	lit := cfg.Literals
	if err := lit.validate(); err != nil {
		return reasonOf(err)
	}
	snap, err := entryInventory()
	if err != nil {
		return reasonOf(err)
	}
	raw, err := readPlan(snap)
	if err != nil {
		return reasonOf(err)
	}
	if err := closePlanAndSweepWith(snap.id, closeFD); err != nil {
		return reasonOf(err)
	}
	entry, err := Decode(raw)
	clear(raw)
	if err != nil {
		return reasonOf(err)
	}
	now, err := cfg.NowMs(lit.ClockProfile)
	if err != nil {
		return reasonOf(err)
	}
	launch, err := prepare(lit, entry, cfg.Environ, now)
	if err != nil {
		return reasonOf(err)
	}
	stdin, err := installPromptWith(launch.Prompt, ops)
	clear(launch.Prompt)
	if err != nil {
		return reasonOf(err)
	}
	again, err := cfg.NowMs(lit.ClockProfile)
	if err != nil {
		return reasonOf(err)
	}
	if err := launch.refreshTimeout(again); err != nil {
		return reasonOf(err)
	}
	if err := verifyDescriptors(snap.id, &stdin); err != nil {
		return reasonOf(err)
	}
	if err := cfg.Exec(lit.Executable, launch.Argv, cfg.Environ); err != nil {
		return ReasonExecFailed
	}
	// A successful execve never returns; reaching here is not a runner start.
	return ReasonExecFailed
}
