//go:build unix

package fakemanager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
)

// FixtureRequest is a separate test adapter contract. It is never accepted on
// the plugin wire, never advertised as a capability, and carries no secrets.
type FixtureRequest struct {
	Operation string   `json:"operation"`
	Session   string   `json:"session,omitempty"`
	Activity  string   `json:"activity,omitempty"`
	Enabled   bool     `json:"enabled,omitempty"`
	Name      string   `json:"name,omitempty"`
	Now       string   `json:"now,omitempty"`
	Kind      string   `json:"kind,omitempty"`
	Bad       []string `json:"bad,omitempty"`
	Config    *Config  `json:"config,omitempty"`
}
type Observation struct {
	PolicyDigests map[string]string `json:"policy_digests"`
	Starts        int               `json:"starts"`
	Alive         map[string]bool   `json:"alive"`
	Gates         map[string]bool   `json:"gates"`
	Retirement    map[string]int    `json:"retirement"`
	// Records intentionally contain only validated admitted metadata, never input
	// from a rejected request. Qualification uses the sentinel scan as a guard.
	Records json.RawMessage `json:"records"`
}
type FixtureResponse struct {
	OK          bool            `json:"ok"`
	Code        string          `json:"code,omitempty"`
	Launch      json.RawMessage `json:"launch,omitempty"`
	Observation *Observation    `json:"observation,omitempty"`
}

// LaunchTemplate supplies a harmless, fully specified positive control. The
// adapter's reset uses the fixture workspace, rather than any real project.
func LaunchTemplate(cwd string) map[string]any {
	return map[string]any{"operation_id": uuid(), "submitted_at": time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), "admission_key": nil, "runtime": "codex_cli", "profile": map[string]string{"name": "sample", "lock_sha256": DefaultConfig().Profiles["sample"].LockSHA256, "fragment_digest": DefaultConfig().Profiles["sample"].FragmentDigest}, "cwd": cwd, "model": nil, "effort": nil, "service_tier": nil, "weights_id": nil, "permissions": map[string]string{"mode": "native", "source": "flag"}, "system_prompt": nil, "sandbox": map[string]any{"mode": "none", "writable_roots": []string{}}, "harness_memory": false, "session_name": "conformance", "labels": map[string]string{}, "child_env": map[string]string{}, "board_goal": "none", "doorbell": nil, "credential_source": nil, "native_tail": []string{}, "detach": true}
}

// Fixture runs one adapter invocation under the very same state lock as RPCs.
// reset is intentionally destructive only within this explicit fake state dir.
func Fixture(dir string, input io.Reader, output io.Writer) int {
	result := FixtureResponse{Code: "fixture_invalid"}
	defer func() { _, _ = output.Write(encode(result)) }()
	b, err := io.ReadAll(io.LimitReader(input, client.Limit+1))
	if err != nil || len(b) > client.Limit {
		return 2
	}
	var r FixtureRequest
	if strict(b, &r) != nil {
		return 2
	}
	m, lock, err := open(dir)
	if err != nil {
		return 2
	}
	defer lock.Close()
	saveControl := false
	switch r.Operation {
	case "reset":
		c := DefaultConfig()
		if r.Config != nil {
			c = *r.Config
		}
		// A reset creates a fresh fixture, including the simulated profile binding.
		m.state = State{Root: m.dir, Sessions: map[string]*Stored{}, Operations: map[string]*Ledger{}, Admissions: map[string]string{}, Generations: map[string]int64{}, Snapshots: map[string]Snapshot{}}
		m.control = Control{}
		m.config = c
		m.dirty = true
		saveControl = true
		if sessionmanager.WriteAtomic(filepath.Join(m.dir, "config.json"), encode(c)) != nil {
			return 2
		}
		result.Launch = encode(LaunchTemplate(m.dir))
	case "set_activity":
		if m.state.Sessions[r.Session] == nil {
			return 2
		}
		switch r.Activity {
		case "idle", "busy", "unknown", "draining":
			if m.control.Activity == nil {
				m.control.Activity = map[string]string{}
			}
			m.control.Activity[r.Session] = r.Activity
		case "uncertain":
			s := m.state.Sessions[r.Session]
			var row map[string]any
			_ = json.Unmarshal(s.Record, &row)
			d := row["doorbell"].(map[string]any)
			d["state"], d["uncertain"], d["reason"] = "off", true, "previous_generation_uncertain"
			s.Record = encode(row)
			m.dirty = true
		case "graceful-hang":
			m.control.GracefulHang = r.Enabled
		case "provider-gone":
			m.control.ProviderGone = append(m.control.ProviderGone, r.Session)
		default:
			return 2
		}
		saveControl = true
	case "set_identity_drift":
		p := m.config.Profiles["sample"]
		if r.Enabled {
			p.LockSHA256 = strings.Repeat("3", 64)
		} else {
			p = DefaultConfig().Profiles["sample"]
		}
		m.config.Profiles["sample"] = p
		if sessionmanager.WriteAtomic(filepath.Join(m.dir, "config.json"), encode(m.config)) != nil {
			return 2
		}
	case "inject_fault":
		m.control.Fault = r.Name
		if r.Bad != nil {
			m.control.Bad = r.Bad
		}
		saveControl = true
	case "set_clock":
		now, e := time.Parse(time.RFC3339, r.Now)
		if e != nil || len(r.Now) != 20 || !strings.HasSuffix(r.Now, "Z") || now.Format(time.RFC3339) != r.Now {
			return 2
		}
		m.control.Now = r.Now
		saveControl = true
	case "corrupt_index":
		switch r.Kind {
		case "unreadable":
			m.control.Unreadable = append(m.control.Unreadable, r.Session)
		case "missing-labels":
			m.control.MissingIndexLabels = append(m.control.MissingIndexLabels, r.Session)
		case "missing-handle":
			m.control.IndexUnreadable = r.Enabled
		case "repair":
			m.control.Unreadable = nil
			m.control.MissingIndexLabels = nil
			m.control.IndexUnreadable = false
		default:
			return 2
		}
		saveControl = true
	case "observe": // Read-only evidence, outside the public plugin contract.
	default:
		return 2
	}
	if saveControl && sessionmanager.WriteAtomic(filepath.Join(m.dir, "control.json"), encode(m.control)) != nil {
		return 2
	}
	if m.save() != nil {
		return 2
	}
	records, err := read(filepath.Join(m.dir, "state.json"), 8<<20)
	if errors.Is(err, os.ErrNotExist) {
		records = []byte("null")
	} else if err != nil {
		return 2
	}
	o := &Observation{PolicyDigests: map[string]string{}, Starts: m.state.Starts, Alive: map[string]bool{}, Gates: map[string]bool{}, Retirement: map[string]int{}, Records: records}
	for h, s := range m.state.Sessions {
		sum := sha256.Sum256(s.Plan)
		o.PolicyDigests[h] = hex.EncodeToString(sum[:])
		o.Alive[h], o.Gates[h], o.Retirement[h] = s.Alive, s.Gate, s.Retirement
	}
	result.OK, result.Code, result.Observation = true, "", o
	return 0
}

// StateArguments decodes the registered CLI prefix shared by the plugin and
// its separate adapter. Fixture operations are never parsed by the plugin CLI.
func StateArguments(args []string) (string, []string) {
	if len(args) >= 2 && args[0] == "--state" {
		return args[1], args[2:]
	}
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(root, "curator-run", "fake-manager"), args
}
