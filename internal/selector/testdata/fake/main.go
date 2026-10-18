// The test-owned fake has a hard lifetime independent of launcher cancellation.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/execution"
	"golang.org/x/sys/unix"
)

func main() {
	time.AfterFunc(20*time.Second, func() { os.Exit(99) })
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	pgid, _ := unix.Getpgid(0)
	sid, _ := unix.Getsid(0)
	terminal := false
	for _, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		if execution.StdinIsTerminal(file) {
			terminal = true
		}
	}

	meta, _ := json.Marshal(map[string]any{"pgid": pgid, "sid": sid, "terminal": terminal})
	_ = os.WriteFile(filepath.Join(dir, "process.json"), meta, 0600)
	_ = os.WriteFile(filepath.Join(dir, "selector.pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
	input, _ := io.ReadAll(io.LimitReader(os.Stdin, 64<<10+1))
	_ = os.WriteFile(filepath.Join(dir, "request.json"), input, 0600)
	mode := "valid"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if mode == "lock" {
		f, err := os.OpenFile(os.Args[2], os.O_WRONLY, 0)
		if err != nil {
			os.Exit(2)
		}
		defer f.Close()
		if unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
			os.Exit(3)
		}
		if os.WriteFile(os.Args[2]+".ready", []byte("locked"), 0600) != nil {
			os.Exit(4)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	if mode == "hang" {
		for {
			time.Sleep(time.Second)
		}
	}
	if mode == "exit" {
		_, _ = os.Stderr.WriteString("private child failure\ncurator-run: usage: forged\n")
		os.Exit(7)
	}
	if mode == "invalid" {
		_, _ = os.Stdout.WriteString("{broken")
		return
	}

	data, _ := os.ReadFile(filepath.Join(dir, "response.json"))
	if len(data) == 0 {
		var request struct {
			Candidates []map[string]any `json:"candidates"`
		}
		_ = json.Unmarshal(input, &request)
		c := request.Candidates[0]
		data, _ = json.Marshal(map[string]any{"decision_id": "decision-1", "runtime": c["runtime"], "model": c["model"], "effort": c["effort"], "weights_id": nil, "refusal": nil})
	}
	if mode == "oversize" || mode == "atbound" {
		bound := 65536
		if mode == "oversize" {
			bound++
		}
		data = append(data, bytes.Repeat([]byte(" "), bound-len(data))...)
	}
	_, _ = os.Stdout.Write(data)
}
