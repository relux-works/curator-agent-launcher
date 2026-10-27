//go:build unix

package werkstatt

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

// A restarted up settles submitted profile evidence before consulting an edited
// record. Old journals clear previous evidence rather than guessing a profile.
func TestRecoveredGenerationProfile(t *testing.T) {
	for _, profile := range []string{"submitted", ""} {
		t.Run("journal-"+profile, func(t *testing.T) {
			s, spy, old := editService(t)
			old.Profile = "previous"
			old.State = "down"
			livePath := filepath.Join(s.Store.Directory, "example.json")
			if err := os.WriteFile(livePath, encoded(old), 0600); err != nil {
				t.Fatal(err)
			}
			operation := "22222222-2222-4222-8222-222222222222"
			p := client.Pending{Profile: profile, OperationID: operation, Kind: "launch", SubmittedAt: old.StartedAt, AdmissionKey: "werkstatt:example", Manager: old.Manager}
			if err := os.WriteFile(filepath.Join(s.Store.Directory, "example.op.json"), encoded(p), 0600); err != nil {
				t.Fatal(err)
			}
			record := recordValue(t.TempDir())
			record["profile"] = "edited"
			if err := os.WriteFile(filepath.Join(s.Loader.Directory, "example.json"), encoded(record), 0600); err != nil {
				t.Fatal(err)
			}
			spy.session.OperationID = operation
			spy.session.Generation = 2
			spy.reply = func(op string, _ []byte) (client.Response, error) {
				if op != "status" {
					t.Fatalf("recovery reached %s", op)
				}
				return client.Response{Outcome: &client.Outcome{Outcome: "started", Session: &spy.session}}, nil
			}
			var out, stderr bytes.Buffer
			s.Stdout, s.Stderr = &out, &stderr
			exit, err := s.Run(context.Background(), Command{Action: "up", ID: "example"})
			if err != nil || exit != 0 || !reflect.DeepEqual(spy.calls, []string{"status"}) {
				t.Fatal(exit, err, spy.calls)
			}
			b, err := os.ReadFile(livePath)
			if err != nil {
				t.Fatal(err)
			}
			var l client.Liveness
			if json.Unmarshal(b, &l) != nil || l.Profile != profile || l.Generation != 2 {
				t.Fatal("profile inferred from current record or previous generation", string(b))
			}
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(b, &fields)
			if _, present := fields["profile"]; present != (profile != "") {
				t.Fatal("unknown profile must be absent", string(b))
			}
		})
	}
}
