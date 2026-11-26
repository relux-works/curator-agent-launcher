package defaults

import (
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

func TestDefaultsAndLineupSupportKimiCode(t *testing.T) {
	if err := Validate([]byte(`{
		"schema": "curator-run-defaults-v3",
		"defaults": {
			"kimi_code": {"permissions": "native", "model": "kimi-code/k3.1-pro"}
		}
	}`)); err != nil {
		t.Fatalf("Validate kimi_code defaults: %v", err)
	}
	reg, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := (Files{}).Complete(fragment.EnvKimiCode, Pair{}, reg)
	if err != nil {
		t.Fatalf("Complete(kimi_code): %v", err)
	}
	if resolved.Runtime != "kimi" || !resolved.Model.Present || resolved.Model.Value != "kimi-code/kimi-for-coding" {
		t.Fatalf("unexpected resolved kimi_code defaults: %+v", resolved)
	}
}
