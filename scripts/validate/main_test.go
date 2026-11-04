package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type processTrace struct {
	Args   []string `json:"args"`
	GOOS   string   `json:"goos"`
	GOARCH string   `json:"goarch"`
}

// This test executable doubles as a Go/gofmt process fixture. The real Go
// compiler runs the shipped collector; only its validation children are faked.
func TestMain(m *testing.M) {
	if os.Getenv("VALIDATE_PROCESS_FIXTURE") != "1" {
		os.Exit(m.Run())
	}
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "run" || args[0] == "env") {
		cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), args...)
		cmd.Stdout, cmd.Stderr, cmd.Env = os.Stdout, os.Stderr, os.Environ()
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	f, err := os.OpenFile(os.Getenv("VALIDATE_TRACE"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(91)
	}
	if err := json.NewEncoder(f).Encode(processTrace{Args: args, GOOS: os.Getenv("GOOS"), GOARCH: os.Getenv("GOARCH")}); err != nil {
		os.Exit(92)
	}
	f.Close()
	if output := os.Getenv("VALIDATE_FORMAT_OUTPUT"); output != "" {
		fmt.Fprint(os.Stdout, output)
	}
	if code := os.Getenv("VALIDATE_FORMAT_EXIT"); code != "" {
		n, err := strconv.Atoi(code)
		if err != nil {
			os.Exit(93)
		}
		os.Exit(n)
	}
	if os.Getenv("VALIDATE_RED_ROOT") == "1" && len(args) > 0 && args[0] != "-C" {
		os.Exit(17)
	}
	os.Exit(0)
}

// A failed or unavailable earlier lane must not suppress later selected lanes;
// the final summary names every verdict and the aggregate cannot report success.
func TestCollectAllVerdicts(t *testing.T) {
	for _, state := range []string{"PASS", "FAIL", "UNAVAILABLE"} {
		t.Run(state, func(t *testing.T) {
			var seen []string
			var out, errOut bytes.Buffer
			rc := collect([]lane{{name: "first"}, {name: "second"}, {name: "third"}}, &out, &errOut,
				func(l lane, _, _ io.Writer) (string, error) {
					seen = append(seen, l.name)
					if l.name == "first" && state != "PASS" {
						return state, errors.New("fixture")
					}
					return "PASS", nil
				})
			if !reflect.DeepEqual(seen, []string{"first", "second", "third"}) {
				t.Fatalf("selected lanes lost: %v", seen)
			}
			want := 0
			if state != "PASS" {
				want = 1
			}
			if rc != want {
				t.Fatalf("aggregate=%d, want %d", rc, want)
			}
			for _, verdict := range []string{state + " first", "PASS second", "PASS third"} {
				if !strings.Contains(out.String(), verdict+"\n") {
					t.Fatalf("missing %s in %s", verdict, &out)
				}
			}
		})
	}
}

// The non-race hosted selection includes every original job check, while check
// adds both race modules. Selection errors execute no validation lane.
func TestSelections(t *testing.T) {
	for target, want := range map[string][]string{
		"verify": {"build/root", "build/schema", "fmt-check", "vet/root", "vet/schema", "test/root", "test/schema", "release-gate-test"},
		"check":  {"build/root", "build/schema", "fmt-check", "vet/root", "vet/schema", "test/root", "test/schema", "race/root", "race/schema", "release-gate-test"},
	} {
		selected, err := lanes(target, "go", "gofmt")
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, l := range selected {
			got = append(got, l.name)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s=%v, want %v", target, got, want)
		}
	}
	if selected, err := lanes("unknown", "go", "gofmt"); err == nil || len(selected) != 0 {
		t.Fatal("unknown selection admitted")
	}
}

// Exercise the canonical Make entry, real collector executable and real process
// exits: a red root still runs the schema command, preserving all original flags.
// These are orchestration controls, not evidence that the product suites pass.
func TestMakeModuleLanes(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for target, args := range map[string][]string{
		"build": {"build", "./..."}, "vet": {"vet", "./..."},
		"test": {"test", "./...", "-count=1", "-timeout=13m"},
		"race": {"test", "./...", "-count=1", "-race", "-timeout=12m"},
	} {
		for _, red := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/red=%v", target, red), func(t *testing.T) {
				trace := filepath.Join(t.TempDir(), "trace.jsonl")
				cmd := exec.Command("make", "--no-print-directory", "GO="+executable, target)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "VALIDATE_PROCESS_FIXTURE=1", "VALIDATE_TRACE="+trace, fmt.Sprintf("VALIDATE_RED_ROOT=%d", map[bool]int{false: 0, true: 1}[red]))
				output, runErr := cmd.CombinedOutput()
				if (runErr != nil) != red {
					t.Fatalf("make error=%v red=%v\n%s", runErr, red, output)
				}
				data, err := os.ReadFile(trace)
				if err != nil {
					t.Fatalf("trace: %v\n%s", err, output)
				}
				var got [][]string
				for _, row := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
					var a processTrace
					if err := json.Unmarshal(row, &a); err != nil {
						t.Fatal(err)
					}
					got = append(got, a.Args)
				}
				schema := append([]string{"-C", "schemas/session-manager"}, args...)
				if target == "test" {
					schema = schema[:len(schema)-1]
				}
				if !reflect.DeepEqual(got, [][]string{args, schema}) {
					t.Fatalf("missing or changed lanes: %v, want root %v then schema %v\n%s", got, args, schema, output)
				}
				first := "PASS"
				if red {
					first = "FAIL"
				}
				for _, verdict := range []string{first + " " + target + "/root", "PASS " + target + "/schema"} {
					if !bytes.Contains(output, []byte(verdict+"\n")) {
						t.Fatalf("missing %s\n%s", verdict, output)
					}
				}
			})
		}
	}
}

// A missing tool must be reported as unavailable, not as a passing empty lane.
func TestMissingTool(t *testing.T) {
	var out bytes.Buffer
	state, err := execute(lane{command: filepath.Join(t.TempDir(), "missing")}, &out, &out)
	if state != "UNAVAILABLE" || err == nil {
		t.Fatalf("state=%s error=%v", state, err)
	}
}

// The Make entry compiles a host executable even with a foreign target, and
// forwards the caller's target to all ten selected child processes. The release
// child is also an inert Go fixture; this proves orchestration, not cross-tests.
func TestMakeTargetEnvironment(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	foreignOS, foreignArch := "linux", "amd64"
	if runtime.GOOS == foreignOS {
		foreignOS, foreignArch = "darwin", "arm64"
	}
	for _, tc := range []struct {
		name, goos, goarch string
		commandLine        bool
	}{
		{"default", "", "", false},
		{"goenv-default", "", "", false},
		{"host", runtime.GOOS, runtime.GOARCH, true},
		{"foreign-environment", foreignOS, foreignArch, false},
		{"os-only", foreignOS, "", true},
		{"arch-only", "", foreignArch, false},
		{"foreign-command-line", foreignOS, foreignArch, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := t.TempDir()
			if err := os.MkdirAll(filepath.Join(fixture, "scripts", "validate"), 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"Makefile", "scripts/validate/main.go"} {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(fixture, name), data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(fixture, "go.mod"), []byte("module validate-fixture\n\ngo 1.24.0\n"), 0644); err != nil {
				t.Fatal(err)
			}
			binary, err := os.ReadFile(executable)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(fixture, "scripts", "release-gate-test.sh"), binary, 0755); err != nil {
				t.Fatal(err)
			}
			trace := filepath.Join(fixture, "trace.jsonl")
			args := []string{"--no-print-directory", "GO=" + executable, "GOFMT=" + executable, "check"}
			envOS, envArch := tc.goos, tc.goarch
			if tc.commandLine {
				// A command-line target must win over a conflicting inherited target.
				envOS, envArch = "windows", "386"
				args = append(args, "GOOS="+tc.goos, "GOARCH="+tc.goarch)
			}
			cmd := exec.Command("make", args...)
			cmd.Dir = fixture
			if tc.name == "goenv-default" {
				targetConfig := filepath.Join(fixture, "goenv")
				if err := os.WriteFile(targetConfig, []byte("GOOS="+foreignOS+"\nGOARCH="+foreignArch+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
				cmd.Env = append(os.Environ(), "GOENV="+targetConfig)
			} else {
				cmd.Env = os.Environ()
			}
			cmd.Env = append(cmd.Env, "GOWORK=off", "GOOS="+envOS, "GOARCH="+envArch, "VALIDATE_PROCESS_FIXTURE=1", "VALIDATE_TRACE="+trace, "VALIDATE_RED_ROOT=0", "VALIDATE_FORMAT_OUTPUT=", "VALIDATE_FORMAT_EXIT=")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("host collector did not execute: %v\n%s", err, output)
			}
			data, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			rows := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
			if len(rows) != 10 {
				t.Fatalf("selected child count=%d, want 10\n%s", len(rows), output)
			}
			for _, row := range rows {
				var got processTrace
				if err := json.Unmarshal(row, &got); err != nil {
					t.Fatal(err)
				}
				if got.GOOS != tc.goos || got.GOARCH != tc.goarch {
					t.Errorf("child %v target=%s/%s, want %s/%s", got.Args, got.GOOS, got.GOARCH, tc.goos, tc.goarch)
				}
			}
		})
	}
}

// A format process must reject both a listed file and a real nonzero exit;
// the empty successful process prevents a reject-all implementation passing.
func TestFormatExecution(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, output, exit, state string }{
		{"files", "cmd/bad.go\ninternal/also-bad.go\n", "0", "FAIL"},
		{"tool-error", "", "19", "FAIL"},
		{"clean", "", "0", "PASS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VALIDATE_PROCESS_FIXTURE", "1")
			t.Setenv("VALIDATE_TRACE", filepath.Join(t.TempDir(), "trace.jsonl"))
			t.Setenv("VALIDATE_FORMAT_OUTPUT", tc.output)
			t.Setenv("VALIDATE_FORMAT_EXIT", tc.exit)
			selected, err := lanes("fmt-check", "unused-go", executable)
			if err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			state, err := execute(selected[0], &out, &errOut)
			if state != tc.state || (err != nil) != (tc.state != "PASS") {
				t.Fatalf("state=%s error=%v; want %s", state, err, tc.state)
			}
			if tc.output != "" && !strings.Contains(out.String(), "gofmt: files need formatting:\n"+tc.output) {
				t.Fatalf("missing offending file list: %q", out.String())
			}
			if tc.exit != "0" {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 19 {
					t.Fatalf("wrong format failure: %v", err)
				}
			}
		})
	}
}
