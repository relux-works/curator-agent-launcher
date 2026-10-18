//go:build unix

package fakemanager

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

// A fresh invocation can stop an admitted no-op provider without losing the
// record shape; this checks the durable path as well as the emitted response.
func TestDurableLifecycle(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	r := LaunchTemplate(root)
	var out bytes.Buffer
	if exit := Run(root, []string{"launch"}, bytes.NewReader(encode(r)), nil, nil, &out); exit != 0 {
		t.Fatalf("launch %d %s", exit, out.Bytes())
	}
	var s client.Session
	_ = json.Unmarshal(out.Bytes(), &s)
	out.Reset()
	stopRequest := encode(map[string]any{"session": s.Session, "expected_generation": s.Generation, "mode": "force", "precondition": "none"})
	exit := Run(root, []string{"stop"}, bytes.NewReader(stopRequest), nil, nil, &out)
	if _, e := client.DecodeResponse("stop", stopRequest, out.Bytes(), exit, "fake"); e != nil {
		t.Fatalf("decode stop: %v exit %d body %s", e, exit, out.Bytes())
	}
	m, lock, e := open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	reply := m.operate("stop", request{Session: s.Session, ExpectedGeneration: s.Generation, Mode: "force", Precondition: "none"}, nil)
	if e := schema.Validate("session.schema.json", encode(reply.body)); e != nil {
		t.Fatalf("stop schema: %v body=%s", e, encode(reply.body))
	}
}

// Config files can remove capabilities without leaving advertised runtime cells;
// optional doorbell records remain schema-valid when their fake hook is enabled.
func TestConfiguredCapabilitiesAndDoorbell(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	cfg := DefaultConfig()
	cfg.Capabilities = append(cfg.Capabilities, "doorbell-codex/1")
	if e := sessionmanager.WriteAtomic(filepath.Join(root, "config.json"), encode(cfg)); e != nil {
		t.Fatal(e)
	}
	request := LaunchTemplate(root)
	request["doorbell"] = map[string]any{"identity": "worker", "heartbeat": "30s", "max_wakes_per_hour": 1}
	var out bytes.Buffer
	exit := Run(root, []string{"launch"}, bytes.NewReader(encode(request)), nil, nil, &out)
	response, e := client.DecodeResponse("launch", encode(request), out.Bytes(), exit, "fake")
	if e != nil {
		t.Fatal(e)
	}
	if response.Session.Doorbell.State != "armed" || response.Session.Doorbell.Generation == nil {
		t.Fatal("enabled fake doorbell was not armed")
	}
	cfg.Capabilities = []string{"status/1"}
	if e := sessionmanager.WriteAtomic(filepath.Join(root, "config.json"), encode(cfg)); e != nil {
		t.Fatal(e)
	}
	out.Reset()
	exit = Run(root, []string{"describe"}, bytes.NewReader([]byte("{}")), nil, nil, &out)
	response, e = client.DecodeResponse("describe", []byte("{}"), out.Bytes(), exit, "fake")
	if e != nil {
		t.Fatal(e)
	}
	for _, ops := range response.Describe.Runtimes {
		if !reflect.DeepEqual(ops, []string{"status"}) {
			t.Fatalf("disabled operations advertised: %v", ops)
		}
	}
	out.Reset()
	request["operation_id"] = uuid()
	exit = Run(root, []string{"launch"}, bytes.NewReader(encode(request)), nil, nil, &out)
	_, e = client.DecodeResponse("launch", encode(request), out.Bytes(), exit, "fake")
	if !client.IsCode(e, "session_host_capability_missing") {
		t.Fatalf("disabled launch accepted: %v", e)
	}
}

// Rejected state files are neither followed nor overwritten. A corrupt index is
// specifically unavailable, while valid state is owner-only and survives reopen.
func TestStateBoundaries(t *testing.T) {
	for _, kind := range []string{"valid", "symlink", "malformed", "incomplete", "index", "nil-index", "nil-labels", "cursor"} {
		t.Run(kind, func(t *testing.T) {
			root, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			var out bytes.Buffer
			r := LaunchTemplate(root)
			if Run(root, []string{"launch"}, bytes.NewReader(encode(r)), nil, nil, &out) != 0 {
				t.Fatal("positive admission failed")
			}
			path := filepath.Join(root, "state.json")
			b, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			st, e := os.Stat(path)
			if e != nil || st.Mode().Perm() != 0600 {
				t.Fatal("state must be owner-only")
			}
			var state State
			_ = json.Unmarshal(b, &state)
			want := "session_host_unavailable"
			switch kind {
			case "valid":
			case "symlink":
				target := filepath.Join(root, "saved.json")
				if e = os.Rename(path, target); e != nil {
					t.Fatal(e)
				}
				if e = os.Symlink(target, path); e != nil {
					t.Fatal(e)
				}
			case "malformed":
				b = []byte("{")
			case "incomplete":
				state.Operations = nil
				b = encode(state)
			case "nil-labels":
				for _, session := range state.Sessions {
					session.Labels = nil
				}
				b = encode(state)
			case "nil-index":
				state.Sessions = nil
				b = encode(state)
			case "index":
				for h := range state.Sessions {
					state.Sessions[h] = nil
				}
				b = encode(state)
			case "cursor":
				state.Snapshots["bad"] = Snapshot{Offset: -1}
				b = encode(state)
			}
			if kind != "valid" && kind != "symlink" {
				if e = os.WriteFile(path, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			out.Reset()
			exit := Run(root, []string{"list"}, bytes.NewReader([]byte("{}")), nil, nil, &out)
			response, e := client.DecodeResponse("list", []byte("{}"), out.Bytes(), exit, "fake")
			if kind == "valid" {
				if e != nil || len(response.Page.Sessions) != 1 {
					t.Fatalf("valid reopen: %v", e)
				}
			} else if !client.IsCode(e, want) {
				t.Fatalf("invalid state accepted: %v", e)
			}
			if kind == "index" || kind == "nil-index" || kind == "nil-labels" || kind == "malformed" {
				var typed *client.Error
				if !errors.As(e, &typed) || typed.Details["reason"] != "index_unreadable" {
					t.Fatal("corrupt index must refuse the whole page")
				}
			}
			after, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("read-only/rejected operation overwrote durable state")
			}
		})
	}
}

// Semantic JSON equality is grounded in the shared CCJ1 canonicalizer, so
// whitespace and member order cannot accidentally create a second provider.
func TestCanonicalReplay(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	request := LaunchTemplate(root)
	data := encode(request)
	var a, b bytes.Buffer
	if Run(root, []string{"launch"}, bytes.NewReader(data), nil, nil, &a) != 0 {
		t.Fatal("positive launch")
	}
	pretty, e := json.MarshalIndent(request, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if Run(root, []string{"launch"}, bytes.NewReader(pretty), nil, nil, &b) != 0 || !sameJSON(a.Bytes(), b.Bytes()) {
		t.Fatal("semantic replay changed the result")
	}
	m, lock, e := open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	if m.state.Starts != 1 {
		t.Fatal("semantic replay started a second provider")
	}
}

// Resume scope uses the same existing workspace through either symlink spelling;
// an unrelated directory remains a location mismatch (covered by conformance).
func TestCanonicalWorkspace(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(root, "workspace")
	if e = os.Mkdir(target, 0700); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "alias")
	if e = os.Symlink(target, link); e != nil {
		t.Fatal(e)
	}
	m, lock, e := open(filepath.Join(root, "manager"))
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	launch := LaunchTemplate(link)
	data := encode(launch)
	var req request
	_ = json.Unmarshal(data, &req)
	result := m.start("launch", req, data)
	if result.code != "" {
		t.Fatal(result.code)
	}
	row := result.body.(map[string]any)
	s := m.state.Sessions[row["session"].(string)]
	if s.CWD != target {
		t.Fatal("workspace binding is not canonical")
	}
	selector := request{Runtime: s.Runtime}
	selector.Selector.Kind = "handle"
	selector.Selector.Handle = s.Handle
	selector.Scope.CWD = target
	selected, code := m.selectResume(selector)
	if code != "" || selected != s {
		t.Fatal("canonical workspace was not selected")
	}
}

// Physical corruption has the same projection as injected unreadability. A
// valid JSON record for another handle/label set cannot become lifecycle evidence.
func TestUnreadableBindings(t *testing.T) {
	for _, kind := range []string{"json", "projection", "handle", "labels"} {
		t.Run(kind, func(t *testing.T) {
			root, e := filepath.EvalSymlinks(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			r := LaunchTemplate(root)
			var out bytes.Buffer
			exit := Run(root, []string{"launch"}, bytes.NewReader(encode(r)), nil, nil, &out)
			response, e := client.DecodeResponse("launch", encode(r), out.Bytes(), exit, "fake")
			if e != nil || !response.Session.Readable {
				t.Fatal("positive readable control")
			}
			handle := response.Session.Session
			path := filepath.Join(root, "state.json")
			data, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			var state State
			if json.Unmarshal(data, &state) != nil {
				t.Fatal("read state")
			}
			stored := state.Sessions[handle]
			var row map[string]any
			_ = json.Unmarshal(stored.Record, &row)
			switch kind {
			case "json":
				stored.Record = json.RawMessage(`"invalid record"`)
			case "projection":
				stored.Record = encode(projection(stored))
			case "handle":
				row["session"] = "SES-other"
				stored.Record = encode(row)
			case "labels":
				row["labels"] = map[string]string{"other": "value"}
				stored.Record = encode(row)
			}
			if e = sessionmanager.WriteAtomic(path, encode(state)); e != nil {
				t.Fatal(e)
			}
			out.Reset()
			exit = Run(root, []string{"list"}, bytes.NewReader([]byte("{}")), nil, nil, &out)
			response, e = client.DecodeResponse("list", []byte("{}"), out.Bytes(), exit, "fake")
			if e != nil || len(response.Page.Sessions) != 1 || response.Page.Sessions[0].Readable || response.Page.Sessions[0].Session != handle {
				t.Fatalf("corrupt record was not projected: %v", e)
			}
			stop := encode(map[string]any{"session": handle, "expected_generation": 1, "mode": "force", "precondition": "none"})
			out.Reset()
			exit = Run(root, []string{"stop"}, bytes.NewReader(stop), nil, nil, &out)
			_, e = client.DecodeResponse("stop", stop, out.Bytes(), exit, "fake")
			if !client.IsCode(e, "session_resume_unreadable") {
				t.Fatalf("unreadable lifecycle accepted: %v", e)
			}
		})
	}
}
