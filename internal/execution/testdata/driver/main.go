// A process-level caller of the API, for signal forwarding and default stdio.
package main

import (
	"fmt"
	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/mapping"
	"github.com/relux-works/skill-agents-management/pkg/agentic"
	"os"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	value := composition.Value{Binary: os.Args[1], WorkDir: os.Args[2], Env: []string{}, RawStdin: agentic.StdinPayload{Attached: true}}
	if len(os.Args) > 4 {
		value.RawStdin.Attached = false
	}
	launch, err := execution.Prepare(value, fragment.Fragment{}, cli.Invocation{EnvID: "pi", Tracked: os.Args[3] == "true"}, mapping.Target{System: "pi-native", Provider: "pi"}, time.Now())
	if err != nil {
		panic(err)
	}
	code := launch.Run(execution.Options{AxBinary: os.Args[1], Boundary: func() error { return nil }})
	if len(os.Args) > 4 {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			panic(err)
		}
		defer tty.Close()
		var group int32
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&group)))
		fmt.Printf("RESTORED=%t\n", errno == 0 && group == int32(syscall.Getpgrp()))
	}
	os.Exit(code)
}
