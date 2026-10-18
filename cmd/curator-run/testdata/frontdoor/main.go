// frontdoor is a bounded, local-only Curator/provider double for binary tests.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if filepath.Base(os.Args[0]) == "a2a" {
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
		if filepath.Base(os.Args[0]) == "codex" {
			fmt.Println("codex-cli 0.153.2")
		} else {
			fmt.Println("2.1.261")
		}
		return
	}
	if os.WriteFile("native-called", []byte("called"), 0600) != nil {
		os.Exit(1)
	}
	fmt.Println("native fixture")
}
