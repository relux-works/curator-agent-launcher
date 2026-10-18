// frontdoor is a bounded, local-only Curator/provider double for binary tests.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed provider-help.txt
var providerHelp string

func main() {
	if filepath.Base(os.Args[0]) == "a2a" {
		if len(os.Args) > 2 && os.Args[1] == "hook" {
			hookInstall()
			return
		}
		if len(os.Args) != 5 || os.Args[1] != "who" || os.Args[2] != "--stale-after" || os.Args[4] != "--json" {
			os.Exit(2)
		}
		d, err := time.ParseDuration(os.Args[3])
		if err != nil || d <= 0 {
			os.Exit(2)
		}
		b, _ := json.Marshal(os.Args[1:])
		if os.WriteFile("a2a-argv.json", b, 0600) != nil {
			os.Exit(1)
		}
		if _, err := os.Stat("a2a-who-fail"); err == nil {
			os.Exit(1)
		}
		if data, err := os.ReadFile("a2a-who.json"); err == nil {
			_, _ = os.Stdout.Write(data)
		} else {
			fmt.Println("[]")
		}
		return
	}
	if filepath.Base(os.Args[0]) == "curator" {
		data, err := os.ReadFile("fragment.json")
		if err != nil {
			os.Exit(1)
		}
		os.Stdout.Write(data)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		if filepath.Base(os.Args[0]) == "muse" {
			fmt.Println("Muse Code 1.4.7 (1.4.7-R6101.1)")
		} else if filepath.Base(os.Args[0]) == "codex" {
			fmt.Println("codex-cli 0.153.2")
		} else {
			fmt.Println("2.1.261")
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Print(providerHelp)
		return
	}
	capture, _ := json.Marshal(map[string]any{"Argv": os.Args[1:], "Env": os.Environ()})
	if err := os.WriteFile("native.json", capture, 0600); err != nil {
		os.Exit(1)
	}
	if os.WriteFile("native-called", []byte("called"), 0600) != nil {
		os.Exit(1)
	}
	fmt.Println("native fixture")
}

// Hook installation is a bounded fixture operation inside the test workspace.
// It preserves unrelated settings and records helper PID/group evidence.
func hookInstall() {
	if len(os.Args) != 9 || os.Args[2] != "install" || os.Args[3] != "--agent" || os.Args[5] != "--runtime" || os.Args[6] != "claude" || os.Args[7] != "--settings" {
		os.Exit(2)
	}
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	group, _ := syscall.Getpgid(0)
	evidence, _ := json.Marshal(map[string]any{"argv": os.Args[1:], "pid": os.Getpid(), "group": group})
	if os.WriteFile("a2a-hook.json", evidence, 0600) != nil {
		os.Exit(1)
	}
	mode, _ := os.ReadFile("a2a-hook-mode")
	if string(mode) == "fail" {
		fmt.Fprintln(os.Stderr, "UNTRUSTED_INSTALL_OUTPUT")
		os.Exit(1)
	}
	if string(mode) == "timeout" {
		time.Sleep(30 * time.Second)
		return
	}
	settings := map[string]any{}
	if old, err := os.ReadFile(os.Args[8]); err == nil {
		json.Unmarshal(old, &settings)
	}
	env, _ := settings["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	env["A2A_AGENT"] = os.Args[4]
	if string(mode) == "wrong-agent" {
		env["A2A_AGENT"] = "other.dev"
	}
	settings["env"] = env
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, sub := range map[string]string{"SessionStart": "session-start", "UserPromptSubmit": "prompt", "Stop": "stop"} {
		if string(mode) == "missing-stop" && event == "Stop" {
			continue
		}
		command := strings.Join([]string{executable, "--as", os.Args[4], "hook", sub, "--runtime", "claude"}, " ")
		typ := "command"
		if string(mode) == "wrong-command" && event == "Stop" {
			command = strings.Replace(command, "hook stop", "hook prompt", 1)
		}
		if string(mode) == "extra-argument" && event == "Stop" {
			command += " --extra"
		}
		if string(mode) == "wrong-type" && event == "Stop" {
			typ = "prompt"
		}
		if string(mode) == "wrong-executable" && event == "Stop" {
			// Keep the wrong executable real so rejection proves path identity.
			wrong := filepath.Join(filepath.Dir(os.Args[8]), "wrong-a2a")
			if err := os.MkdirAll(filepath.Dir(wrong), 0700); err != nil {
				os.Exit(1)
			}
			if err := os.WriteFile(wrong, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				os.Exit(1)
			}
			command = strings.Replace(command, executable, wrong, 1)
		}
		entries, _ := hooks[event].([]any)
		hooks[event] = append(entries, map[string]any{"hooks": []any{map[string]any{"type": typ, "command": command}}})
	}
	settings["hooks"] = hooks
	data, _ := json.Marshal(settings)
	if string(mode) == "malformed" {
		data = []byte("{")
	}
	if err := os.MkdirAll(filepath.Dir(os.Args[8]), 0700); err != nil {
		os.Exit(1)
	}
	if string(mode) == "fifo" {
		if syscall.Mkfifo(os.Args[8], 0600) != nil {
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(os.Args[8], data, 0600); err != nil {
		os.Exit(1)
	}
	switch string(mode) {
	case "post-unsafe-directory":
		if os.Chmod(filepath.Dir(os.Args[8]), 0777) != nil {
			os.Exit(1)
		}
	case "post-hard-link":
		if os.Link(os.Args[8], "a2a-shared-settings") != nil {
			os.Exit(1)
		}
	case "post-symlink":
		if os.Rename(os.Args[8], os.Args[8]+".target") != nil || os.Symlink(os.Args[8]+".target", os.Args[8]) != nil {
			os.Exit(1)
		}
	}
}
