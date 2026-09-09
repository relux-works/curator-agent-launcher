// Fake provider and fake ax. Never invokes an installed model or ax.
package main

import (
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	data, _ := io.ReadAll(os.Stdin)
	cwd, _ := os.Getwd()
	env := os.Environ()
	if env == nil {
		env = []string{}
	}
	_ = json.NewEncoder(os.Stdout).Encode(struct {
		Argv, Env []string
		Stdin     []byte
		WorkDir   string
	}{os.Args[1:], env, data, cwd})
	behavior, _ := os.ReadFile("behavior")
	if string(behavior) == "waitsignal" {
		os.Stdout.Write([]byte("READY\n"))
		for {
			time.Sleep(time.Hour)
		}
	}
	if string(behavior) == "signal" {
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		select {}
	}
	if strings.HasPrefix(string(behavior), "exit:") {
		code, _ := strconv.Atoi(strings.TrimPrefix(string(behavior), "exit:"))
		os.Stderr.Write([]byte("{\"code\":\"launch_plan_invalid\"}\n\x00raw\xff"))
		os.Exit(code)
	}
	os.Stderr.Write([]byte("helper stderr\n"))
}
