package launcherconfig

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
)

func fixture(t *testing.T) configfile.Resolver {
	t.Helper()
	root := t.TempDir()
	system, user := filepath.Join(root, "system"), filepath.Join(root, "user")
	for _, d := range []string{system, user} {
		if err := os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return configfile.Resolver{Locations: configfile.Locations{System: system, User: user}}
}
func put(t *testing.T, dir, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "launcher.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

// Absent member/file means auto/default; each explicit machine mode carries
// machine provenance, while a schema-only user layer has no effect.
func TestMachineModes(t *testing.T) {
	for _, mode := range []string{"absent", "member-absent", "auto", "native", "hosted"} {
		t.Run(mode, func(t *testing.T) {
			r := fixture(t)
			want := Mode{"auto", "default"}
			switch mode {
			case "absent":
			case "member-absent":
				put(t, r.Locations.System, `{"schema":"curator-run-launcher-v1"}`)
			default:
				put(t, r.Locations.System, fmt.Sprintf(`{"schema":"curator-run-launcher-v1","host_mode":%q}`, mode))
				want = Mode{mode, "machine"}
			}
			put(t, r.Locations.User, `{"schema":"curator-run-launcher-v1"}`)
			got, err := Load(r)
			if err != nil || got.HostMode != want {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
}

// Closed JSON/schema/vocabulary errors and user authority violations refuse
// with no partial mode, even when a valid machine mode would otherwise win.
func TestInvalidAndUserAuthority(t *testing.T) {
	for _, doc := range []string{`{`, `{} {}`, `null`, `[]`, `{}`, `{"schema":"wrong"}`, `{"schema":"curator-run-launcher-v1","extra":true}`, `{"schema":"curator-run-launcher-v1","host_mode":null}`, `{"schema":"curator-run-launcher-v1","host_mode":"cloud"}`, `{"schema":"curator-run-launcher-v1","host_mode":"auto","host_mode":"native"}`} {
		t.Run(doc, func(t *testing.T) {
			r := fixture(t)
			put(t, r.Locations.System, `{"schema":"curator-run-launcher-v1","host_mode":"native"}`)
			if got, err := Load(r); err != nil || got.HostMode != (Mode{"native", "machine"}) {
				t.Fatal(got, err)
			}
			put(t, r.Locations.User, doc)
			got, err := Load(r)
			if err == nil || got != (Config{}) || !strings.Contains(err.Error(), CodeInvalid) {
				t.Fatal(got, err)
			}
		})
	}
	for _, mode := range []string{"native", "hosted", "auto"} {
		r := fixture(t)
		put(t, r.Locations.User, fmt.Sprintf(`{"schema":"curator-run-launcher-v1","host_mode":%q}`, mode))
		_, err := Load(r)
		if err == nil || !strings.Contains(err.Error(), filepath.Join(r.Locations.System, "launcher.json")) {
			t.Fatal(err)
		}
	}
}

// Legacy launcher policy uses the shared fallback and emits one deprecation
// line, while an invalid current file refuses without opening the old file.
func TestLauncherFallbackAndInvalidCurrent(t *testing.T) {
	r := fixture(t)
	r.Locations.LegacySystem = filepath.Join(t.TempDir(), "old")
	os.Mkdir(r.Locations.LegacySystem, 0700)
	put(t, r.Locations.LegacySystem, `{"schema":"curator-run-launcher-v1","host_mode":"hosted"}`)
	var out bytes.Buffer
	r.Stderr = &out
	got, err := Load(r)
	if err != nil || got.HostMode != (Mode{"hosted", "machine"}) || strings.Count(out.String(), "deprecated config path") != 1 {
		t.Fatal(got, err, &out)
	}
	out.Reset()
	put(t, r.Locations.System, `{"schema":"wrong"}`)
	_, err = Load(r)
	if err == nil || out.Len() != 0 {
		t.Fatal(err, &out)
	}
}

// Delegated trial is an optional system boolean exposed alongside mode. It
// never accepts coercions or a user-layer value, including explicit false.
func TestDelegatedTrial(t *testing.T) {
	for _, value := range []bool{false, true} {
		r := fixture(t)
		put(t, r.Locations.System, fmt.Sprintf(`{"schema":"curator-run-launcher-v1","host_mode":"native","delegated_trial":%t}`, value))
		got, err := Load(r)
		if err != nil || got.HostMode != (Mode{"native", "machine"}) || got.DelegatedTrial != value || !got.DelegatedTrialSet {
			t.Fatal(got, err)
		}
	}
	r := fixture(t)
	got, err := Load(r)
	if err != nil || got.DelegatedTrial || got.DelegatedTrialSet {
		t.Fatal("absent", got, err)
	}
	for _, value := range []string{`null`, `1`, `"true"`, `[]`, `{}`} {
		r := fixture(t)
		put(t, r.Locations.System, `{"schema":"curator-run-launcher-v1","delegated_trial":`+value+`}`)
		got, err := Load(r)
		if err == nil || got != (Config{}) || !strings.Contains(err.Error(), "delegated_trial must be a boolean") {
			t.Fatal(got, err)
		}
	}
	for _, value := range []string{`true`, `false`, `null`} {
		r := fixture(t)
		put(t, r.Locations.User, `{"schema":"curator-run-launcher-v1","delegated_trial":`+value+`}`)
		got, err := Load(r)
		if err == nil || got != (Config{}) || !strings.Contains(err.Error(), filepath.Join(r.Locations.System, "launcher.json")) {
			t.Fatal(got, err)
		}
	}
}
