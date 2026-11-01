package execution_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/composition"
	"github.com/relux-works/curator-agent-launcher/internal/execution"
)

// The final environment callback runs after the late boundary, affects only the
// actual child env and cannot mutate the prepared snapshot used by a second run.
func TestLateChildEnvironment(t *testing.T) {
	dir := t.TempDir()
	env := []string{"KEEP=original"}
	l := execution.Prepare(composition.Value{Binary: helper, WorkDir: dir, Env: env, Argv: []string{"safe-argument"}})
	var out, stderr bytes.Buffer
	boundary := false
	calls := 0
	code := l.Run(execution.Options{IO: execution.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}, Boundary: func() error { boundary = true; return nil }, ChildEnvironment: func(child []string) ([]string, error) {
		calls++
		if !boundary {
			t.Fatal("lookup before boundary")
		}
		child[0] = "KEEP=child"
		return append(child, "CLAUDE_CODE_OAUTH_TOKEN=synthetic-only-child"), nil
	}})
	var got capture
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if code != 0 || calls != 1 || !reflect.DeepEqual(got.Env, []string{"KEEP=child", "CLAUDE_CODE_OAUTH_TOKEN=synthetic-only-child"}) || !reflect.DeepEqual(env, []string{"KEEP=original"}) {
		t.Fatal("late env isolation failed")
	}
	if strings.Contains(stderr.String(), "synthetic-only-child") || strings.Contains(strings.Join(got.Argv, " "), "synthetic-only-child") {
		t.Fatal("secret escaped child env")
	}
	code, again, _ := run(t, l, func() error { return nil })
	if code != 0 || !reflect.DeepEqual(again.Env, []string{"KEEP=original"}) {
		t.Fatal("prepared snapshot changed")
	}
}

// Missing binary, missing/failed boundary never resolve credentials; resolver
// failure produces no child/start callback and a fixed diagnostic, even with a hostile error.
func TestLateChildEnvironmentRefusals(t *testing.T) {
	for _, kind := range []string{"binary", "missing-boundary", "boundary", "resolver", "nil-env"} {
		t.Run(kind, func(t *testing.T) {
			v := composition.Value{Binary: helper, WorkDir: t.TempDir(), Env: []string{}}
			boundary := execution.Boundary(func() error { return nil })
			switch kind {
			case "binary":
				v.Binary = "/missing/provider"
			case "missing-boundary":
				boundary = nil
			case "boundary":
				boundary = func() error { return errors.New("plan_refused: late check failed") }
			}
			calls, starts := 0, 0
			var out, stderr bytes.Buffer
			code := execution.Prepare(v).Run(execution.Options{IO: execution.IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}, Boundary: boundary, Started: func() { starts++ }, ChildEnvironment: func([]string) ([]string, error) {
				calls++
				if kind == "nil-env" {
					return nil, nil
				}
				return []string{"TOKEN=private-error-plant"}, errors.New("private-error-plant")
			}})
			want := 0
			if kind == "resolver" || kind == "nil-env" {
				want = 1
			}
			if code == 0 || calls != want || starts != 0 || out.Len() != 0 || strings.Contains(stderr.String(), "private-error-plant") {
				t.Fatal("late failure escaped its boundary")
			}
			if (kind == "resolver" || kind == "nil-env") && !strings.Contains(stderr.String(), "final child environment was refused") {
				t.Fatal("wrong refusal")
			}
		})
	}
}
