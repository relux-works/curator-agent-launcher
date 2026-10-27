// The isolated fake emits scripted published envelopes and records every call.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

type Step struct {
	Operation string
	Body      json.RawMessage
	Exit      int
}

func main() {
	dir := os.Getenv("LEASE_FIXTURE")
	args := os.Args[1:]
	f, e := os.OpenFile(filepath.Join(dir, "trace"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		os.Exit(99)
	}
	_ = json.NewEncoder(f).Encode(struct {
		PID  int
		Args []string
	}{os.Getpid(), args})
	f.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "plan"))
	var steps []Step
	_ = json.Unmarshal(b, &steps)
	raw, _ := os.ReadFile(filepath.Join(dir, "index"))
	i, _ := strconv.Atoi(string(raw))
	if len(args) < 3 || i >= len(steps) || args[2] != steps[i].Operation {
		os.Exit(98)
	}
	if os.WriteFile(filepath.Join(dir, "index"), []byte(strconv.Itoa(i+1)), 0600) != nil {
		os.Exit(97)
	}
	if string(steps[i].Body) != "null" {
		os.Stdout.Write(steps[i].Body)
	}
	os.Stderr.WriteString("untrusted stderr must never escape")
	os.Exit(steps[i].Exit)
}
