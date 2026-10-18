package werkstatt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

func recordValue(root string) map[string]any {
	return map[string]any{
		"schema": "werkstatt-record-v1", "id": "example", "project": "sample", "role": "developer", "runtime": "claude_code",
		"model": "claude-opus-5", "effort": "medium", "profile": "sample", "clone": root, "identity": "example.dev",
		"room": "#sample", "orchestrator": "example.orch", "sandbox": "none", "trust_class": "standard", "doorbell": nil,
	}
}
func encoded(v any) []byte {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return b
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	if !client.IsCode(err, want) {
		t.Fatalf("error=%v want=%s", err, want)
	}
}

// The editable record is closed, duplicate-safe and defaulted by context mode.
// Secret/reserved child inputs are rejected without exposing their values.
func TestRecord(t *testing.T) {
	for _, name := range []string{"valid", "fresh", "unknown", "id", "role", "alias", "relative", "weights", "doorbell", "secret-name", "secret-value", "reserved", "oversized", "duplicate", "missing-schema"} {
		t.Run(name, func(t *testing.T) {
			v := recordValue(t.TempDir())
			want := "werkstatt_record_invalid"
			switch name {
			case "valid", "fresh":
				want = ""
				if name == "fresh" {
					v["context_mode"] = "fresh"
				}
			case "unknown":
				v["unexpected"] = true
			case "id":
				v["id"] = "../escape"
			case "role":
				v["role"] = "owner"
			case "alias":
				v["runtime"] = "claude"
			case "relative":
				v["clone"] = "workspace"
			case "weights":
				v["weights_id"] = "bad"
			case "doorbell":
				v["doorbell"] = map[string]any{"heartbeat": "1s", "max_wakes_per_hour": 6}
			case "secret-name":
				v["child_env"] = map[string]string{"API_TOKEN": "sentinel"}
				want = "secret_policy_violation"
			case "secret-value":
				v["child_env"] = map[string]string{"VALUE": "ghp_sentinel"}
				want = "secret_policy_violation"
			case "reserved":
				v["child_env"] = map[string]string{"LD_PRELOAD": "sentinel"}
				want = "credential_reserved_name"
			case "oversized":
				v["child_env"] = map[string]string{"VALUE": string(make([]byte, 4097))}
				want = "secret_policy_violation"
			case "missing-schema":
				delete(v, "schema")
			}
			b := encoded(v)
			if name == "duplicate" {
				b = append([]byte(`{"id":"example",`), b[1:]...)
			}
			r, err := Parse(b, "example")
			if want != "" {
				code(t, err, want)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.ContextMode == "" || r.HarnessMemory == nil || *r.HarnessMemory != (name != "fresh") {
				t.Fatal("context defaults", r)
			}
		})
	}
}

// Loader uses the shared S1 security boundary; present unsafe input never falls
// through as absence. The neighboring ordinary file remains readable.
func TestLoader(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "example.json")
	l := Loader{Directory: root}
	_, _, err := l.Load(context.Background(), "example")
	code(t, err, "werkstatt_not_found")
	if err = os.WriteFile(p, encoded(recordValue(root)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = l.Load(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(p, 0666); err != nil {
		t.Fatal(err)
	}
	_, _, err = l.Load(context.Background(), "example")
	code(t, err, "werkstatt_record_invalid")
	if err = os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(root, "absent"), p); err != nil {
		t.Fatal(err)
	}
	_, _, err = l.Load(context.Background(), "example")
	code(t, err, "werkstatt_record_invalid")
}

// Workspace checking traverses lexical and physical strict ancestors, refuses
// unreadable directories, and never reads the planted credential contents.
func TestWorkspace(t *testing.T) {
	for _, name := range []string{"clean", "CLAUDE.md", "AGENTS.override.md", "physical", "lexical", "unreadable", "identity", "missing-clone", "git-missing", "git-inside"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			clone := filepath.Join(root, "physical", "clone")
			identities := filepath.Join(root, "identities")
			for _, p := range []string{clone, identities, filepath.Join(root, "lexical")} {
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(identities, "example.dev.json"), []byte("NOT_JSON_DO_NOT_READ"), 0600); err != nil {
				t.Fatal(err)
			}
			r, err := Parse(encoded(recordValue(clone)), "example")
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			switch name {
			case "CLAUDE.md", "AGENTS.override.md":
				_ = os.WriteFile(filepath.Join(root, "physical", name), []byte("inert"), 0600)
				want = "werkstatt_context_leak"
			case "physical", "lexical":
				alias := filepath.Join(root, "lexical", "clone")
				if err = os.Symlink(clone, alias); err != nil {
					t.Fatal(err)
				}
				r.Clone = alias
				ancestor := "physical"
				if name == "lexical" {
					ancestor = "lexical"
				}
				_ = os.WriteFile(filepath.Join(root, ancestor, "CLAUDE.local.md"), []byte("inert"), 0600)
				want = "werkstatt_context_leak"
			case "unreadable":
				p := filepath.Dir(clone)
				_ = os.Chmod(p, 0000)
				t.Cleanup(func() { _ = os.Chmod(p, 0700) })
				want = "werkstatt_context_check_failed"
			case "identity":
				_ = os.Remove(filepath.Join(identities, r.Identity+".json"))
				want = "werkstatt_identity_missing"
			case "missing-clone":
				r.Clone = filepath.Join(root, "absent")
				want = "werkstatt_clone_missing"
			case "git-missing":
				r.GitDir = filepath.Join(root, "absent")
				want = "werkstatt_git_layout_invalid"
			case "git-inside":
				r.GitDir = clone
				want = "werkstatt_git_layout_invalid"
			}
			err = Workspace(context.Background(), r, identities)
			if want != "" {
				code(t, err, want)
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
