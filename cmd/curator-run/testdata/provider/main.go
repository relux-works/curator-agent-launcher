// Fake provider and fake ax. Never invokes an installed model or ax.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Fprintln(os.Stdout, os.Getenv("CURATOR_TEST_RELEASE"))
		return
	}
	_ = os.WriteFile("started", []byte("started"), 0600)
	data, _ := io.ReadAll(os.Stdin)
	cwd, _ := os.Getwd()
	env := os.Environ()
	filtered := env[:0]
	for _, entry := range env {
		if !strings.HasPrefix(entry, "CURATOR_TEST_RELEASE=") {
			filtered = append(filtered, entry)
		}
	}
	env = filtered
	// Fake ax inherits the real launcher environment. Capture only the harmless
	// test marker, so a failed refusal test cannot print host credentials.
	if len(os.Args) > 1 && os.Args[1] == "start" {
		env = nil
		for _, entry := range os.Environ() {
			if strings.HasPrefix(entry, "PARENT=") {
				env = append(env, entry)
			}
		}
	}
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
