//go:build unix

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"time"

	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

// PreparedHolder deliberately has no source of process-incarnation authority.
// Its null session/generation/process members cannot be upgraded from v1 status.
type PreparedHolder struct {
	Kind                string  `json:"kind"`
	Launcher            string  `json:"launcher"`
	ManagerID           string  `json:"manager_id"`
	ManagerRegisteredAt string  `json:"manager_registered_at"`
	RequestID           string  `json:"request_id"`
	Werkstatt           *string `json:"werkstatt"`
	Session             *string `json:"session"`
	Generation          *int64  `json:"generation"`
	Process             any     `json:"process"`
}
type LeaseRecord struct {
	Kind          string         `json:"kind"`
	LeaseID       string         `json:"lease_id"`
	ManagerRootID string         `json:"manager_root_id"`
	HomeID        string         `json:"home_id"`
	Environment   string         `json:"environment"`
	Profile       string         `json:"profile"`
	LockSHA256    string         `json:"lock_sha256"`
	Revision      int64          `json:"revision"`
	Phase         string         `json:"phase"`
	AcquiredAt    string         `json:"acquired_at"`
	Holder        PreparedHolder `json:"holder"`
	Observation   struct {
		State      string  `json:"state"`
		ObservedAt *string `json:"observed_at"`
	} `json:"observation"`
}
type LeaseAdmission struct {
	RequestID    string         `json:"request_id"`
	Route        string         `json:"route"`
	Environment  string         `json:"environment"`
	LockSHA256   string         `json:"lock_sha256"`
	Holder       PreparedHolder `json:"holder"`
	Capabilities []string       `json:"capabilities"`
	Operations   []string       `json:"operations"`
	Record       *LeaseRecord   `json:"record"`
}
type leaseEnvelope struct {
	Operation string  `json:"operation"`
	Exit      int     `json:"exit"`
	OK        bool    `json:"ok"`
	Code      *string `json:"code"`
	Reason    string  `json:"reason"`
	Result    struct {
		Changed bool          `json:"changed"`
		Lease   *LeaseRecord  `json:"lease"`
		Leases  []LeaseRecord `json:"leases"`
	} `json:"result"`
}

func leaseFailure(reason string) *Error {
	return refusal("session_home_lease_unavailable", map[string]string{"reason": reason})
}

// LeaseCLI speaks only the published read/acquire API. No release or break
// method exists: neither termination nor settlement grants that authority.
type LeaseCLI struct{ Environment []string }
type leaseOutput struct {
	data     []byte
	overflow bool
}

func (w *leaseOutput) Write(p []byte) (int, error) {
	n := len(p)
	available := Limit - len(w.data)
	if len(p) > available {
		w.overflow = true
		p = p[:available]
	}
	w.data = append(w.data, p...)
	return n, nil
}
func (c LeaseCLI) call(ctx context.Context, operation string, args ...string) (leaseEnvelope, error) {
	var out leaseEnvelope
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// LookPath must use the caller snapshot, not a composed provider environment.
	env := c.Environment
	if env == nil {
		env = os.Environ()
	}
	path := ""
	for _, s := range env {
		if strings.HasPrefix(s, "PATH=") {
			path = strings.TrimPrefix(s, "PATH=")
		}
	}
	executable, err := lookPathIn("curator", path)
	if err != nil {
		return out, leaseFailure("curator_lease_unreachable")
	}
	argv := append([]string{"env", "lease", operation}, args...)
	argv = append(argv, "--format", "json")
	cmd := exec.CommandContext(ctx, executable, argv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	defer killGroup(cmd)
	cmd.Env = env
	cmd.Stdin = nil
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	var stdout leaseOutput
	cmd.Stdout = &stdout
	err = cmd.Run()
	exit := 0
	if err != nil {
		var e *exec.ExitError
		if errors.As(err, &e) {
			exit = e.ExitCode()
		} else {
			return out, leaseFailure("curator_lease_unreachable")
		}
	}
	if ctx.Err() != nil {
		return out, leaseFailure("curator_lease_timeout")
	}
	if operation == "list" && exit == 2 && len(stdout.data) == 0 {
		return out, leaseFailure("curator_lease_unsupported")
	}
	if stdout.overflow || schema.Validate("lease-result.schema.json", stdout.data) != nil {
		return out, leaseFailure("curator_lease_protocol_error")
	}
	if json.Unmarshal(stdout.data, &out) != nil || out.Operation != operation || out.Exit != exit {
		return out, leaseFailure("curator_lease_protocol_error")
	}
	if !out.OK {
		details := map[string]string{"reason": out.Reason}
		if out.Code != nil {
			details["curator_code"] = *out.Code
		}
		return out, refusal("session_home_lease_unavailable", details)
	}
	return out, nil
}
func lookPathIn(name, path string) (string, error) {
	for _, dir := range strings.Split(path, string(os.PathListSeparator)) {
		if dir == "" || !strings.HasPrefix(dir, "/") {
			continue
		}
		p := dir + "/" + name
		info, e := os.Stat(p)
		if e == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}
func (c LeaseCLI) List(ctx context.Context) ([]LeaseRecord, error) {
	e, err := c.call(ctx, "list")
	if err != nil {
		return nil, err
	}
	if e.Reason != "listed" || e.Result.Leases == nil {
		return nil, leaseFailure("curator_lease_protocol_error")
	}
	return e.Result.Leases, nil
}
func (c LeaseCLI) Acquire(ctx context.Context, p Pending) (*LeaseRecord, string, error) {
	l := p.Lease
	h, err := json.Marshal(l.Holder)
	if err != nil {
		return nil, "", leaseFailure("curator_lease_protocol_error")
	}
	e, err := c.call(ctx, "acquire", "--env", l.Environment, "--profile", p.Profile, "--lock-sha256", l.LockSHA256, "--request-id", l.RequestID, "--holder", string(h))
	if err != nil {
		return nil, "", err
	}
	r := e.Result.Lease
	if r == nil || r.Environment != l.Environment || r.Profile != p.Profile || r.LockSHA256 != l.LockSHA256 || r.Holder.RequestID != l.RequestID || r.Holder.ManagerID != p.Manager.ID || r.Holder.ManagerRegisteredAt != p.Manager.RegisteredAt || !reflect.DeepEqual(r.Holder.Werkstatt, l.Holder.Werkstatt) {
		return nil, "", leaseFailure("curator_lease_protocol_error")
	}
	if l.Record != nil && (r.LeaseID != l.Record.LeaseID || r.ManagerRootID != l.Record.ManagerRootID || r.HomeID != l.Record.HomeID) {
		return nil, "", leaseFailure("curator_lease_protocol_error")
	}
	// A terminal record may retain a later bound holder; it never admits a call.
	if e.Reason == "already_released" || e.Reason == "already_broken" {
		return r, e.Reason, nil
	}
	if e.Reason != "acquired" || r.Phase != "prepared" || !reflect.DeepEqual(r.Holder, l.Holder) {
		return nil, "", leaseFailure("curator_lease_protocol_error")
	}

	return r, e.Reason, nil
}
func blocking(r LeaseRecord) bool {
	return r.Phase == "prepared" || r.Phase == "active" || r.Phase == "unknown"
}
func Allocation(records []LeaseRecord, admission Pending) error {
	l := admission.Lease
	for _, r := range records {
		if !blocking(r) {
			continue
		}
		sameHome := l.Record != nil && r.ManagerRootID == l.Record.ManagerRootID && r.HomeID == l.Record.HomeID
		selected := r.Environment == l.Environment && r.Profile == admission.Profile
		sameLane := reflect.DeepEqual(r.Holder.Werkstatt, l.Holder.Werkstatt)
		if (sameHome || selected) && !sameLane {
			return leaseFailure("allocation_conflict")
		}
		if l.Holder.Werkstatt != nil && sameLane && (!selected || r.LockSHA256 != l.LockSHA256 || l.Record != nil && !sameHome) {
			return leaseFailure("allocation_conflict")
		}
	}
	return nil
}
func leaseEvidence(d Describe, runtime string) ([]string, []string) {
	caps := slices.Clone(d.Capabilities)
	ops := slices.Clone(d.Runtimes[runtime])
	slices.Sort(caps)
	slices.Sort(ops)
	return caps, ops
}

// WriteLeaseBreakHint prints a separate operator action using current public
// read-only evidence. An unavailable revision never becomes a runnable command.
func WriteLeaseBreakHint(ctx context.Context, w io.Writer, c LeaseCLI, p Pending) {
	records, err := c.List(ctx)
	if err == nil {
		for _, r := range records {
			if r.Holder.RequestID == p.Lease.RequestID && r.Environment == p.Lease.Environment && r.Profile == p.Profile && r.Holder.ManagerID == p.Manager.ID && r.Holder.ManagerRegisteredAt == p.Manager.RegisteredAt {
				fmt.Fprintf(w, "curator-run: retained lease=%s revision=%d; operator action: curator env lease break --lease %s --if-revision %d --operator --reason <reason> --format json\n", r.LeaseID, r.Revision, r.LeaseID, r.Revision)
				return
			}
		}
	}
	fmt.Fprintln(w, "curator-run: retained home lease; current lease/revision unavailable; inspect curator env lease list --format json before a deliberate operator break")
}
