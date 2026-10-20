//go:build unix

// Package fakemanager is a deterministic, disk-backed protocol fixture. Its
// provider is a no-op lifecycle, never a harness, daemon or credential client.
package fakemanager

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
	"golang.org/x/sys/unix"
)

const Retention = 7 * 24 * time.Hour

var errIndexUnreadable = errors.New("invalid fixture index")

type Profile struct {
	LockSHA256     string `json:"lock_sha256"`
	FragmentDigest string `json:"fragment_digest"`
}
type Config struct {
	// InitialState controls only the fixture no-op provider. The default stays
	// unknown; this option never claims real provider activity.
	InitialState string `json:"initial_state,omitempty"`
	// Models restricts this fixture to declared, nonsecret model values when set.
	Models           []string            `json:"models,omitempty"`
	ID               string              `json:"id"`
	Capabilities     []string            `json:"capabilities"`
	Runtimes         map[string][]string `json:"runtimes"`
	Profiles         map[string]Profile  `json:"profiles"`
	CredentialSource json.RawMessage     `json:"credential_source"`
}

// Control is an out-of-band fixture adapter, never a plugin RPC. It carries
// no credentials. Bad selects deliberately nonconforming qualification plants.
type Control struct {
	DoorbellReason     string            `json:"doorbell_reason,omitempty"`
	Now                string            `json:"now"`
	Fault              string            `json:"fault"`
	Bad                []string          `json:"bad"`
	Activity           map[string]string `json:"activity"`
	Unreadable         []string          `json:"unreadable"`
	IndexUnreadable    bool              `json:"index_unreadable"`
	MissingIndexLabels []string          `json:"missing_index_labels"`
	Uncertain          []string          `json:"uncertain"`
	GracefulHang       bool              `json:"graceful_hang"`
	ProviderGone       []string          `json:"provider_gone"`
	AttachExit         int               `json:"attach_exit"`
}

func DefaultConfig() Config {
	ops := []string{"launch", "resume", "attach", "status", "cancel", "list", "stop"}
	return Config{ID: "fake", Capabilities: []string{"launch-delegated/1", "resume/1", "attach/1", "status/1", "list/1", "stop/1", "admission/1", "labels/1", "doorbell-codex/1", "doorbell-claude/1"}, Runtimes: map[string][]string{"codex_cli": ops, "claude_code": ops}, Profiles: map[string]Profile{"sample": {strings.Repeat("1", 64), fragment.DigestPrefix + strings.Repeat("2", 64)}}}
}

// MuseConfig qualifies the fixture's no-op Muse launch/attach lifecycle only.
// DefaultConfig deliberately remains without Muse to witness the closed gate.
func MuseConfig() Config {
	c := DefaultConfig()
	c.Runtimes["muse"] = []string{"launch", "attach", "status", "cancel", "list", "stop"}
	return c
}

type Stored struct {
	Handle          string            `json:"handle"`
	Manager         string            `json:"manager"`
	Runtime         string            `json:"runtime"`
	CWD             string            `json:"cwd"`
	Labels          map[string]string `json:"labels"`
	ProfileName     string            `json:"profile_name"`
	Profile         Profile           `json:"profile"`
	Credential      json.RawMessage   `json:"credential"`
	DoorbellRequest json.RawMessage   `json:"doorbell_request"`
	// Plan is admitted non-secret request metadata, retained unchanged on resume.
	Plan       json.RawMessage `json:"plan"`
	Record     json.RawMessage `json:"record"`
	Alive      bool            `json:"alive"`
	Gate       bool            `json:"gate"`
	Admission  string          `json:"admission"`
	Retirement int             `json:"retirement"`
}
type Ledger struct {
	Digest      string          `json:"digest"`
	SubmittedAt string          `json:"submitted_at"`
	Outcome     string          `json:"outcome"`
	Handle      string          `json:"handle"`
	Code        string          `json:"code"`
	Record      json.RawMessage `json:"record"`
}
type Snapshot struct {
	Rows   []json.RawMessage `json:"rows"`
	Offset int               `json:"offset"`
	Labels map[string]string `json:"labels"`
}

// Process records are bounded fixture evidence, not provider liveness evidence.
type CallProcess struct{ PID, Group int }
type CallHistory struct {
	Calls         []string      `json:"calls"`
	CallProcesses []CallProcess `json:"call_processes"`
}

type State struct {
	Root        string              `json:"root"`
	Sessions    map[string]*Stored  `json:"sessions"`
	Operations  map[string]*Ledger  `json:"operations"`
	Admissions  map[string]string   `json:"admissions"`
	Generations map[string]int64    `json:"generations"`
	Snapshots   map[string]Snapshot `json:"snapshots"`
	Starts      int                 `json:"starts"`
	Rejected    json.RawMessage     `json:"rejected,omitempty"`
}
type request struct {
	OperationID  string  `json:"operation_id"`
	SubmittedAt  string  `json:"submitted_at"`
	AdmissionKey *string `json:"admission_key"`
	Runtime      string  `json:"runtime"`
	Profile      struct {
		Name           string `json:"name"`
		LockSHA256     string `json:"lock_sha256"`
		FragmentDigest string `json:"fragment_digest"`
	} `json:"profile"`
	CWD        string            `json:"cwd"`
	Labels     map[string]string `json:"labels"`
	Credential json.RawMessage   `json:"credential_source"`
	Doorbell   json.RawMessage   `json:"doorbell"`
	Selector   struct {
		Kind   string `json:"kind"`
		Handle string `json:"handle"`
		ID     string `json:"id"`
	} `json:"selector"`
	Scope struct {
		CWD    string            `json:"cwd"`
		Labels map[string]string `json:"labels"`
	} `json:"scope"`
	Session            string `json:"session"`
	ExpectedGeneration int64  `json:"expected_generation"`
	Mode               string `json:"mode"`
	Precondition       string `json:"precondition"`
	Cursor             string `json:"cursor"`
	Limit              int    `json:"limit"`
}
type manager struct {
	dir     string
	config  Config
	control Control
	state   State
	now     time.Time
	dirty   bool
}
type reply struct {
	body    any
	code    string
	details map[string]string
	drop    bool
}

func encode(v any) []byte { b, _ := json.Marshal(v); return b }
func envelope(v map[string]any) map[string]any {
	v["protocol"], v["ok"] = client.Protocol, true
	return v
}
func refusal(code string, details map[string]string) reply {
	if details == nil {
		details = map[string]string{}
	}
	return reply{code: code, details: details}
}
func (m *manager) bad(name string) bool   { return slices.Contains(m.control.Bad, name) }
func (m *manager) fail(code string) reply { return refusal(code, nil) }
func strict(data []byte, v any) error {
	if _, err := fragment.ParseJSON(data); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func read(path string, limit int) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "fixture state")
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		return nil, errors.New("unsafe fixture state")
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if len(b) > limit {
		return nil, errors.New("fixture state bound")
	}
	return b, err
}
func loadOptional(path string, v any) error {
	b, err := read(path, client.Limit)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return strict(b, v)
}
func open(dir string) (*manager, *os.File, error) {
	if !filepath.IsAbs(dir) {
		return nil, nil, errors.New("absolute state required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(dir)
	var directory unix.Stat_t
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 || unix.Stat(dir, &directory) != nil || directory.Uid != uint32(os.Geteuid()) {
		return nil, nil, errors.New("unsafe state directory")
	}
	fd, err := unix.Open(filepath.Join(dir, "lock"), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, nil, err
	}
	lock := os.NewFile(uintptr(fd), "fixture lock")
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		lock.Close()
		return nil, nil, errors.New("unsafe fixture lock")
	}
	until := time.Now().Add(5 * time.Second)
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if err != unix.EAGAIN && err != unix.EWOULDBLOCK || time.Now().After(until) {
			lock.Close()
			return nil, nil, errors.New("fixture lock unavailable")
		}
		time.Sleep(5 * time.Millisecond)
	}
	fresh := false
	m := &manager{dir: dir, now: time.Now().UTC().Truncate(time.Second)}
	// A persisted configuration replaces the default maps in full. Unmarshalling
	// into populated defaults would silently restore omitted runtime entries.
	configData, configErr := read(filepath.Join(dir, "config.json"), client.Limit)
	if errors.Is(configErr, os.ErrNotExist) {
		m.config = DefaultConfig()
		err = nil
	} else if configErr != nil {
		err = configErr
	} else {
		err = strict(configData, &m.config)
	}
	if err == nil {
		err = loadOptional(filepath.Join(dir, "control.json"), &m.control)
	}
	if err == nil {
		supported := []string{"launch-delegated/1", "resume/1", "attach/1", "status/1", "list/1", "stop/1", "admission/1", "labels/1", "doorbell-codex/1", "doorbell-claude/1", "credential-node/1"}
		for _, cap := range m.config.Capabilities {
			if !slices.Contains(supported, cap) {
				err = errors.New("unimplemented fake capability")
			}
		}
	}
	if err == nil {
		capability := map[string]string{"launch": "launch-delegated/1", "resume": "resume/1", "attach": "attach/1", "status": "status/1", "cancel": "admission/1", "list": "list/1", "stop": "stop/1"}
		for runtime, ops := range m.config.Runtimes {
			filtered := []string{}
			for _, op := range ops {
				if c := capability[op]; c != "" && slices.Contains(m.config.Capabilities, c) {
					filtered = append(filtered, op)
				}
			}
			m.config.Runtimes[runtime] = filtered
		}
	}
	if err == nil && m.control.Now != "" {
		m.now, err = time.Parse(time.RFC3339, m.control.Now)
		if len(m.control.Now) != 20 || !strings.HasSuffix(m.control.Now, "Z") {
			err = errors.New("invalid fixture clock")
		}
	}
	if err == nil {
		var b []byte
		b, err = read(filepath.Join(dir, "state.json"), 8<<20)
		if errors.Is(err, os.ErrNotExist) {
			fresh = true
			err = nil
		} else if err == nil {
			err = strict(b, &m.state)
		}
		if err != nil {
			err = errIndexUnreadable
		}
	}
	if err != nil {
		lock.Close()
		return nil, nil, err
	}
	if !fresh && m.state.Root != dir {
		lock.Close()
		return nil, nil, errors.New("state root changed")
	}
	if m.state.Sessions == nil {
		if !fresh {
			lock.Close()
			return nil, nil, errIndexUnreadable
		}
		m.state = State{Root: dir, Sessions: map[string]*Stored{}, Operations: map[string]*Ledger{}, Admissions: map[string]string{}, Generations: map[string]int64{}, Snapshots: map[string]Snapshot{}}
	}
	if m.state.Operations == nil || m.state.Admissions == nil || m.state.Generations == nil || m.state.Snapshots == nil {
		lock.Close()
		return nil, nil, errors.New("incomplete fixture state")
	}
	for h, s := range m.state.Sessions {
		if s == nil || h != s.Handle {
			lock.Close()
			return nil, nil, errIndexUnreadable
		}
	}
	for _, l := range m.state.Operations {
		if l == nil {
			lock.Close()
			return nil, nil, errors.New("invalid fixture ledger")
		}
	}
	for _, snap := range m.state.Snapshots {
		if snap.Offset < 0 || snap.Offset > len(snap.Rows) {
			lock.Close()
			return nil, nil, errors.New("invalid fixture cursor")
		}
	}
	return m, lock, nil
}
func (m *manager) save() error {
	if !m.dirty {
		return nil
	}
	data := encode(m.state)
	if len(data) > 8<<20 {
		return errors.New("fixture state bound")
	}
	return sessionmanager.WriteAtomic(filepath.Join(m.dir, "state.json"), data)
}
func (m *manager) drift(s *Stored) bool {
	if m.bad("identity-drift") {
		return false
	}
	p, ok := m.config.Profiles[s.ProfileName]
	return !ok || p != s.Profile
}
func (m *manager) unreadable(s *Stored) bool {
	if m.bad("unreadable") {
		return false
	}
	if slices.Contains(m.control.Unreadable, s.Handle) || schema.Validate("session-record.schema.json", s.Record) != nil {
		return true
	}
	var record client.Session
	if json.Unmarshal(s.Record, &record) != nil {
		return true
	}
	return !record.Readable || record.Session != s.Handle || record.Manager != s.Manager || record.Runtime != s.Runtime || !reflect.DeepEqual(record.Labels, s.Labels)
}
func projection(s *Stored) map[string]any {
	return map[string]any{"session": s.Handle, "manager": s.Manager, "labels": s.Labels, "readable": false, "state": "unknown", "operation_id": nil, "runtime": nil, "native_id": nil, "generation": nil, "started_at": nil, "last_activity_at": nil, "doorbell": client.Doorbell{State: "off"}}
}

func (m *manager) row(s *Stored) map[string]any {
	if m.unreadable(s) {
		return projection(s)
	}
	var r map[string]any
	_ = json.Unmarshal(s.Record, &r)
	if state, ok := m.control.Activity[s.Handle]; ok && s.Alive && r["state"] != "draining" {
		r["state"] = state
		if state == "unknown" && m.bad("unknown-liveness") {
			r["state"] = "idle"
		}
		if (state == "idle" || state == "busy") && m.bad("activity-on-read") {
			r["last_activity_at"] = m.now.Format(time.RFC3339)
		}
	}
	if slices.Contains(m.control.Uncertain, s.Handle) {
		d := r["doorbell"].(map[string]any)
		d["state"], d["uncertain"], d["reason"] = "off", true, "previous_generation_uncertain"
	}
	return r
}
func (m *manager) outcome(l *Ledger) reply {
	if l == nil {
		return reply{body: envelope(map[string]any{"outcome": "not_found"})}
	}
	state := l.Outcome
	result := envelope(map[string]any{"outcome": state})
	if state == "started" {
		s := m.state.Sessions[l.Handle]
		if s == nil || m.unreadable(s) {
			if s == nil || !m.bad("started-unreadable") {
				return reply{body: envelope(map[string]any{"outcome": "unknown"})}
			}
			result["session"] = m.row(s)
		} else if m.drift(s) {
			return reply{body: envelope(map[string]any{"outcome": "failed", "code": "session_curator_identity_stale"})}
		} else {
			result["session"] = json.RawMessage(l.Record)
		}
	}
	if state == "failed" {
		result["code"] = l.Code
	}
	return reply{body: result}
}
func uuid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func token() string { return strings.ReplaceAll(uuid(), "-", "") }
func subset(have, want map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}
func sameJSON(a, b []byte) bool {
	var x, y any
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	return reflect.DeepEqual(x, y)
}
func canonicalCWD(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func (m *manager) selectResume(r request) (*Stored, string) {
	candidates := []*Stored{}
	for _, s := range m.state.Sessions {
		if r.Selector.Kind == "handle" && s.Handle != r.Selector.Handle && !m.bad("resume-missing") {
			continue
		}
		if r.Selector.Kind == "provider_id" {
			row := m.row(s)
			if row["native_id"] != r.Selector.ID {
				continue
			}
		}
		if (s.CWD != canonicalCWD(r.Scope.CWD) || s.Runtime != r.Runtime || !subset(s.Labels, r.Scope.Labels)) && !m.bad("resume-location") {
			if r.Selector.Kind == "handle" {
				return nil, "session_resume_location_mismatch"
			}
			continue
		}
		candidates = append(candidates, s)
	}
	if len(candidates) == 0 {
		return nil, "session_resume_not_found"
	}
	if r.Selector.Kind == "latest" {
		for _, s := range candidates {
			if m.unreadable(s) {
				return nil, "session_resume_ambiguous"
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			return m.row(candidates[i])["started_at"].(string) > m.row(candidates[j])["started_at"].(string)
		})
		if len(candidates) > 1 && m.row(candidates[0])["started_at"] == m.row(candidates[1])["started_at"] && !m.bad("resume-ambiguous") {
			return nil, "session_resume_ambiguous"
		}
	} else if len(candidates) > 1 {
		return nil, "session_resume_ambiguous"
	}
	s := candidates[0]
	if m.unreadable(s) {
		return nil, "session_resume_unreadable"
	}
	return s, ""
}
func (m *manager) start(op string, r request, data []byte) reply {
	v, _ := fragment.ParseJSON(data)
	sum := sha256.Sum256(fragment.Canonical(v))
	digest := hex.EncodeToString(sum[:])
	if l := m.state.Operations[r.OperationID]; l != nil {
		if s := m.state.Sessions[l.Handle]; s != nil && m.drift(s) {
			return m.fail("session_curator_identity_stale")
		}
		if l.Digest != "" && l.Digest != digest && !m.bad("operation-conflict") {
			return m.fail("session_operation_conflict")
		}
		if l.Outcome == "cancelled" && !m.bad("cancel-fence") {
			return refusal("session_host_unavailable", map[string]string{"reason": "cancelled"})
		}
		if !m.bad("idempotency") && !(l.Outcome == "cancelled" && m.bad("cancel-fence")) && l.Outcome != "prepared" {
			o := m.outcome(l)
			if l.Outcome == "started" {
				if o.body.(map[string]any)["outcome"] == "started" {
					var row map[string]any
					_ = json.Unmarshal(l.Record, &row)
					return reply{body: envelope(row)}
				}
				return m.fail("session_liveness_unknown")
			}
			if l.Outcome == "failed" {
				return m.fail(l.Code)
			}
			return m.fail("session_liveness_unknown")
		}
	}
	submitted, _ := time.Parse(time.RFC3339, r.SubmittedAt)
	if m.now.Sub(submitted) > Retention && !m.bad("operation-expiry") {
		return m.fail("session_operation_expired")
	}
	if m.control.Fault == "before-admission" && !m.bad("lost-before-admission") {
		return reply{drop: true}
	}
	if m.control.Fault == "prepared" {
		m.state.Operations[r.OperationID] = &Ledger{Digest: digest, SubmittedAt: r.SubmittedAt, Outcome: "prepared"}
		m.dirty = true
		return reply{drop: true}
	}
	fail := func(code string) reply {
		m.state.Operations[r.OperationID] = &Ledger{Digest: digest, SubmittedAt: r.SubmittedAt, Outcome: "failed", Code: code}
		m.dirty = true
		return m.fail(code)
	}
	var s *Stored
	if op == "resume" {
		var code string
		s, code = m.selectResume(r)
		if code != "" {
			return fail(code)
		}
		if m.drift(s) {
			return fail("session_curator_identity_stale")
		}
		if s.ProfileName != r.Profile.Name && !m.bad("resume-identity") {
			return fail("session_resume_identity_mismatch")
		}
		if !sameJSON(s.Credential, r.Credential) {
			return fail("credential_source_stale")
		}
		if !sameJSON(s.DoorbellRequest, r.Doorbell) {
			return fail("policy_refused")
		}
	}
	p, ok := m.config.Profiles[r.Profile.Name]
	if !ok || p != (Profile{r.Profile.LockSHA256, r.Profile.FragmentDigest}) {
		if !m.bad("profile-digest") {
			return fail("policy_refused")
		}
	}
	if !sameJSON(r.Credential, m.config.CredentialSource) {
		return fail("credential_source_stale")
	}
	if op == "launch" && len(m.config.Models) > 0 {
		var plan struct {
			Model *string `json:"model"`
		}
		_ = json.Unmarshal(data, &plan)
		if plan.Model == nil || !slices.Contains(m.config.Models, *plan.Model) {
			return fail("policy_refused")
		}
	}
	key := ""
	if r.AdmissionKey != nil {
		key = *r.AdmissionKey
	}
	if owner := m.state.Admissions[key]; key != "" && owner != "" && !m.bad("admission") && !(owner == r.OperationID && m.bad("idempotency")) {
		old := m.state.Operations[owner]
		details := map[string]string{}
		if old != nil && old.Handle != "" {
			details["session"] = old.Handle
		}
		return refusal("werkstatt_already_up", details)
	}
	if s != nil && s.Alive && !m.bad("admission") {
		return refusal("werkstatt_already_up", map[string]string{"session": s.Handle})
	}
	l := &Ledger{Digest: digest, SubmittedAt: r.SubmittedAt, Outcome: "admitted"}
	m.state.Operations[r.OperationID] = l
	m.dirty = true
	if key != "" {
		m.state.Admissions[key] = r.OperationID
	}
	// Reserve durably before even the no-op provider can start. The drop
	// plant completes admission but loses its reply; a separate crash plant
	// deliberately leaves uncertain start evidence and holds the key.
	if err := m.save(); err != nil {
		return refusal("session_host_unavailable", map[string]string{"reason": "transport"})
	}
	if m.control.Fault == "crash-after-reservation" {
		if m.bad("reservation") {
			_ = os.Remove(filepath.Join(m.dir, "state.json"))
		}
		os.Exit(1) // Explicit fixture crash: no deferred/final persistence.
	}
	if m.control.Fault == "failed-start" {
		l.Outcome, l.Code = "failed", "policy_refused"
		if m.bad("lost-failed-start") {
			l.Outcome = "admitted"
		}
		delete(m.state.Admissions, key)
		return reply{drop: true}
	}
	generation := int64(1)
	if s == nil {
		h := "SES-" + token()
		s = &Stored{Handle: h, Manager: m.config.ID, Runtime: r.Runtime, CWD: canonicalCWD(r.CWD), Labels: r.Labels, ProfileName: r.Profile.Name, Profile: p, Credential: r.Credential, DoorbellRequest: r.Doorbell, Plan: append(json.RawMessage{}, data...)}
		m.state.Sessions[h] = s
	} else {
		generation = int64(m.row(s)["generation"].(float64)) + 1
		if m.bad("resume-policy") {
			s.Plan = append(json.RawMessage{}, data...)
		}
	}
	if key != "" {
		generation = max(generation, m.state.Generations[key]+1)
		m.state.Generations[key] = generation
	}
	doorbell := client.Doorbell{State: "off"}
	if string(r.Doorbell) != "null" && len(r.Doorbell) > 0 {
		g := token()
		doorbell.State = "armed"
		doorbell.Generation = &g
		switch m.control.DoorbellReason {
		case "unsupported_version":
			reason := m.control.DoorbellReason
			doorbell = client.Doorbell{State: "unavailable", Reason: &reason}
		case "previous_generation_uncertain":
			reason := m.control.DoorbellReason
			doorbell = client.Doorbell{State: "off", Uncertain: true, Generation: &g, Reason: &reason}
		}
	}
	if len(s.Record) > 0 {
		var prev client.Session
		_ = json.Unmarshal(s.Record, &prev)
		if prev.Doorbell.Uncertain || slices.Contains(m.control.Uncertain, s.Handle) {
			reason := "previous_generation_uncertain"
			doorbell = client.Doorbell{State: "off", Uncertain: true, Generation: prev.Doorbell.Generation, Reason: &reason}
			if m.bad("doorbell-generation") {
				doorbell.Generation = nil
			}
		}
	}
	state := "unknown"
	if m.config.InitialState == "idle" || m.config.InitialState == "busy" {
		state = m.config.InitialState
	}
	row := map[string]any{"session": s.Handle, "operation_id": r.OperationID, "manager": m.config.ID, "runtime": r.Runtime, "native_id": s.Handle[4:], "readable": true, "state": state, "labels": s.Labels, "generation": generation, "started_at": m.now.Format(time.RFC3339), "last_activity_at": nil, "doorbell": doorbell}
	s.Alive, s.Gate, s.Admission, s.Retirement = true, true, key, 0
	s.Record = encode(row)
	l.Handle, l.Record = s.Handle, append(json.RawMessage{}, s.Record...)
	m.state.Starts++
	if m.control.Fault == "crash-after-start" {
		l.Outcome = "unknown"
		if m.bad("unknown-start") {
			l.Outcome = "started"
		}
		return reply{drop: true}
	}
	l.Outcome = "started"
	if m.control.Fault == "after-admission" && m.bad("lost-after-admission") {
		l.Outcome = "unknown"
	}
	if m.control.Fault == "after-admission" {
		return reply{drop: true}
	}
	return reply{body: envelope(row)}
}
func (m *manager) list(r request) reply {
	limit := r.Limit
	if limit == 0 {
		limit = 50
	}
	var snap Snapshot
	if r.Cursor != "" {
		var ok bool
		snap, ok = m.state.Snapshots[r.Cursor]
		if ok && m.bad("snapshot") {
			for i, row := range snap.Rows {
				var record client.Session
				_ = json.Unmarshal(row, &record)
				if s := m.state.Sessions[record.Session]; s != nil {
					snap.Rows[i] = encode(m.row(s))
				}
			}
		}
		if !ok || !reflect.DeepEqual(snap.Labels, r.Labels) {
			return m.fail("launch_plan_invalid")
		}
	} else {
		if m.control.IndexUnreadable && !m.bad("index-unreadable") {
			return refusal("session_host_unavailable", map[string]string{"reason": "index_unreadable"})
		}
		keys := []string{}
		for h := range m.state.Sessions {
			keys = append(keys, h)
		}
		sort.Strings(keys)
		snap.Labels = r.Labels
		for _, h := range keys {
			s := m.state.Sessions[h]
			if schema.Validate("session-record.schema.json", encode(projection(s))) != nil && !m.bad("index-unreadable") {
				return refusal("session_host_unavailable", map[string]string{"reason": "index_unreadable"})
			}
			if len(r.Labels) > 0 && slices.Contains(m.control.MissingIndexLabels, h) && !m.bad("index-unreadable") {
				return refusal("session_host_unavailable", map[string]string{"reason": "index_unreadable"})
			}
			if subset(s.Labels, r.Labels) || m.bad("labels") {
				snap.Rows = append(snap.Rows, encode(m.row(s)))
			}
		}
	}
	rows := []json.RawMessage{}
	end := snap.Offset
	for end < len(snap.Rows) && len(rows) < limit {
		next := append(append([]json.RawMessage{}, rows...), snap.Rows[end])
		if len(encode(next)) > client.Limit-512 {
			break
		}
		rows = next
		end++
	}
	var cursor *string
	if end < len(snap.Rows) && !m.bad("pagination") {
		c := token()
		cursor = &c
		snap.Offset = end
		m.state.Snapshots[c] = snap
		m.dirty = true
	}
	return reply{body: envelope(map[string]any{"sessions": rows, "next_cursor": cursor})}
}
func (m *manager) operate(op string, r request, data []byte) reply {
	if op == "describe" {
		if m.bad("advertised-operations") {
			runtimes := map[string][]string{}
			for k, v := range m.config.Runtimes {
				runtimes[k] = v
			}
			runtimes["muse"] = []string{"launch", "resume", "attach", "status", "list", "stop", "cancel"}
			return reply{body: envelope(map[string]any{"id": m.config.ID, "version": "1.0.0", "capabilities": m.config.Capabilities, "runtimes": runtimes})}
		}
		if m.bad("wire") {
			return reply{body: map[string]any{"protocol": "wrong/1", "ok": true}}
		}
		return reply{body: envelope(map[string]any{"id": m.config.ID, "version": "1.0.0", "capabilities": m.config.Capabilities, "runtimes": m.config.Runtimes})}
	}
	cap := map[string]string{"launch": "launch-delegated/1", "resume": "resume/1", "attach": "attach/1", "status": "status/1", "cancel": "admission/1", "list": "list/1", "stop": "stop/1"}[op]
	if cap == "" {
		return m.fail("launch_plan_invalid")
	}
	if !slices.Contains(m.config.Capabilities, cap) && !m.bad("missing-capability") {
		return refusal("session_host_capability_missing", map[string]string{"capability": cap})
	}
	if op == "resume" && m.bad("advertised-resume") {
		return m.fail("policy_refused")
	}
	if op == "launch" || op == "resume" {
		if !slices.Contains(m.config.Runtimes[r.Runtime], op) && !m.bad("unsupported-runtime") {
			return refusal("session_host_provider_unsupported", map[string]string{"environment": r.Runtime})
		}
		for c, needed := range map[string]bool{"admission/1": r.AdmissionKey != nil, "labels/1": len(r.Labels) > 0 || len(r.Scope.Labels) > 0} {
			if needed && !slices.Contains(m.config.Capabilities, c) && !m.bad("missing-capability") {
				return refusal("session_host_capability_missing", map[string]string{"capability": c})
			}
		}
		var needs map[string]any
		_ = json.Unmarshal(data, &needs)
		if needs["weights_id"] != nil {
			return refusal("session_host_capability_missing", map[string]string{"capability": "weights-pin"})
		}
		for field, c := range map[string]string{"doorbell": map[string]string{"codex_cli": "doorbell-codex/1", "claude_code": "doorbell-claude/1"}[r.Runtime], "credential_source": "credential-node/1", "board_goal": "board-goal/1"} {
			v := needs[field]
			if field == "doorbell" && v != nil && c == "" {
				return refusal("session_host_provider_unsupported", map[string]string{"environment": r.Runtime})
			}
			if v != nil && v != "none" && !slices.Contains(m.config.Capabilities, c) && !m.bad("missing-capability") {
				return refusal("session_host_capability_missing", map[string]string{"capability": c})
			}
		}
		return m.start(op, r, data)
	}
	switch op {
	case "list":
		return m.list(r)
	case "cancel":
		l := m.state.Operations[r.OperationID]
		if l == nil || l.Outcome == "prepared" {
			if l == nil {
				l = &Ledger{SubmittedAt: m.now.Format(time.RFC3339)}
				m.state.Operations[r.OperationID] = l
			}
			l.Outcome = "cancelled"
			m.dirty = true
		}
		return m.outcome(l)
	case "status":
		if r.OperationID != "" {
			if m.bad("ledger-retention") {
				if l := m.state.Operations[r.OperationID]; l != nil {
					submitted, _ := time.Parse(time.RFC3339, l.SubmittedAt)
					if m.now.Sub(submitted) > 24*time.Hour {
						return m.outcome(nil)
					}
				}
			}
			return m.outcome(m.state.Operations[r.OperationID])
		}
	}
	s := m.state.Sessions[r.Session]
	if s == nil {
		if m.bad("unknown-stop") {
			return m.fail("policy_refused")
		}
		return refusal("session_host_unavailable", map[string]string{"reason": "not_found"})
	}
	if m.drift(s) {
		return m.fail("session_curator_identity_stale")
	}
	if op == "status" {
		if s.Retirement > 0 && slices.Contains(m.control.ProviderGone, s.Handle) {
			m.retire(s)
		}
		return reply{body: envelope(m.row(s))}
	}
	if m.unreadable(s) {
		return m.fail("session_resume_unreadable")
	}
	row := m.row(s)
	if int64(row["generation"].(float64)) != r.ExpectedGeneration && !m.bad("generation") {
		if op == "attach" && m.bad("attach-framing") {
			return reply{drop: true}
		}
		return m.fail("session_generation_mismatch")
	}
	if op == "attach" {
		return reply{}
	}
	if r.Precondition == "not-busy" && (row["state"] == "busy" || row["state"] == "unknown") && !m.bad("not-busy") {
		if m.bad("busy-side-effect") {
			m.retire(s)
		}
		return refusal("werkstatt_busy", map[string]string{"state": "busy"})
	}
	if r.Mode == "graceful" && m.control.GracefulHang && s.Alive && !m.bad("graceful-retirement") {
		row["state"] = "draining"
		// The provider outlived its graceful share (§5.4 draining reason).
		row["draining"] = map[string]any{"reason": "provider_alive_after_graceful", "remediation": "wait"}
		// Closing the gate and projecting off are one durable transaction.
		if !m.bad("doorbell-gate-closure") {
			d := row["doorbell"].(map[string]any)
			d["state"], d["reason"] = "off", nil
			if d["uncertain"] == true {
				d["reason"] = "previous_generation_uncertain"
			}
		}
		s.Record = encode(row)
		s.Gate = false
		s.Retirement = 1
		m.dirty = true
	} else {
		m.retire(s)
	}
	return reply{body: envelope(m.row(s))}
}
func (m *manager) retire(s *Stored) {
	row := m.row(s)
	d := row["doorbell"].(map[string]any)
	d["state"] = "off"
	if d["uncertain"] == true {
		d["reason"] = "previous_generation_uncertain"
	} else {
		d["reason"] = nil
	}
	if m.bad("doorbell-uncertain") {
		d["uncertain"], d["reason"] = false, nil
	}
	row["state"] = "down"
	// The draining explanation is valid only while the state is draining.
	delete(row, "draining")
	s.Record = encode(row)
	s.Alive, s.Gate, s.Retirement = false, false, 4
	owner := m.state.Operations[m.state.Admissions[s.Admission]]
	if (owner != nil && owner.Handle == s.Handle) || m.bad("admission-ownership") {
		delete(m.state.Admissions, s.Admission)
	}
	m.dirty = true
}
func terminal(status io.Writer, code string) int {
	exit, message, ok := hosted.StatusRegistryRow(code)
	if !ok {
		code = "launch_plan_invalid"
		exit, message, _ = hosted.StatusRegistryRow(code)
	}
	b := encode(map[string]any{"schema": hosted.StatusSchema, "schema_version": hosted.StatusSchemaVer, "type": hosted.StatusTypeRefusal, "code": code, "message": message, "details": map[string]string{}})
	// The shared classifier validates even this emitter's fixed frame.
	if _, err := hosted.NormalizeStatusRecord(b); err != nil {
		return 6
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(b)))
	if status == nil {
		return 6
	}
	if _, err := status.Write(length[:]); err != nil {
		return 6
	}
	if _, err := status.Write(b); err != nil {
		return 6
	}
	return exit
}

// Run handles one process invocation. It never spawns a child or retains a
// service. State changes are serialized and atomically durable before replies.
func Run(dir string, args []string, input io.Reader, attachInput io.Reader, status, output io.Writer) int {
	if len(args) == 0 {
		return 2
	}
	op := args[0]
	if op == "launch-exact" {
		return terminal(status, "session_host_capability_missing")
	}
	if op == "attach" {
		input = attachInput
	}
	if input == nil {
		return 2
	}
	data, err := io.ReadAll(io.LimitReader(input, client.Limit+1))
	if err != nil {
		return 1
	}
	m, lock, err := open(dir)
	if err != nil {
		reason := "transport"
		if errors.Is(err, errIndexUnreadable) {
			reason = "index_unreadable"
		}
		return emit(refusal("session_host_unavailable", map[string]string{"reason": reason}), op, status, output)
	}
	defer lock.Close()
	// Call telemetry is separate from admitted state. Reads and rejected RPCs
	// must never rewrite the state or change qualification observations.
	if slices.Contains([]string{"describe", "launch", "resume", "attach", "status", "cancel", "list", "stop"}, op) {
		var history CallHistory
		path := filepath.Join(m.dir, "call-history.json")
		if loadOptional(path, &history) != nil {
			return 1
		}
		group, _ := unix.Getpgid(0)
		history.Calls = append(history.Calls, op)
		history.CallProcesses = append(history.CallProcesses, CallProcess{PID: os.Getpid(), Group: group})
		if len(history.Calls) > 1024 {
			history.Calls = history.Calls[len(history.Calls)-1024:]
		}
		if len(history.CallProcesses) > 1024 {
			history.CallProcesses = history.CallProcesses[len(history.CallProcesses)-1024:]
		}
		if sessionmanager.WriteAtomic(path, encode(history)) != nil {
			return 1
		}
	}
	if op == "attach" && m.control.Fault == "plain-attach-exit" {
		if m.bad("attach-plain-exit") {
			return terminal(status, "policy_refused")
		}
		return 7
	}
	known := slices.Contains([]string{"describe", "launch", "resume", "attach", "status", "cancel", "list", "stop"}, op)
	if !known {
		if m.bad("unknown-operation") {
			return emit(m.fail("policy_refused"), op, status, output)
		}
		return emit(m.fail("launch_plan_invalid"), op, status, output)
	}
	validation := client.ValidateRequest(op, data)
	if validation != nil && !m.bad("request-validation") {
		var e *client.Error
		_ = errors.As(validation, &e)
		code := e.Code
		if code == "credential_reserved_name" {
			code = "secret_policy_violation"
		}
		if m.bad("credential-leak") {
			m.state.Rejected = append(json.RawMessage{}, data...)
			m.dirty = true
			if m.save() != nil {
				return 1
			}
		}
		return emit(m.fail(code), op, status, output)
	}
	if validation != nil {
		// An explicit known-bad control admits otherwise forbidden child_env.
		// Used only to establish that qualification observes the boundary.
		var bad request
		if json.Unmarshal(data, &bad) != nil {
			return emit(m.fail("policy_refused"), op, status, output)
		}
		result := m.start(op, bad, data)
		_ = m.save()
		return emit(result, op, status, output)
	}
	var r request
	_ = json.Unmarshal(data, &r)
	result := m.operate(op, r, data)
	if err := m.save(); err != nil {
		return emit(refusal("session_host_unavailable", map[string]string{"reason": "transport"}), op, status, output)
	}
	if result.drop {
		return 1
	}
	fault := m.control.Fault
	if op == "status" && r.OperationID != "" {
		for _, name := range []string{"malformed", "over-bound", "protocol", "exit-ok", "detail-text", "detail-control", "detail-secret", "started-unreadable"} {
			if m.bad("poison-" + name) {
				fault = name
				break
			}
		}
	}
	switch fault {
	case "malformed":
		_, _ = output.Write([]byte("{"))
		return 0
	case "over-bound":
		_, _ = output.Write(bytes.Repeat([]byte(" "), client.Limit+1))
		return 0
	case "protocol":
		_, _ = output.Write(encode(map[string]any{"protocol": "other/1", "ok": true}))
		return 0
	case "exit-ok":
		_, _ = output.Write(encode(envelope(map[string]any{"outcome": "unknown"})))
		return 1
	case "detail-text", "detail-control", "detail-secret":
		value := "arbitrary text"
		if fault == "detail-control" {
			value = "\n"
		}
		if fault == "detail-secret" {
			value = "sk-conformance-sentinel"
		}
		_, _ = output.Write(encode(map[string]any{"protocol": client.Protocol, "ok": false, "code": "session_host_unavailable", "details": map[string]string{"reason": value}}))
		return 1
	}
	if fault == "started-unreadable" && op == "status" && r.OperationID != "" {
		l := m.state.Operations[r.OperationID]
		if l != nil && m.state.Sessions[l.Handle] != nil {
			_, _ = output.Write(encode(envelope(map[string]any{"outcome": "started", "session": m.row(m.state.Sessions[l.Handle])})))
			return 0
		}
	}
	if op == "attach" && result.code == "" {
		return m.control.AttachExit
	}
	if m.bad("started-unreadable") && op == "status" && r.OperationID != "" && result.body != nil {
		_, _ = output.Write(encode(result.body))
		return 0
	}
	return emit(result, op, status, output)
}
func emit(r reply, op string, status, output io.Writer) int {
	if r.code != "" {
		if op == "attach" {
			code := r.code
			if code == "session_generation_mismatch" {
				code = "session_generation_stale"
			}
			return terminal(status, code)
		}
		body := map[string]any{"protocol": client.Protocol, "ok": false, "code": r.code, "details": r.details}
		b := encode(body)
		if schema.Validate("refusal.schema.json", b) != nil {
			return 6
		}
		_, _ = output.Write(b)
		return (&client.Error{Code: r.code}).ExitCode()
	}
	if op == "attach" {
		return 0
	}
	b := encode(r.body)
	schemaName := map[string]string{"describe": "describe", "launch": "session", "resume": "session", "stop": "session", "list": "list-page", "cancel": "outcome"}[op]
	if op == "status" {
		var v map[string]any
		_ = json.Unmarshal(b, &v)
		if _, ok := v["outcome"]; ok {
			schemaName = "outcome"
		} else {
			schemaName = "session"
		}
	}
	if schema.Validate(schemaName+".schema.json", b) != nil {
		return 6
	}
	_, _ = output.Write(b)
	return 0
}
