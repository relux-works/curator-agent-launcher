package m0adapter

import "syscall"

// Config is everything Run uses. Main passes the installed literals, the
// three refusing seams, the process environment and syscall.Exec; nothing
// else can construct a production Config. Tests substitute fakes.
type Config struct {
	Literals Literals
	WaitForC func() error
	NowMs    func(profile string) (int64, error)
	Environ  []string
	Exec     func(path string, argv []string, env []string) error
}

// Main is the production entry point. It never returns: it either becomes
// the runner through execve or exits 78 with one sanitized reason line.
func Main() {
	Exit(Run(Config{
		Literals: installedLiterals(),
		WaitForC: waitForC,
		NowMs:    qualifiedNowMs,
		Environ:  syscall.Environ(),
		Exec:     syscall.Exec,
	}))
}
