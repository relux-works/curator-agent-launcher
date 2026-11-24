package defaults

import (
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

func TestDefaultsAndLineupSupportAntigravity(t *testing.T) {
	if err := Validate([]byte(`{
		"schema": "curator-run-defaults-v3",
		"defaults": {
			"antigravity": {"permissions": "native"}
		}
	}`)); err != nil {
		t.Fatalf("Validate antigravity defaults: %v", err)
	}
	reg, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := (Files{}).Complete(fragment.EnvAntigravity, Pair{}, reg)
	if err != nil {
		t.Fatalf("Complete(antigravity): %v", err)
	}
	if resolved.Runtime != "agy" || !resolved.Model.Present || resolved.Model.Value == "" {
		t.Fatalf("unexpected resolved antigravity defaults: %+v", resolved)
	}
}
