// terminal is a bounded subprocess double for terminal stream routing tests.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

func main() {
	time.AfterFunc(10*time.Second, func() { os.Exit(94) })
	if os.WriteFile("terminal.pid", []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
		os.Exit(90)
	}
	input := os.Stdin
	if len(os.Args) > 1 && os.Args[1] == "attach" {
		input = os.NewFile(3, "request")
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(input, 1<<20)); err != nil {
		os.Exit(91)
	}
	fmt.Fprint(os.Stderr, "SESSION_STDERR\ncurator-run: policy_refused: provider text\n")
	os.Exit(7)
}
