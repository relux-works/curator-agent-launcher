package m0adapter

import "syscall"

// Config is everything Run uses. Main passes the installed literals, the
// refusing seams and syscall.Exec. The raw environment is read once, after
// durable C and before any environment mutation; only in-package tests can
// substitute its source.
type Config struct {
	Literals Literals
	WaitForC func() error
	NowMs    func(profile string) (int64, error)
	Exec     func(path string, argv []string, env []string) error

	readEnviron func() ([]string, error)
}

// Main is the production entry point. It never returns: it either becomes
// the runner through execve or exits 78 with one sanitized reason line.
func Main() {
	Exit(Run(Config{
		Literals: installedLiterals(),
		WaitForC: waitForC,
		NowMs:    qualifiedNowMs,
		Exec:     syscall.Exec,
	}))
}
