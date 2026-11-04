// Command validate runs every selected validation lane and reports their verdicts.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type lane struct {
	name    string
	command string
	args    []string
	format  bool
}

func lanes(target, goTool, gofmt string) ([]lane, error) {
	module := func(name string, args ...string) []lane {
		return []lane{
			{name: name + "/root", command: goTool, args: args},
			{name: name + "/schema", command: goTool, args: append([]string{"-C", "schemas/session-manager"}, args...)},
		}
	}
	build := module("build", "build", "./...")
	format := []lane{{name: "fmt-check", command: gofmt, args: []string{"-l", "cmd", "internal", "schemas", "scripts/validate"}, format: true}}
	vet := module("vet", "vet", "./...")
	test := module("test", "test", "./...", "-count=1", "-timeout=13m")
	// Preserve the nested module's existing ordinary-test timeout default.
	test[1].args = test[1].args[:len(test[1].args)-1]
	race := module("race", "test", "./...", "-count=1", "-race", "-timeout=12m")
	release := []lane{{name: "release-gate-test", command: "scripts/release-gate-test.sh"}}
	var result []lane
	add := func(groups ...[]lane) {
		for _, group := range groups {
			result = append(result, group...)
		}
	}
	switch target {
	case "build":
		add(build)
	case "fmt-check":
		add(format)
	case "vet":
		add(vet)
	case "test":
		add(test)
	case "race":
		add(race)
	case "verify":
		add(build, format, vet, test, release)
	case "check":
		add(build, format, vet, test, race, release)
	default:
		return nil, fmt.Errorf("unknown validation target %q", target)
	}
	return result, nil
}

// execute distinguishes a tool that could not start from one that ran and failed.
func execute(l lane, out, errOut io.Writer) (string, error) {
	if l.name == "release-gate-test" {
		if err := os.MkdirAll(".temp", 0755); err != nil {
			return "UNAVAILABLE", err
		}
	}
	cmd := exec.Command(l.command, l.args...)
	cmd.Stderr = errOut
	if !l.format {
		cmd.Stdout = out
		if err := cmd.Run(); err != nil {
			if _, ok := err.(*exec.ExitError); ok {
				return "FAIL", err
			}
			return "UNAVAILABLE", err
		}
		return "PASS", nil
	}
	var formatted strings.Builder
	cmd.Stdout = &formatted
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return "FAIL", err
		}
		return "UNAVAILABLE", err
	}
	if formatted.Len() != 0 {
		fmt.Fprintln(out, "gofmt: files need formatting:")
		fmt.Fprint(out, formatted.String())
		return "FAIL", fmt.Errorf("unformatted Go files")
	}
	return "PASS", nil
}

type executor func(lane, io.Writer, io.Writer) (string, error)

func collect(selected []lane, out, errOut io.Writer, run executor) int {
	failed := false
	type verdict struct{ name, state string }
	results := make([]verdict, 0, len(selected))
	for _, l := range selected {
		fmt.Fprintf(out, "=== %s: %s %s\n", l.name, l.command, strings.Join(l.args, " "))
		state, err := run(l, out, errOut)
		results = append(results, verdict{l.name, state})
		if err != nil || state != "PASS" {
			failed = true
			fmt.Fprintf(errOut, "%s: %s: %v\n", state, l.name, err)
		}
	}
	fmt.Fprintln(out, "=== Validation results")
	for _, result := range results {
		fmt.Fprintf(out, "%s %s\n", result.state, result.name)
	}
	if failed {
		return 1
	}
	return 0
}

func main() {
	target := flag.String("target", "", "build, fmt-check, vet, test, race, verify or check")
	goTool := flag.String("go", "go", "Go executable")
	gofmt := flag.String("gofmt", "gofmt", "gofmt executable")
	targetEnv := flag.Bool("target-env", false, "restore Make target environment for validation children")
	flag.Parse()
	selected, err := lanes(*target, *goTool, *gofmt)
	if err != nil || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "validation setup:", err, "unexpected positional arguments:", flag.Args())
		os.Exit(2)
	}
	if *targetEnv {
		for _, name := range []string{"GOOS", "GOARCH"} {
			if err := os.Setenv(name, os.Getenv("VALIDATE_"+name)); err != nil {
				fmt.Fprintln(os.Stderr, "validation target environment:", err)
				os.Exit(2)
			}
		}
	}
	os.Exit(collect(selected, os.Stdout, os.Stderr, execute))
}
