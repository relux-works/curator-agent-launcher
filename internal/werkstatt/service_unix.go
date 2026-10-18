//go:build unix

package werkstatt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/configfile"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/werkstattcommand"
)

type Command = werkstattcommand.Command

type Remote interface {
	client.Caller
	ListAll(context.Context, map[string]string, int) ([]client.Session, error)
}
type Prepared struct {
	Request []byte
	Observe func(string)
}
type Status struct {
	Record   *Record          `json:"record"`
	Liveness *client.Liveness `json:"liveness"`
	A2A      any              `json:"a2a"`
	Stale    bool             `json:"stale"`
	Manager  *client.Session  `json:"manager_session"`
}
type Service struct {
	Loader            Loader
	Store             client.Store
	Registry          func() (sessionmanager.Registry, error)
	Trial             func() (bool, error)
	Client            func(string, sessionmanager.Entry) Remote
	Environment       []string
	IdentityDirectory string
	Prepare           func(context.Context, *Record, Command, string) (Prepared, error)
	Attach            func(context.Context, string, sessionmanager.Entry, *client.Session) (int, error)
	Presence          func(context.Context, *Record) any
	Stdout, Stderr    io.Writer
	Now               func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s Service) remote(id string, e sessionmanager.Entry) Remote {
	if s.Client != nil {
		return s.Client(id, e)
	}
	return client.Client{ID: id, Entry: e, Environment: s.Environment}
}
func binding(id string, e sessionmanager.Entry) client.Binding {
	return client.Binding{ID: id, Executable: e.Executable, RegisteredAt: e.RegisteredAt}
}
func (s Service) reconciler(r sessionmanager.Registry) client.Reconciler {
	return client.Reconciler{Registry: r.Managers, Now: s.Now, Caller: func(id string, e sessionmanager.Entry) client.Caller { return s.remote(id, e) }}
}
func call(ctx context.Context, c client.Caller, op string, request any) (client.Response, error) {
	b, _ := json.Marshal(request)
	return c.Call(ctx, op, b)
}
func owner(reg sessionmanager.Registry, l *client.Liveness) (sessionmanager.Entry, error) {
	e, ok := reg.Managers[l.Manager.ID]
	if !ok {
		return e, failure("werkstatt_operation_unresolved", map[string]string{"reason": "manager_missing"})
	}
	if binding(l.Manager.ID, e) != l.Manager {
		return e, failure("werkstatt_operation_unresolved", map[string]string{"reason": "manager_changed"})
	}
	return e, nil
}
func (s Service) ownerRead(ctx context.Context, reg sessionmanager.Registry, l *client.Liveness) (*client.Session, error) {
	if l == nil || l.Session == "" {
		return nil, nil
	}
	e, err := owner(reg, l)
	if err != nil {
		return nil, err
	}
	response, err := call(ctx, s.remote(l.Manager.ID, e), "status", map[string]string{"session": l.Session})
	if err != nil || response.Session == nil || !response.Session.Readable || response.Session.State == "unknown" {
		return response.Session, failure("session_liveness_unknown", nil)
	}
	return response.Session, nil
}
func refresh(tx *client.Transaction, l *client.Liveness, v *client.Session, at time.Time) error {
	next := *l
	next.State, next.Session, next.Generation = v.State, v.Session, v.Generation
	op := v.OperationID
	next.OperationID = &op
	next.StateSource = "manager"
	next.StartedAt, next.LastActivityAt = v.StartedAt, v.LastActivityAt
	next.Doorbell, next.DoorbellUncertain = v.Doorbell.State, v.Doorbell.Uncertain
	next.LastHeartbeatAt = at.UTC().Format(time.RFC3339)
	written, err := tx.RefreshSession(l.Expected(), next)
	if err != nil {
		return err
	}
	if !written {
		return failure("werkstatt_liveness_write_failed", map[string]string{"session": v.Session})
	}
	return nil
}
func (s Service) Run(ctx context.Context, c Command) (int, error) {
	if c.Action == "list" || c.Action == "status" && c.ID == "" {
		entries, err := configfile.ReadDirectory(s.Loader.Directory)
		if err != nil {
			return 0, invalid("", "file")
		}
		rows := []Status{}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".json")
			if !ValidID(id) {
				return 0, invalid("", "id")
			}
			row, _, err := s.one(ctx, Command{Action: "status", ID: id})
			if err != nil {
				return 0, err
			}
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Record.ID < rows[j].Record.ID })
		return 0, s.output(c, rows)
	}
	row, exit, err := s.one(ctx, c)
	if err != nil {
		return exit, err
	}
	if c.Action == "status" {
		return exit, s.output(c, []Status{row})
	}
	return exit, nil
}
func (s Service) output(c Command, rows []Status) error {
	asJSON := c.JSON
	for _, row := range rows {
		asJSON = asJSON || row.Stale
	}
	if asJSON {
		return json.NewEncoder(s.Stdout).Encode(rows)
	}
	for _, r := range rows {
		state, source, session := "unknown", "none", ""
		if r.Liveness != nil {
			state, source, session = r.Liveness.State, r.Liveness.StateSource, r.Liveness.Session
		}
		if _, err := fmt.Fprintf(s.Stdout, "%s state=%s state_source=%s session=%s stale=%t a2a=%v\n", r.Record.ID, state, source, session, r.Stale, r.A2A); err != nil {
			return err
		}
	}
	return nil
}
func (s Service) one(ctx context.Context, c Command) (row Status, exit int, result error) {
	tx, err := s.Store.Lock(ctx, c.ID)
	if err != nil {
		if c.Action != "status" || !client.IsCode(err, "werkstatt_busy") {
			return row, 0, err
		}
		row.Record, _, err = s.Loader.Load(ctx, c.ID)
		if err != nil {
			return row, 0, err
		}
		row.Liveness, err = s.Store.Snapshot(c.ID)
		row.Stale = true
		return row, 0, err
	}
	defer tx.Close()
	if c.AbandonOperation {
		if err := tx.Abandon(s.now()); err != nil {
			return row, 0, err
		}
	}
	p, err := tx.Pending()
	if err != nil {
		return row, 0, err
	}
	var reg sessionmanager.Registry
	loaded := false
	load := func() error {
		if loaded {
			return nil
		}
		var err error
		reg, err = s.Registry()
		loaded = err == nil
		return err
	}
	if p != nil {
		if err = load(); err != nil {
			return row, 0, err
		}
		recovered, err := s.reconciler(reg).Recover(ctx, tx)
		if err != nil {
			return row, 0, err
		}
		if recovered != nil && c.Action == "up" {
			fmt.Fprintln(s.Stdout, recovered.Session)
			if c.Attach && s.Attach != nil {
				exit, err = s.Attach(ctx, p.Manager.ID, reg.Managers[p.Manager.ID], recovered)
			}
			return row, exit, err
		}
	}
	row.Record, _, err = s.Loader.Load(ctx, c.ID)
	if err != nil {
		return row, 0, err
	}
	row.Liveness, err = tx.Liveness()
	if err != nil {
		return row, 0, err
	}
	if err = load(); err != nil {
		return row, 0, err
	}
	switch c.Action {
	case "up":
		exit, err = s.up(ctx, c, tx, reg, row.Record, row.Liveness)
	case "down", "recycle":
		err = s.down(ctx, c, tx, reg, row.Liveness)
		if err == nil && c.Action == "recycle" {
			l, e := tx.Liveness()
			if e != nil {
				return row, 0, e
			}
			pollCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			drainingTimeout := func() error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return failure("werkstatt_busy", map[string]string{"state": "draining"})
			}
			for l != nil && l.State == "draining" {
				select {
				case <-ctx.Done():
					return row, 0, ctx.Err()
				case <-pollCtx.Done():
					return row, 0, drainingTimeout()
				case <-time.After(time.Second):
				}
				v, e := s.ownerRead(pollCtx, reg, l)
				if pollCtx.Err() != nil {
					return row, 0, drainingTimeout()
				}
				if e != nil {
					return row, 0, e
				}
				if e = refresh(tx, l, v, s.now()); e != nil {
					return row, 0, e
				}
				l, e = tx.Liveness()
				if e != nil {
					return row, 0, e
				}
			}
			c.New = true
			exit, err = s.up(ctx, c, tx, reg, row.Record, l)
		}
	case "status":
		if c.RebindManager && row.Liveness != nil && row.Liveness.Session != "" {
			l := row.Liveness
			e, ok := reg.Managers[l.Manager.ID]
			if !ok {
				return row, 0, failure("werkstatt_operation_unresolved", map[string]string{"reason": "manager_missing"})
			}
			response, eErr := call(ctx, s.remote(l.Manager.ID, e), "status", map[string]string{"session": l.Session})
			if eErr != nil {
				return row, 0, eErr
			}
			if response.Session == nil || response.Session.Session != l.Session {
				return row, 0, failure("session_liveness_unknown", nil)
			}
			written, eErr := tx.Rebind(l.Expected(), binding(l.Manager.ID, e))
			if eErr != nil {
				return row, 0, eErr
			}
			if !written {
				return row, 0, failure("werkstatt_liveness_write_failed", nil)
			}
			row.Liveness, err = tx.Liveness()
			if err != nil {
				return row, 0, err
			}
		}
		// An explicit override is shown as a decision in this invocation. A later
		// ordinary status goes back to the owner and replaces it with evidence.
		if !c.AbandonOperation && row.Liveness != nil && row.Liveness.Session != "" {
			v, e := s.ownerRead(ctx, reg, row.Liveness)
			row.Manager = v
			if e != nil {
				if !client.IsCode(e, "session_liveness_unknown") {
					return row, 0, e
				}
				next := *row.Liveness
				next.State = "unknown"
				if v != nil {
					next.StateSource = "manager"
				}
				next.LastHeartbeatAt = s.now().UTC().Format(time.RFC3339)
				written, writeErr := tx.RefreshSession(row.Liveness.Expected(), next)
				if writeErr != nil {
					return row, 0, writeErr
				}
				if !written {
					return row, 0, failure("werkstatt_liveness_write_failed", nil)
				}
			} else if err = refresh(tx, row.Liveness, v, s.now()); err != nil {
				return row, 0, err
			}
			row.Liveness, err = tx.Liveness()
			if err != nil {
				return row, 0, err
			}
		}
		if row.Liveness == nil && (c.SetTask || c.QueueDepth != nil) {
			return row, 0, failure("werkstatt_not_up", map[string]string{"id": c.ID})
		}
		if row.Liveness != nil && (c.SetTask || c.QueueDepth != nil) {
			written, e := tx.EditDispatch(row.Liveness.Expected(), c.SetTask, c.Task, c.QueueDepth)
			if e != nil {
				return row, 0, e
			}
			if !written {
				return row, 0, failure("werkstatt_liveness_write_failed", nil)
			}
		}
		row.Liveness, err = tx.Liveness()
		if s.Presence != nil {
			row.A2A = s.Presence(ctx, row.Record)
		}
	}
	return row, exit, err
}
func (s Service) up(ctx context.Context, c Command, tx *client.Transaction, reg sessionmanager.Registry, r *Record, l *client.Liveness) (int, error) {
	selected, err := sessionmanager.Resolve(reg, sessionmanager.ExecutableAvailable)
	if err != nil {
		return 0, err
	}
	if selected.Kind != "selected" {
		return 0, failure("werkstatt_session_manager_required", map[string]string{"state": selected.Kind})
	}
	remote := s.remote(selected.ID, selected.Entry)
	response, err := call(ctx, remote, "describe", map[string]any{})
	if err != nil {
		return 0, err
	}
	trial, err := s.Trial()
	if err != nil {
		return 0, err
	}
	needs := client.Needs{Runtime: r.Runtime, Werkstatt: true, Detached: true, AttachAfter: c.Attach, DelegatedTrial: trial, Doorbell: r.Doorbell != nil}
	if _, err = client.Required(*response.Describe, needs); err != nil {
		return 0, err
	}
	var confirmed *client.Session
	if l != nil && l.State != "down" {
		confirmed, err = s.ownerRead(ctx, reg, l)
		if err != nil {
			return 0, err
		}
		if confirmed != nil && confirmed.State != "down" {
			return 0, failure("werkstatt_already_up", map[string]string{"session": confirmed.Session})
		}
	}
	sessions, err := remote.ListAll(ctx, map[string]string{"werkstatt": r.ID}, 0)
	if err != nil {
		return 0, err
	}
	for _, v := range sessions {
		if !v.Readable {
			code := "session_liveness_unknown"
			if r.ContextMode == "continue" && !c.New {
				code = "session_resume_ambiguous"
			}
			return 0, failure(code, nil)
		}
		if v.State == "unknown" {
			return 0, failure("session_liveness_unknown", nil)
		}
		if v.State != "down" {
			return 0, failure("werkstatt_already_up", map[string]string{"session": v.Session})
		}
	}
	target := ""
	selectedBinding := binding(selected.ID, selected.Entry)
	if r.ContextMode == "continue" && !c.New && (l == nil || l.Manager == selectedBinding) {
		if l != nil {
			if confirmed != nil && confirmed.State == "down" {
				target = confirmed.Session
			}
			for _, v := range sessions {
				if v.Session == l.Session && v.State == "down" {
					target = v.Session
				}
			}
		}
		if target == "" {
			latest := ""
			tie := false
			for _, v := range sessions {
				if v.State != "down" {
					continue
				}
				if v.StartedAt > latest {
					target, latest, tie = v.Session, v.StartedAt, false
				} else if v.StartedAt == latest {
					tie = true
				}
			}
			if tie {
				return 0, failure("session_resume_ambiguous", nil)
			}
		}
	}
	if target != "" {
		needs.Resume = true
		if _, err = client.Required(*response.Describe, needs); err != nil {
			return 0, err
		}
	}
	if err = Workspace(ctx, r, s.IdentityDirectory); err != nil {
		return 0, err
	}
	fmt.Fprintf(s.Stderr, "curator-run: werkstatt %s source=%s\n", r.ID, filepath.Join(s.Loader.Directory, r.ID+".json"))
	if confirmed != nil && confirmed.State == "down" && l != nil && l.State != "down" {
		if err = refresh(tx, l, confirmed, s.now()); err != nil {
			return 0, err
		}
	}
	prepared, err := s.Prepare(ctx, r, c, target)
	if err != nil {
		return 0, err
	}
	op := "launch"
	if target != "" {
		op = "resume"
	}
	if _, err = client.RequiredForRequest(*response.Describe, op, prepared.Request, needs); err != nil {
		return 0, err
	}
	var correlation struct {
		OperationID string `json:"operation_id"`
		SubmittedAt string `json:"submitted_at"`
	}
	_ = json.Unmarshal(prepared.Request, &correlation)
	p := client.Pending{OperationID: correlation.OperationID, Kind: op, SubmittedAt: correlation.SubmittedAt, AdmissionKey: "werkstatt:" + r.ID, Manager: selectedBinding}
	session, err := s.reconciler(reg).Submit(ctx, tx, p, prepared.Request)
	if prepared.Observe != nil {
		outcome := "launched"
		if err != nil {
			var e *client.Error
			if errors.As(err, &e) {
				outcome = "refused:" + e.Code
			}
		} else if session != nil {
			outcome = "hosted:" + session.Session
		}
		prepared.Observe(outcome)
	}
	if session != nil {
		fmt.Fprintln(s.Stdout, session.Session)
	}
	if err != nil {
		return 0, err
	}
	if c.Attach && s.Attach != nil {
		return s.Attach(ctx, selected.ID, selected.Entry, session)
	}
	return 0, nil
}
func (s Service) down(ctx context.Context, c Command, tx *client.Transaction, reg sessionmanager.Registry, l *client.Liveness) error {
	if l == nil || l.Session == "" {
		fmt.Fprintln(s.Stdout, "down")
		return nil
	}
	v, err := s.ownerRead(ctx, reg, l)
	if err != nil {
		return err
	}
	if v.State == "down" {
		if err = refresh(tx, l, v, s.now()); err == nil {
			fmt.Fprintln(s.Stdout, "down")
		}
		return err
	}
	e, err := owner(reg, l)
	if err != nil {
		return err
	}
	mode, precondition := "graceful", "not-busy"
	if c.Force {
		mode, precondition = "force", "none"
	}
	response, err := call(ctx, s.remote(l.Manager.ID, e), "stop", map[string]any{"session": v.Session, "expected_generation": v.Generation, "mode": mode, "precondition": precondition})
	if err != nil {
		return err
	}
	if response.Session == nil || !response.Session.Readable {
		return failure("session_liveness_unknown", nil)
	}
	if err = refresh(tx, l, response.Session, s.now()); err != nil {
		return err
	}
	fmt.Fprintln(s.Stdout, response.Session.State)
	return nil
}
