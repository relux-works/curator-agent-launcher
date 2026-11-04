//go:build !darwin && !linux

package recoveryjournal

import (
	"context"
	"errors"
	"testing"
)

// Platforms without no-follow directory-descriptor support refuse rather than
// substitute path-based journal access. This does not qualify a recovery route.
func TestUnsupportedLayout(t *testing.T) {
	n, err := OpenExisting(Binding{})
	if n != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatal("unsupported layout did not refuse")
	}
	if p, err := new(Namespace).Prepare(context.Background(), fixtureEnvelope()); p != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatal("unsupported preparation did not refuse")
	}
	if entry, err := new(Namespace).ReadExact(context.Background(), fixtureEnvelope().AttemptID, Binding{}); entry != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatal("unsupported exact-entry reading did not refuse")
	}
}
