//go:build unix

package werkstatt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

type getOnlyStation struct{}

func (getOnlyStation) Get(context.Context, string) (*Record, error) { return nil, nil }

// HL24 / allocation qualification: file-only inventory is complete; every
// conflicting or unreadable record and a nonenumerable station refuses.
func TestLeaseAllocationInventory(t *testing.T) {
	for _, name := range []string{"same-record", "disjoint", "conflict", "invalid", "symlink", "get-only"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			v := recordValue(dir)
			selected, err := Parse(encoded(v), "example")
			if err != nil {
				t.Fatal(err)
			}
			s := Service{Loader: Loader{Directory: dir}}
			if err = os.WriteFile(filepath.Join(dir, "example.json"), encoded(v), 0600); err != nil {
				t.Fatal(err)
			}
			if name == "get-only" {
				s.Loader.Station = getOnlyStation{}
			} else if name != "same-record" {
				v["id"] = "other"
				if name == "disjoint" {
					v["profile"] = "other"
				}
				if name == "invalid" {
					v["extra"] = true
				}
				if err = os.WriteFile(filepath.Join(dir, "other.json"), encoded(v), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if name == "symlink" {
				path := filepath.Join(dir, "other.json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(dir, "example.json"), path); err != nil {
					t.Fatal(err)
				}
			}
			err = s.allocationInventory(context.Background(), selected)
			if name == "same-record" || name == "disjoint" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				code(t, err, "session_home_lease_unavailable")
				if err.(*client.Error).Details["reason"] != "allocation_conflict" {
					t.Fatal(err)
				}
			}
		})
	}
}
