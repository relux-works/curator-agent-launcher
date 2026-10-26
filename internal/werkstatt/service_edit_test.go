//go:build unix

package werkstatt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/werkstattcommand"
)

type editSpy struct {
	calls    []string
	requests []string
	session  client.Session
	reply    func(string, []byte) (client.Response, error)
}

func (s *editSpy) Call(_ context.Context, op string, request []byte) (client.Response, error) {
	s.calls = append(s.calls, op)
	s.requests = append(s.requests, op+" "+string(request))
	if s.reply != nil {
		return s.reply(op, request)
	}
	if op != "status" {
		return client.Response{}, fmt.Errorf("forbidden manager mutation: %s", op)
	}
	return client.Response{Session: &s.session}, nil
}
func (s *editSpy) ListAll(context.Context, map[string]string, int) ([]client.Session, error) {
	s.calls = append(s.calls, "list")
	return nil, fmt.Errorf("unexpected list")
}

func editService(t *testing.T) (Service, *editSpy, client.Liveness) {
	t.Helper()
	root := t.TempDir()
	config := filepath.Join(root, "records")
	state := filepath.Join(root, "state")
	for _, dir := range []string{config, state} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(config, "example.json"), encoded(recordValue(root)), 0600); err != nil {
		t.Fatal(err)
	}
	const stamp = "2026-01-01T00:00:00Z"
	op, task := "11111111-1111-4111-8111-111111111111", "previous"
	entry := sessionmanager.Entry{Executable: "/protocol", RegisteredAt: stamp}
	l := client.Liveness{ID: "example", State: "idle", StateSource: "manager", Manager: binding("demo", entry), Session: "SES-example", OperationID: &op, Generation: 1, StartedAt: stamp, LastHeartbeatAt: stamp, Doorbell: "off", CurrentTask: &task, QueueDepth: 2, AbandonedOperations: []client.Abandoned{}}
	spy := &editSpy{session: client.Session{Session: l.Session, OperationID: op, Generation: 1, State: "idle", Readable: true, StartedAt: stamp, Doorbell: client.Doorbell{State: "off"}}}
	s := Service{Loader: Loader{Directory: config}, Store: client.Store{Directory: state}, Now: func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }, Registry: func() (sessionmanager.Registry, error) {
		return sessionmanager.Registry{Managers: map[string]sessionmanager.Entry{"demo": entry}}, nil
	}, Client: func(string, sessionmanager.Entry) Remote { return spy }}
	return s, spy, l
}

// With a fixed clock and identical starting state, every new form and its alias
// have byte-identical stdout/liveness and calls; only the alias warns once.
// Abandon preserves its exact operator-override tombstone and removes the journal.
func TestEditRecoveryAliasParity(t *testing.T) {
	for _, tc := range []struct {
		name, action string
		flags        []string
		calls        []string
	}{
		{"set-task", "update", []string{"--set-task", "next"}, []string{"status"}},
		{"clear-task", "update", []string{"--clear-task"}, []string{"status"}},
		{"queue-depth", "update", []string{"--queue-depth", "0"}, []string{"status"}},
		{"combined-fields", "update", []string{"--set-task", "next", "--queue-depth", "5"}, []string{"status"}},
		{"rebind", "recover", []string{"--rebind-manager"}, []string{"status", "status"}},
		{"abandon", "recover", []string{"--abandon-operation"}, nil},
	} {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				s, spy, initial := editService(t)
				if tc.name == "rebind" {
					initial.Manager.Executable = "/previous"
				}
				livePath := filepath.Join(s.Store.Directory, "example.json")
				pendingPath := filepath.Join(s.Store.Directory, "example.op.json")
				var baselineOut, baselineLive []byte
				var baselineRequests []string
				for _, action := range []string{tc.action, "status"} {
					if err := os.WriteFile(livePath, encoded(initial), 0600); err != nil {
						t.Fatal(err)
					}
					if tc.name == "abandon" {
						p := client.Pending{OperationID: *initial.OperationID, Kind: "launch", SubmittedAt: initial.StartedAt, AdmissionKey: "werkstatt:example", Manager: initial.Manager}
						if err := os.WriteFile(pendingPath, encoded(p), 0600); err != nil {
							t.Fatal(err)
						}
					}
					flags := append([]string{action, "example"}, tc.flags...)
					if jsonOutput {
						flags = append(flags, "--json")
					}
					c, err := werkstattcommand.ParseCommand(flags)
					if err != nil {
						t.Fatal(err)
					}
					var out, stderr bytes.Buffer
					s.Stdout, s.Stderr = &out, &stderr
					spy.calls, spy.requests = nil, nil
					exit, err := s.Run(context.Background(), c)
					if exit != 0 || err != nil {
						t.Fatal(exit, err)
					}
					live, err := os.ReadFile(livePath)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(spy.calls, tc.calls) {
						t.Fatalf("manager calls=%v want=%v", spy.calls, tc.calls)
					}
					if action == tc.action {
						baselineOut, baselineLive = bytes.Clone(out.Bytes()), live
						baselineRequests = append([]string(nil), spy.requests...)
						if stderr.Len() != 0 {
							t.Fatalf("new form warned: %s", stderr.String())
						}
					} else {
						if !bytes.Equal(baselineOut, out.Bytes()) || !bytes.Equal(baselineLive, live) || !reflect.DeepEqual(baselineRequests, spy.requests) {
							t.Fatal("alias changed stdout or liveness")
						}
						want := "curator-run: deprecated: use 'werkstatt " + tc.action + " <id> …'\n"
						if stderr.String() != want {
							t.Fatalf("deprecation warning=%q want=%q", stderr.String(), want)
						}
					}
					var got client.Liveness
					if err := json.Unmarshal(live, &got); err != nil {
						t.Fatal(err)
					}
					switch tc.name {
					case "set-task", "combined-fields":
						if got.CurrentTask == nil || *got.CurrentTask != "next" {
							t.Fatal("task not set")
						}
					case "clear-task":
						if got.CurrentTask != nil {
							t.Fatal("task not cleared")
						}
					case "queue-depth":
						if got.QueueDepth != 0 {
							t.Fatal("queue not zero")
						}
					case "rebind":
						if got.Manager.Executable != "/protocol" || got.Manager.ID != initial.Manager.ID || got.Manager.RegisteredAt != initial.Manager.RegisteredAt {
							t.Fatal("binding changed unexpectedly")
						}
					case "abandon":
						if got.State != "down" || got.StateSource != "operator_override" || !reflect.DeepEqual(got.AbandonedOperations, []client.Abandoned{{OperationID: *initial.OperationID, AbandonedAt: "2026-01-02T00:00:00Z"}}) {
							t.Fatalf("override=%+v", got)
						}
						if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
							t.Fatal("journal survived", err)
						}
					}
				}
			})
		}
	}
}

// A dedicated command and its alias retain the same typed operational refusal,
// output, calls and unchanged state for missing liveness or mismatched ownership.
func TestEditRecoveryRefusalParity(t *testing.T) {
	for _, tc := range []struct {
		name, action string
		flags        []string
		want         string
	}{
		{"missing-liveness", "update", []string{"--set-task", "next"}, "werkstatt_not_up"},
		{"changed-owner", "update", []string{"--queue-depth", "3"}, "werkstatt_operation_unresolved"},
		{"wrong-session", "recover", []string{"--rebind-manager"}, "session_liveness_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, spy, initial := editService(t)
			if tc.name == "changed-owner" {
				initial.Manager.Executable = "/previous"
			}
			if tc.name == "wrong-session" {
				spy.session.Session = "SES-other"
			}
			path := filepath.Join(s.Store.Directory, "example.json")
			var original []byte
			if tc.name != "missing-liveness" {
				original = encoded(initial)
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
			}
			var firstOut, firstErr string
			var firstExit int
			var firstCalls []string
			for i, action := range []string{tc.action, "status"} {
				c, err := werkstattcommand.ParseCommand(append([]string{action, "example", "--json"}, tc.flags...))
				if err != nil {
					t.Fatal(err)
				}
				var out, stderr bytes.Buffer
				s.Stdout, s.Stderr = &out, &stderr
				spy.calls, spy.requests = nil, nil
				exit, err := s.Run(context.Background(), c)
				code(t, err, tc.want)
				after, readErr := os.ReadFile(path)
				if !bytes.Equal(original, after) || original == nil && !os.IsNotExist(readErr) {
					t.Fatal("refusal changed liveness", readErr)
				}
				if i == 0 {
					firstOut, firstErr, firstExit, firstCalls = out.String(), err.Error(), exit, append([]string(nil), spy.calls...)
					if stderr.Len() != 0 {
						t.Fatal("new form warned")
					}
				} else {
					if out.String() != firstOut || err.Error() != firstErr || exit != firstExit || !reflect.DeepEqual(firstCalls, spy.calls) {
						t.Fatal("alias changed refusal")
					}
					if stderr.String() != "curator-run: deprecated: use 'werkstatt "+tc.action+" <id> …'\n" {
						t.Fatal("alias warning", stderr.String())
					}
				}
			}
		})
	}
}

// Settled status/list may refresh manager evidence, but preserve router fields,
// binding and tombstones and invoke only status RPCs. This does not cover the
// separately specified pending-operation cancellation fence.
func TestStatusNoOperatorEdits(t *testing.T) {
	for _, action := range []string{"status-id", "status-all", "list"} {
		for _, state := range []string{"idle", "busy", "unknown"} {
			t.Run(action+"/"+state, func(t *testing.T) {
				s, spy, initial := editService(t)
				spy.session.State = state
				initial.AbandonedOperations = []client.Abandoned{{OperationID: "22222222-2222-4222-8222-222222222222", AbandonedAt: initial.StartedAt}}
				path := filepath.Join(s.Store.Directory, "example.json")
				if err := os.WriteFile(path, encoded(initial), 0600); err != nil {
					t.Fatal(err)
				}
				c := Command{Action: "status", JSON: true}
				if action == "status-id" {
					c.ID = "example"
				}
				if action == "list" {
					c.Action = "list"
				}
				var out, stderr bytes.Buffer
				s.Stdout, s.Stderr = &out, &stderr
				exit, err := s.Run(context.Background(), c)
				if err != nil || exit != 0 || stderr.Len() != 0 {
					t.Fatal(exit, err, stderr.String())
				}
				after, err := s.Store.Snapshot("example")
				if err != nil {
					t.Fatal(err)
				}
				if after.State != state || after.LastHeartbeatAt == initial.LastHeartbeatAt {
					t.Fatal("ordinary owner refresh lost", after)
				}
				preserved := *after
				preserved.State, preserved.LastHeartbeatAt = initial.State, initial.LastHeartbeatAt
				if !reflect.DeepEqual(preserved, initial) {
					t.Fatalf("status performed an operator edit: %+v", after)
				}
				if !reflect.DeepEqual(spy.calls, []string{"status"}) || !reflect.DeepEqual(spy.requests, []string{`status {"session":"SES-example"}`}) {
					t.Fatal("status called a mutating manager operation", spy.calls, spy.requests)
				}
			})
		}
	}
}

// Legacy mixed aliases retain rebind-before-edit and abandon-before-edit even
// when the later edit refuses; exactly one warning names both replacements.
func TestMixedStatusAliasOrdering(t *testing.T) {
	for _, recovery := range []string{"--rebind-manager", "--abandon-operation"} {
		t.Run(recovery, func(t *testing.T) {
			s, spy, initial := editService(t)
			path := filepath.Join(s.Store.Directory, "example.json")
			pending := filepath.Join(s.Store.Directory, "example.op.json")
			if recovery == "--rebind-manager" {
				initial.Manager.Executable = "/previous"
				if err := os.WriteFile(path, encoded(initial), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				p := client.Pending{OperationID: *initial.OperationID, Kind: "launch", SubmittedAt: initial.StartedAt, AdmissionKey: "werkstatt:example", Manager: initial.Manager}
				if err := os.WriteFile(pending, encoded(p), 0600); err != nil {
					t.Fatal(err)
				}
			}
			c, err := werkstattcommand.ParseCommand([]string{"status", "example", "--clear-task", recovery, "--json"})
			if err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			s.Stdout, s.Stderr = &out, &stderr
			exit, err := s.Run(context.Background(), c)
			if exit != 0 {
				t.Fatal(exit)
			}
			got, readErr := s.Store.Snapshot("example")
			if readErr != nil || got == nil {
				t.Fatal(got, readErr)
			}
			if recovery == "--rebind-manager" {
				if err != nil || got.Manager.Executable != "/protocol" || got.CurrentTask != nil || out.Len() == 0 || !reflect.DeepEqual(spy.calls, []string{"status", "status"}) {
					t.Fatal("rebind did not precede dispatch", got, err, spy.calls)
				}
			} else {
				code(t, err, "werkstatt_not_up")
				if got.StateSource != "operator_override" || got.State != "down" || got.Session != "" || !reflect.DeepEqual(got.AbandonedOperations, []client.Abandoned{{OperationID: *initial.OperationID, AbandonedAt: "2026-01-02T00:00:00Z"}}) || len(spy.calls) != 0 || out.Len() != 0 {
					t.Fatal("dispatch refusal preceded abandonment", got, spy.calls)
				}
				if _, err := os.Stat(pending); !os.IsNotExist(err) {
					t.Fatal("journal not abandoned", err)
				}
			}
			want := "curator-run: deprecated: use 'werkstatt update <id> …' and 'werkstatt recover <id> …'\n"
			if stderr.String() != want {
				t.Fatalf("mixed warning=%q want=%q", stderr.String(), want)
			}
		})
	}
}

// Only pending prepared/not_found reconciliation cancels: the request is fenced
// by its operation id, terminal cancellation clears the journal without operator
// edits, and the next settled status makes only a session-status read.
func TestStatusReconciliationCancelFence(t *testing.T) {
	for _, outcome := range []string{"prepared", "not_found"} {
		t.Run(outcome, func(t *testing.T) {
			s, spy, initial := editService(t)
			path := filepath.Join(s.Store.Directory, "example.json")
			journal := filepath.Join(s.Store.Directory, "example.op.json")
			original := encoded(initial)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			pending := client.Pending{OperationID: *initial.OperationID, Kind: "launch", SubmittedAt: initial.StartedAt, AdmissionKey: "werkstatt:example", Manager: initial.Manager}
			if err := os.WriteFile(journal, encoded(pending), 0600); err != nil {
				t.Fatal(err)
			}
			request := string(encoded(map[string]string{"operation_id": pending.OperationID}))
			spy.reply = func(op string, b []byte) (client.Response, error) {
				switch len(spy.calls) {
				case 1:
					if op != "status" || string(b) != request {
						t.Fatalf("unexpected pending read: %s %s", op, b)
					}
					return client.Response{Outcome: &client.Outcome{Outcome: outcome}}, nil
				case 2:
					if op != "cancel" || string(b) != request {
						t.Fatalf("cancel outside reconciliation fence: %s %s", op, b)
					}
					return client.Response{Outcome: &client.Outcome{Outcome: "cancelled"}}, nil
				case 3:
					if op != "status" || string(b) != `{"session":"SES-example"}` {
						t.Fatalf("settled status mutated manager: %s %s", op, b)
					}
					return client.Response{Session: &spy.session}, nil
				default:
					t.Fatalf("unexpected RPC: %s %s", op, b)
					return client.Response{}, nil
				}
			}
			var out, stderr bytes.Buffer
			s.Stdout, s.Stderr = &out, &stderr
			_, err := s.Run(context.Background(), Command{Action: "status", ID: "example", JSON: true})
			code(t, err, "session_host_unavailable")
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("reconciliation cancellation changed operator state", err)
			}
			if _, err := os.Stat(journal); !os.IsNotExist(err) {
				t.Fatal("terminal reconciliation retained journal", err)
			}
			exit, err := s.Run(context.Background(), Command{Action: "status", ID: "example", JSON: true})
			if err != nil || exit != 0 || stderr.Len() != 0 || !reflect.DeepEqual(spy.calls, []string{"status", "cancel", "status"}) {
				t.Fatal(exit, err, stderr.String(), spy.calls)
			}
		})
	}
}
