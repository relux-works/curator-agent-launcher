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
	"strings"
	"testing"
)

// This test executable doubles as a Go/gofmt process fixture. The real Go
// compiler runs the shipped collector; only its validation children are faked.
func TestMain(m *testing.M) {
	if os.Getenv("VALIDATE_PROCESS_FIXTURE") != "1" {
		os.Exit(m.Run())
	}
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "run" {
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
	if err := json.NewEncoder(f).Encode(args); err != nil {
		os.Exit(92)
	}
	f.Close()
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
					var a []string
					if err := json.Unmarshal(row, &a); err != nil {
						t.Fatal(err)
					}
					got = append(got, a)
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
