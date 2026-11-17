package network

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Claim: the production exactLegacyListed reports listed only for the exact base name in a real directory listing — a case-variant entry does not count (on a case-sensitive host it is simply a different entry, and a host whose listing folds case must still show the exact spelling it was created with) — with the exact-name control listed, paging past one read batch, and a spent step propagated as an error.
func TestExactLegacyListedReal(t *testing.T) {
	step := func() error { return nil }
	write := func(t *testing.T, dir, name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("control: exact name is listed", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, ".config.json")
		write(t, dir, ".claude.json")
		listed, err := exactLegacyListed(dir, ".config.json", step)
		if err != nil || !listed {
			t.Fatalf("listed = %v, err = %v", listed, err)
		}
	})
	t.Run("not listed under exact name", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, ".CONFIG.JSON")
		write(t, dir, ".claude.json")
		listed, err := exactLegacyListed(dir, ".config.json", step)
		if err != nil || listed {
			t.Fatalf("listed = %v, err = %v", listed, err)
		}
	})
	t.Run("control: exact name beyond the first read batch", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < 300; i++ {
			write(t, dir, fmt.Sprintf("filler-%03d", i))
		}
		write(t, dir, ".config.json")
		listed, err := exactLegacyListed(dir, ".config.json", step)
		if err != nil || !listed {
			t.Fatalf("listed = %v, err = %v", listed, err)
		}
	})
	t.Run("a spent step refuses the listing", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, ".config.json")
		spent := errors.New("spent")
		listed, err := exactLegacyListed(dir, ".config.json", func() error { return spent })
		if !errors.Is(err, spent) || listed {
			t.Fatalf("listed = %v, err = %v", listed, err)
		}
	})
}
