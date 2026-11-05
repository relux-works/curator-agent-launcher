package execution_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
)

// The final callback sees the resolved executable and post-injection env. Its
// detached argv/env copies cannot mutate the actual child; a refusal starts no
// helper and emits no arbitrary callback error or synthetic credential.
func TestFinalProcessBoundary(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(map[bool]string{false: "accept", true: "refuse"}[refuse], func(t *testing.T) {
			var out, diag bytes.Buffer
			var returned []string
			called, started := false, false
			value := composition.Value{Binary: helper, WorkDir: t.TempDir(), Argv: []string{"kept"}, Env: []string{"ORIGINAL=yes"}}
			code := execution.Prepare(value).Run(execution.Options{IO: execution.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &diag}, Boundary: func() error { return nil }, Started: func() { started = true },
				ChildEnvironment: func(env []string) ([]string, error) {
					returned = append(env, "SYNTHETIC_SECRET=child-only")
					return returned, nil
				},
				BeforeStart: func(binary string, argv, env []string) error {
					called = true
					expected, _ := filepath.Abs(helper)
					if binary != expected || !reflect.DeepEqual(argv, []string{"kept"}) || !reflect.DeepEqual(env, []string{"ORIGINAL=yes", "SYNTHETIC_SECRET=child-only"}) {
						t.Fatal("wrong final process projection")
					}
					argv[0] = "changed"
					env[0] = "CHANGED=yes"
					returned[0] = "RETAINED_ALIAS=yes"
					if refuse {
						return errors.New("child-only callback payload")
					}
					return nil
				},
			})
			if !called {
				t.Fatal("final boundary skipped")
			}
			if refuse {
				if code == 0 || started || out.Len() != 0 {
					t.Fatal("refused helper started")
				}
				if strings.Contains(diag.String(), "child-only") {
					t.Fatal("callback payload leaked")
				}
				return
			}
			if code != 0 || !started {
				t.Fatalf("valid helper failed: %d %s", code, diag.String())
			}
			var got struct{ Argv, Env []string }
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Argv, []string{"kept"}) || !reflect.DeepEqual(got.Env, []string{"ORIGINAL=yes", "SYNTHETIC_SECRET=child-only"}) {
				t.Fatalf("callback mutated actual child: %#v", got)
			}
		})
	}
}

// A declined environment never reaches final verification or process start.
func TestFinalProcessBoundaryEnvironmentRefusal(t *testing.T) {
	var out, diag bytes.Buffer
	called, started := false, false
	code := execution.Prepare(composition.Value{Binary: helper, WorkDir: t.TempDir(), Env: []string{"PATH=" + filepath.Dir(helper)}}).Run(execution.Options{IO: execution.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &diag}, Boundary: func() error { return nil }, Started: func() { started = true }, ChildEnvironment: func([]string) ([]string, error) { return nil, os.ErrPermission }, BeforeStart: func(string, []string, []string) error { called = true; return nil }})
	if code == 0 || called || started || out.Len() != 0 {
		t.Fatal("environment refusal reached child")
	}
}
