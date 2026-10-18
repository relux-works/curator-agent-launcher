// frontdoor is a bounded, local-only Curator/provider double for binary tests.
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
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
