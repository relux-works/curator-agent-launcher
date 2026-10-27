//go:build unix

package werkstatt

import (
	"context"
	"strings"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

// A Get-only station cannot establish a complete allocation inventory. The
// production file loader enumerates and validates every record before deciding.
func (s Service) allocationInventory(ctx context.Context, selected *Record) error {
	conflict := func() error {
		return &client.Error{Code: "session_home_lease_unavailable", Details: map[string]string{"reason": "allocation_conflict"}}
	}
	if s.Loader.Station != nil {
		return conflict()
	}
	entries, err := configfile.ReadDirectory(s.Loader.Directory)
	if err != nil {
		return conflict()
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		r, _, e := s.Loader.Load(ctx, id)
		if e != nil {
			return conflict()
		}
		if r.ID != selected.ID && r.Runtime == selected.Runtime && r.Profile == selected.Profile {
			return conflict()
		}
	}
	return nil
}
