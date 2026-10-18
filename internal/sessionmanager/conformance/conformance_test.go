//go:build unix

package conformance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/relux-works/curator-agent-launcher/internal/hosted"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/client"
	"github.com/relux-works/curator-agent-launcher/internal/sessionmanager/fakemanager"
)

type scenario struct {
	Name  string
	Tier  string
	Run   func(*harness) error
	Plant string
	// Space-separated operation and capability prerequisites, including runtime cells.
	Needs string
}

func scenarios() []scenario {
	cases := []scenario{
		// A normal describe response obeys the shared wire schema.
		{"wire", "A", wireCase, "wire", ""},
		// Unknown operations refuse without replacing a valid describe.
		{"unknown-operation", "A", unknownCase, "unknown-operation", ""},
		// A requested unadvertised optional capability refuses.
		{"missing-capability", "A", missingCapabilityCase, "missing-capability", "launch"},
		// A valid but unadvertised runtime refuses.
		{"unsupported-runtime", "A", unsupportedRuntimeCase, "unsupported-runtime", "launch"},
		// Each advertised launch runtime supports its lifecycle operations.
		{"advertised-operations", "A", advertisedCase, "advertised-operations", ""},
		// A different resolved fragment digest refuses a fresh start.
		{"profile-digest", "B", profileCase, "profile-digest", "launch"},
		// Secret-shaped child values refuse; harmless literals admit.
		{"secret-child-env", "A", secretCase(false), "request-validation", "launch"},
		// Reserved child names refuse; harmless names admit.
		{"reserved-child-env", "A", secretCase(true), "request-validation", "launch"},
		// Stopping an unknown handle refuses after a valid stop control.
		{"unknown-stop", "A", unknownStopCase, "unknown-stop", "launch stop"},
		// Semantic JSON replay preserves the result and provider count.
		{"idempotency", "A", idempotencyCase, "idempotency", "launch"},
		// Listing independently confirms replay creates no second session.
		{"idempotency-list", "A", idempotencyListCase, "idempotency", "launch list"},
		// An old down handle cannot release a newer key owner.
		{"admission-recycle", "A", recycleCase, "admission-ownership", "launch stop admission/1"},
		// A different request digest cannot replace the original operation.
		{"operation-conflict", "A", conflictCase, "operation-conflict", "launch"},
		// A durable tombstone prevents late admission.
		{"cancel-fence", "A", cancelCase(false), "cancel-fence", "launch cancel list"},
		// Cancelling prepared evidence prevents a late start.
		{"prepared-cancel", "B", cancelCase(true), "cancel-fence", "launch cancel list"},
		// Stale attach refuses without changing the current generation.
		{"generation", "A", generationCase, "generation", "launch attach status"},
		// Stale stop refuses and the current generation can still stop.
		{"generation-stop", "A", stopGenerationCase, "generation", "launch stop status"},
		// Pagination counts an isolated fixture or a unique label scope.
		{"pagination", "A", paginationCase, "pagination", "launch list"},
		// Exact indexed values match; a value prefix never matches.
		{"label-subset", "A", labelCase, "labels", "launch list labels/1"},
		// A missing target cannot create or start a session.
		{"resume-missing", "A", resumeFailureCase("missing"), "resume-missing", "launch stop resume list"},
		// A foreign scope cannot resume or create a session.
		{"resume-location", "A", resumeFailureCase("location"), "resume-location", "launch stop resume list"},
		// A different requested profile name cannot replace stored identity.
		{"resume-identity", "B", resumeFailureCase("identity"), "resume-identity", "launch stop resume list"},
		// Tied latest candidates refuse; an explicit handle disambiguates.
		{"resume-ambiguous", "B", ambiguousCase, "resume-ambiguous", "launch stop resume"},
		// Resume preserves the original admitted policy digest.
		{"resume-preserves-policy", "B", resumePolicyCase, "resume-policy", "launch stop resume"},
		// Busy stop refusal changes no records, gate or provider.
		{"not-busy-busy", "B", notBusyCase("busy"), "not-busy", "launch stop"},
		// Unknown activity refuses not-busy with no lifecycle effects.
		{"not-busy-unknown", "B", notBusyCase("unknown"), "not-busy", "launch stop"},
		// Activity events, rather than polling or record updates, advance the timestamp.
		{"activity-timestamp", "B", activityTimestampCase, "activity-on-read", "launch status list"},
		// Missing evidence remains unknown; busy evidence is observable.
		{"unknown-liveness", "B", unknownLivenessCase, "unknown-liveness", "launch status"},
		// Stored drift wins refusal precedence during resume, retry and recovery.
		{"identity-drift", "B", identityCase, "identity-drift", "launch stop resume status"},
		// Old launch refuses; the exact retention boundary still admits.
		{"operation-expiry", "B", expiryCase, "operation-expiry", "launch"},
		// Resume expiry is independent of launch-only expiry validation.
		{"resume-operation-expiry", "B", resumeExpiryCase, "operation-expiry", "launch stop resume"},
		// Started, failed and cancelled evidence survives seven days.
		{"ledger-retention", "B", retentionCase, "ledger-retention", "launch cancel status admission/1"},
		// Pagination preserves membership and values across later mutations.
		{"snapshot", "B", snapshotCase, "snapshot", "launch stop list"},
		// Unreadable status/list retain only indexed identity and labels.
		{"unreadable-row", "B", unreadableCase, "unreadable", "launch list status resume"},
		// Missing label evidence refuses the entire filtered page.
		{"filtered-index", "B", indexCase("missing-labels"), "index-unreadable", "launch list labels/1"},
		// Missing handle evidence refuses the entire page.
		{"unreadable-index", "B", indexCase("missing-handle"), "index-unreadable", "launch list"},
		// An unreadable current record cannot establish historical started.
		{"started-unreadable", "B", unreadableOutcomeCase, "started-unreadable", "launch status"},
		// An invalid unreadable-started packet poisons every result field.
		{"started-unreadable-poison", "B", unreadablePoisonCase, "poison-started-unreadable", "launch status"},
		// An uncertain start keeps its admission key and starts no duplicate.
		{"unknown-start", "B", crashCase, "unknown-start", "launch status admission/1"},
		// Reservation survives an actual process exit before provider start.
		{"reserved-crash", "B", reservationCase, "reservation", "launch status admission/1"},
		// Closing an armed generation gate immediately projects off at that generation.
		{"doorbell-gate-closure", "B", doorbellGateCase, "doorbell-gate-closure", "launch stop doorbell"},
		// Stop and rebind preserve persisted prior uncertainty.
		{"doorbell-uncertain", "B", doorbellCase, "doorbell-uncertain", "launch stop resume doorbell"},
		// A live graceful timeout drains; retirement finishes after provider exit.
		{"graceful-retirement", "B", gracefulCase, "graceful-retirement", "launch stop status"},
		// Rejected credential bytes never reach output or persisted records.
		{"credential-sentinel", "B", credentialCase, "credential-leak", "launch"},
		// Stale attach and unsupported exact launch emit fd4 refusals.
		{"attach-framing", "A", attachCase(false), "attach-framing", "launch attach"},
		// A plain terminal exit remains distinct from a typed fd4 refusal.
		{"attach-plain-exit", "B", attachCase(true), "attach-plain-exit", "launch attach"},
	}
	// Each concurrent operation pairing admits exactly one active generation.
	for _, pair := range []string{"launch-launch", "launch-resume", "resume-resume"} {
		p := pair
		cases = append(cases, scenario{"admission-" + p, "A", admissionCase(p), "admission", "launch stop list admission/1" + resumeNeed(p)})
	}
	// Each loss boundary reconciles to its own durable outcome and start count.
	for _, fault := range []string{"before-admission", "after-admission", "failed-start"} {
		f := fault
		cases = append(cases, scenario{"lost-" + f, "B", lostCase(f), "lost-" + f, "launch status"})
	}
	// Every malformed wire/detail variant yields exit 6 without hostile diagnostics.
	for _, fault := range []string{"malformed", "over-bound", "protocol", "exit-ok", "detail-text", "detail-control", "detail-secret"} {
		f := fault
		cases = append(cases, scenario{"poison-" + f, "B", poisonCase(f), "poison-" + f, "launch status"})
	}
	return cases
}

// Every case pairs a valid request/control with the named rejection or lifecycle
// invariant. Tier B is never counted as passed without the separate adapter.
func TestConformance(t *testing.T) {
	binary := *plugin
	if binary == "" {
		binary = buildFake(t)
	}
	runConformance(t, func(t *testing.T) *harness { return newHarness(t, binary, nil) })
}

func resumeNeed(pair string) string {
	if strings.Contains(pair, "resume") {
		return " resume"
	}
	return ""
}

var operationCapability = map[string]string{"launch": "launch-delegated/1", "resume": "resume/1", "attach": "attach/1", "status": "status/1", "cancel": "admission/1", "list": "list/1", "stop": "stop/1"}

func (c scenario) applicable(h *harness, d *client.Describe) error {
	runtime, _ := h.template["runtime"].(string)
	for _, need := range strings.Fields(c.Needs) {
		cap := need
		if need == "doorbell" {
			cap = map[string]string{"codex_cli": "doorbell-codex/1", "claude_code": "doorbell-claude/1"}[runtime]
		}
		if v := operationCapability[need]; v != "" {
			cap = v
		}
		if cap == "" || !slices.Contains(d.Capabilities, cap) {
			return &notApplicable{Reason: "requires " + need}
		}
		if operationCapability[need] != "" && !slices.Contains(d.Runtimes[runtime], need) {
			return &notApplicable{Reason: "requires runtime " + runtime + "/" + need}
		}
	}
	// The terminal refusal case specifically requests an absent exact capability.
	if c.Name == "attach-framing" && slices.Contains(d.Capabilities, "launch-exact/2") {
		return &notApplicable{Reason: "exact capability advertised; absence probe does not apply"}
	}
	return nil
}

func runConformance(t *testing.T, makeHarness func(*testing.T) *harness) {
	unqualified := 0
	for _, c := range scenarios() {
		t.Run(c.Tier+"/"+c.Name, func(t *testing.T) {
			if c.Tier == "B" && *plugin != "" && *fixture == "" {
				unqualified++
				t.Skip("UNQUALIFIED: tier B requires -fixture")
			}
			h := makeHarness(t)
			d, e := h.call("describe", map[string]any{})
			if e != nil {
				t.Fatal(setup(e))
			}
			if !slices.Contains(d.Describe.Capabilities, "labels/1") {
				h.template["labels"] = map[string]any{}
			}
			if e = c.applicable(h, d.Describe); e == nil {
				e = c.Run(h)
			}
			if e != nil {
				var na *notApplicable
				if errors.As(e, &na) {
					t.Skip(na.Error())
				}
				t.Fatal(e)
			}
		})
	}
	if unqualified > 0 {
		t.Fatalf("not conforming (tier B unqualified): %d cases", unqualified)
	}
}

// The same scenarios, not parallel assertions, must detect each named known-bad
// plugin plant. Setup failures never count as detection of the target property.
func TestKnownBadControls(t *testing.T) {
	if *plugin != "" {
		t.Skip("reference-only instrument controls; external qualification is TestConformance")
	}
	binary := buildFake(t)
	for _, c := range scenarios() {
		if c.Plant == "" {
			continue
		}
		plants := []string{c.Plant}
		if c.Name == "advertised-operations" {
			plants = append(plants, "advertised-resume")
		}
		if c.Name == "doorbell-uncertain" {
			plants = append(plants, "doorbell-generation")
		}
		if strings.HasPrefix(c.Name, "not-busy-") {
			plants = append(plants, "busy-side-effect")
		}
		for _, plant := range plants {
			t.Run(c.Name+"/"+plant, func(t *testing.T) {
				h := newHarness(t, binary, []string{plant})
				err := c.Run(h)
				var finding *violation
				if !errors.As(err, &finding) || finding.Setup || finding.Property != c.Name {
					t.Fatalf("plant %s was not detected as %s (got %v)", plant, c.Name, err)
				}
				// These controls must reach the effect scanners, not fail early on wire/code.
				if plant == "credential-leak" && !strings.Contains(finding.Detail, "sentinel reached") {
					t.Fatalf("credential scanner control failed elsewhere: %v", finding)
				}
				if plant == "busy-side-effect" && !strings.Contains(finding.Detail, "busy refusal changed") {
					t.Fatalf("effect scanner control failed elsewhere: %v", finding)
				}
				for selected, detail := range map[string]string{
					"activity-on-read":      "read manufactured activity",
					"doorbell-gate-closure": "closed gate must project off",
					"doorbell-generation":   "rebind must retain uncertainty and last armed generation",
					"advertised-resume":     "resume did not preserve handle",
				} {
					if plant == selected && !strings.Contains(finding.Detail, detail) {
						t.Fatalf("plant %s failed outside its target assertion: %v", plant, finding)
					}
				}
				t.Logf("KILLED %s: %s", plant, finding.Property)
			})
		}
	}
}

func wireCase(h *harness) error {
	v, e := h.call("describe", map[string]any{})
	if err := check("wire", e == nil && v.Describe != nil, "valid describe must decode"); err != nil {
		return err
	}
	return nil
}
func unknownCase(h *harness) error {
	if e := wireCase(h); e != nil {
		return setup(e)
	}
	for _, op := range []string{"not-an-operation", "fixture"} {
		_, e := h.call(op, map[string]any{"operation": "reset"})
		if e = h.expectCode("unknown-operation", "launch_plan_invalid", e); e != nil {
			return e
		}
	}
	return nil
}

func missingCapabilityCase(h *harness) error {
	d, e := h.call("describe", map[string]any{})
	if e != nil {
		return setup(e)
	}
	_, e = h.start(h.launch())
	if e != nil {
		return e
	}
	r := h.launch()
	field := ""
	if !slices.Contains(d.Describe.Capabilities, "board-goal/1") {
		r["board_goal"] = "workspace"
		field = "board-goal/1"
	} else if !slices.Contains(d.Describe.Capabilities, "doorbell-codex/1") {
		r["doorbell"] = map[string]any{"identity": "probe", "heartbeat": "30s", "max_wakes_per_hour": 1}
		field = "doorbell-codex/1"
	}
	if field == "" {
		return &notApplicable{Reason: "all optional capabilities advertised"}
	}
	_, e = h.call("launch", r)
	return h.expectCode("missing-capability", "session_host_capability_missing", e)
}
func unsupportedRuntimeCase(h *harness) error {
	d, e := h.call("describe", map[string]any{})
	if e != nil {
		return setup(e)
	}
	if _, e = h.start(h.launch()); e != nil {
		return e
	}
	for _, runtime := range []string{"muse", "claude_code", "codex_cli"} {
		if !slices.Contains(d.Describe.Runtimes[runtime], "launch") {
			r := h.launch()
			r["runtime"] = runtime
			_, e = h.call("launch", r)
			return h.expectCode("unsupported-runtime", "session_host_provider_unsupported", e)
		}
	}
	return &notApplicable{Reason: "all delegated runtimes advertised"}
}
func advertisedCase(h *harness) error {
	const p = "advertised-operations"
	d, e := h.call("describe", map[string]any{})
	if e != nil {
		return setup(e)
	}
	supports := func(ops []string, op string) bool {
		return slices.Contains(ops, op) && slices.Contains(d.Describe.Capabilities, operationCapability[op])
	}
	exercised := false
	for runtime, ops := range d.Describe.Runtimes {
		if !supports(ops, "launch") {
			continue
		}
		exercised = true
		r := h.launch()
		r["runtime"] = runtime
		if !slices.Contains(d.Describe.Capabilities, "labels/1") {
			r["labels"] = map[string]string{}
		}
		started, e := h.call("launch", r)
		if e != nil || started.Session == nil {
			return check(p, false, "advertised launch refused")
		}
		s := started.Session
		if s.Runtime != runtime {
			return check(p, false, "runtime correlation")
		}
		if supports(ops, "status") {
			if _, e = h.status(s); e != nil {
				return check(p, false, "status refused")
			}
		}
		if supports(ops, "list") {
			if _, e = h.all(r["labels"], 1); e != nil {
				return check(p, false, "list refused")
			}
		}
		if supports(ops, "attach") {
			code, exit, e := h.attach(s, s.Generation)
			if e != nil || code != "" || exit != 0 {
				return check(p, false, "attach refused")
			}
		}
		if supports(ops, "cancel") {
			v, e := h.call("cancel", map[string]any{"operation_id": operationID()})
			if e != nil || v.Outcome == nil || v.Outcome.Outcome != "cancelled" {
				return check(p, false, "cancel refused")
			}
		}
		stopped := supports(ops, "stop")
		if stopped {
			if _, e = h.stop(s, "force", "none"); e != nil {
				return check(p, false, "stop refused")
			}
		}
		if supports(ops, "resume") {
			resume := h.resume(s)
			resume["scope"].(map[string]any)["labels"] = r["labels"]
			if !stopped {
				// Without a stop prerequisite, probe lookup without requiring an active
				// session to restart. Full positive resume cases require advertised stop.
				resume["selector"] = map[string]string{"kind": "handle", "handle": "SES-absent"}
				_, e = h.call("resume", resume)
				if e = h.expectCode(p, "session_resume_not_found", e); e != nil {
					return e
				}
			} else {
				v, e := h.call("resume", resume)
				if e != nil || v.Session == nil || v.Session.Session != s.Session {
					return check(p, false, "resume did not preserve handle")
				}
			}
		}
	}
	if !exercised {
		return &notApplicable{Reason: "no delegated launch runtime advertised"}
	}
	return nil
}

func profileCase(h *harness) error {
	if _, e := h.start(h.launch()); e != nil {
		return e
	}
	r := h.launch()
	r["profile"].(map[string]any)["fragment_digest"] = "sha256:" + strings.Repeat("f", 64)
	_, e := h.call("launch", r)
	return h.expectCode("profile-digest", "policy_refused", e)
}
func secretCase(reserved bool) func(*harness) error {
	return func(h *harness) error {
		name := "secret-child-env"
		key, value := "SAFE_OPTION", "sk-conformance-sentinel"
		if reserved {
			name = "reserved-child-env"
			key, value = "CODEX_HOME", "safe"
		}
		r := h.launch()
		r["child_env"] = map[string]string{"SAFE_OPTION": "safe"}
		if _, e := h.start(r); e != nil {
			return e
		}
		r = h.launch()
		r["child_env"] = map[string]string{key: value}
		_, e := h.call("launch", r)
		return h.expectCode(name, "secret_policy_violation", e)
	}
}
func unknownStopCase(h *harness) error {
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	if _, e = h.stop(s, "force", "none"); e != nil {
		return setup(e)
	}
	_, e = h.stop(&client.Session{Session: "SES-unknown", Generation: 1}, "force", "none")
	return h.expectCode("unknown-stop", "session_host_unavailable", e)
}
func idempotencyCase(h *harness) error {
	r := h.launch()
	s, e := h.start(r)
	if e != nil {
		return e
	}
	pretty, _ := json.MarshalIndent(r, "", "  ")
	w, e := h.exec(h.plugin, "launch", pretty, false)
	if e != nil {
		return setup(e)
	}
	v, e := client.DecodeResponse("launch", pretty, w.Body, w.Exit, h.id)
	if err := check("idempotency", e == nil && reflect.DeepEqual(v.Session, s), "same operation must replay the original record"); err != nil {
		return err
	}
	if h.fixture.Path != "" {
		o, e := h.observe()
		if e != nil {
			return setup(e)
		}
		return check("idempotency", o.Starts == 1, "retry started provider twice")
	}
	return nil
}
func idempotencyListCase(h *harness) error {
	r := h.launch()
	if _, e := h.start(r); e != nil {
		return e
	}
	if _, e := h.start(r); e != nil {
		return e
	}
	rows, e := h.all(r["labels"], 1)
	return check("idempotency-list", e == nil && len(rows) == 1, "retry created another session")
}

func conflictCase(h *harness) error {
	r := h.launch()
	s, e := h.start(r)
	if e != nil {
		return e
	}
	changed := clone(r)
	changed["model"] = "changed"
	_, e = h.call("launch", changed)
	if e = h.expectCode("operation-conflict", "session_operation_conflict", e); e != nil {
		return e
	}
	v, e := h.call("launch", r)
	return check("operation-conflict", e == nil && reflect.DeepEqual(v.Session, s), "conflict changed the original operation")
}
func cancelCase(prepared bool) func(*harness) error {
	return func(h *harness) error {
		p := "cancel-fence"
		r := h.launch()
		if prepared {
			p = "prepared-cancel"
			if e := h.fault("prepared"); e != nil {
				return e
			}
			_, _ = h.call("launch", r)
			if e := h.fault(""); e != nil {
				return e
			}
		}
		id := map[string]any{"operation_id": r["operation_id"]}
		v, e := h.call("cancel", id)
		if e != nil {
			return setup(e)
		}
		if err := check(p, v.Outcome.Outcome == "cancelled", "cancel must make a durable tombstone"); err != nil {
			return err
		}
		_, e = h.call("launch", r)
		if err := h.expectCode(p, "session_host_unavailable", e); err != nil {
			return err
		}
		rows, e := h.all(r["labels"], 1)
		if e != nil {
			return setup(e)
		}
		if err := check(p, len(rows) == 0, "late request created a session"); err != nil {
			return err
		}
		if _, e = h.start(h.launch()); e != nil {
			return e
		}
		return nil
	}
}
func generationCase(h *harness) error {
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	bad := *s
	bad.Generation++
	code, exit, e := h.attach(s, bad.Generation)
	if err := check("generation", e == nil && code == "session_generation_stale" && exit == 1, "stale attach must use existing fd4 registry"); err != nil {
		return err
	}
	current, e := h.status(s)
	if e != nil {
		return setup(e)
	}
	if err := check("generation", current.State != "down" && current.Generation == s.Generation, "stale request changed provider"); err != nil {
		return err
	}
	code, exit, e = h.attach(s, s.Generation)
	return check("generation", e == nil && code == "" && exit == 0, "current generation attach positive control")
}
func stopGenerationCase(h *harness) error {
	const p = "generation-stop"
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	bad := *s
	bad.Generation++
	_, e = h.stop(&bad, "force", "none")
	if e = h.expectCode(p, "session_generation_mismatch", e); e != nil {
		return e
	}
	current, e := h.status(s)
	if e = check(p, e == nil && current.State != "down" && current.Generation == s.Generation, "stale stop changed provider"); e != nil {
		return e
	}
	v, e := h.stop(s, "force", "none")
	return check(p, e == nil && v.Session.State == "down", "current generation stop positive control")
}

func paginationCase(h *harness) error {
	r := h.launch()
	want := map[string]bool{}
	for i := 0; i < 3; i++ {
		r["operation_id"] = operationID()
		s, e := h.start(r)
		if e != nil {
			return e
		}
		want[s.Session] = true
	}
	rows, e := h.all(r["labels"], 1)
	if e != nil {
		return check("pagination", false, e.Error())
	}
	for _, s := range rows {
		if !want[s.Session] {
			return check("pagination", false, "unexpected or duplicate row")
		}
		delete(want, s.Session)
	}
	return check("pagination", len(rows) == 3 && len(want) == 0, "all pages must be complete, without duplicates")
}
func labelCase(h *harness) error {
	r := h.launch()
	r["labels"].(map[string]any)["role"] = "worker"
	s, e := h.start(r)
	if e != nil {
		return e
	}
	labels := clone(r["labels"].(map[string]any))
	rows, e := h.all(labels, 1)
	if e != nil {
		return setup(e)
	}
	if err := check("label-subset", len(rows) == 1 && rows[0].Session == s.Session, "exact matching positive control"); err != nil {
		return err
	}
	labels["role"] = "work"
	rows, e = h.all(labels, 1)
	return check("label-subset", e == nil && len(rows) == 0, "prefix values must not match")
}
func resumeFailureCase(kind string) func(*harness) error {
	return func(h *harness) error {
		p := "resume-" + kind
		s, e := h.start(h.launch())
		if e != nil {
			return e
		}
		if _, e = h.stop(s, "force", "none"); e != nil {
			return setup(e)
		}
		r := h.resume(s)
		code := "session_resume_not_found"
		switch kind {
		case "missing":
			r["selector"] = map[string]string{"kind": "handle", "handle": "SES-absent"}
		case "location":
			r["scope"].(map[string]any)["cwd"] = h.root + "/elsewhere"
			code = "session_resume_location_mismatch"
		case "identity":
			r["profile"].(map[string]any)["name"] = "other"
			code = "session_resume_identity_mismatch"
		}
		_, e = h.call("resume", r)
		if e = h.expectCode(p, code, e); e != nil {
			return e
		}
		rows, e := h.all(h.template["labels"], 1)
		if e != nil {
			return setup(e)
		}
		if err := check(p, len(rows) == 1 && rows[0].State == "down", "refused resume must not create/start a session"); err != nil {
			return err
		}
		v, e := h.call("resume", h.resume(s))
		return check(p, e == nil && v.Session.Session == s.Session && v.Session.Generation > s.Generation, "valid resume positive control")
	}
}
func ambiguousCase(h *harness) error {
	now := time.Now().UTC().Truncate(time.Second)
	if _, e := h.control(fakemanager.FixtureRequest{Operation: "set_clock", Now: now.Format(time.RFC3339)}); e != nil {
		return setup(e)
	}
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	other, e := h.start(h.launch())
	if e != nil {
		return e
	}
	_, _ = h.stop(s, "force", "none")
	_, _ = h.stop(other, "force", "none")
	r := h.resume(s)
	r["selector"] = map[string]string{"kind": "latest"}
	_, e = h.call("resume", r)
	if e = h.expectCode("resume-ambiguous", "session_resume_ambiguous", e); e != nil {
		return e
	}
	v, e := h.call("resume", h.resume(s))
	return check("resume-ambiguous", e == nil && v.Session.Session == s.Session, "explicit handle disambiguates")
}
func resumePolicyCase(h *harness) error {
	r := h.launch()
	// The adapter supplies the admitted model and policy; preserve those values.
	s, e := h.start(r)
	if e != nil {
		return e
	}
	_, e = h.stop(s, "force", "none")
	if e != nil {
		return setup(e)
	}
	before, e := h.observe()
	if e != nil {
		return setup(e)
	}
	v, e := h.call("resume", h.resume(s))
	if e != nil {
		return setup(e)
	}
	after, e := h.observe()
	if e != nil {
		return setup(e)
	}
	return check("resume-preserves-policy", v.Session.Session == s.Session && before.PolicyDigests[s.Session] != "" && before.PolicyDigests[s.Session] == after.PolicyDigests[s.Session], "resume must preserve admitted policy bytes")
}
func notBusyCase(state string) func(*harness) error {
	return func(h *harness) error {
		p := "not-busy-" + state
		s, e := h.start(h.launch())
		if e != nil {
			return e
		}
		if e = h.activity(s, state); e != nil {
			return e
		}
		before, e := h.observe()
		if e != nil {
			return setup(e)
		}
		_, e = h.stop(s, "graceful", "not-busy")
		if e = h.expectCode(p, "werkstatt_busy", e); e != nil {
			return e
		}
		after, e := h.observe()
		if e != nil {
			return setup(e)
		}
		if err := check(p, reflect.DeepEqual(before, after), "busy refusal changed records, gate or provider"); err != nil {
			return err
		}
		if e = h.activity(s, "idle"); e != nil {
			return e
		}
		v, e := h.stop(s, "force", "not-busy")
		return check(p, e == nil && v.Session.State == "down", "idle positive control must stop")
	}
}
func unknownLivenessCase(h *harness) error {
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	if e = h.activity(s, "unknown"); e != nil {
		return e
	}
	v, e := h.status(s)
	if e != nil {
		return setup(e)
	}
	if err := check("unknown-liveness", v.State == "unknown" && v.LastActivityAt == nil, "absence of evidence must stay unknown"); err != nil {
		return err
	}
	if e = h.activity(s, "busy"); e != nil {
		return e
	}
	v, e = h.status(s)
	return check("unknown-liveness", e == nil && v.State == "busy" && v.LastActivityAt != nil, "evidence positive control")
}
func identityCase(h *harness) error {
	r := h.launch()
	s, e := h.start(r)
	if e != nil {
		return e
	}
	_, e = h.stop(s, "force", "none")
	if e != nil {
		return setup(e)
	}
	if _, e = h.control(fakemanager.FixtureRequest{Operation: "set_identity_drift", Enabled: true}); e != nil {
		return setup(e)
	}
	changed := h.resume(s)
	changed["profile"].(map[string]any)["name"] = "other"
	_, e = h.call("resume", changed)
	if e = h.expectCode("identity-drift", "session_curator_identity_stale", e); e != nil {
		return e
	}
	_, e = h.call("launch", r)
	if e = h.expectCode("identity-drift", "session_curator_identity_stale", e); e != nil {
		return e
	}
	v, e := h.call("status", map[string]any{"operation_id": r["operation_id"]})
	if err := check("identity-drift", e == nil && v.Outcome.Outcome == "failed" && v.Outcome.Code == "session_curator_identity_stale", "historical outcome must revalidate identity"); err != nil {
		return err
	}
	_, e = h.control(fakemanager.FixtureRequest{Operation: "set_identity_drift", Enabled: false})
	if e != nil {
		return setup(e)
	}
	v, e = h.call("resume", h.resume(s))
	return check("identity-drift", e == nil && v.Session.Session == s.Session, "restored identity positive control")
}
func expiryCase(h *harness) error {
	now := time.Now().UTC().Truncate(time.Second)
	_, e := h.control(fakemanager.FixtureRequest{Operation: "set_clock", Now: now.Format(time.RFC3339)})
	if e != nil {
		return setup(e)
	}
	r := h.launch()
	r["submitted_at"] = now.Add(-fakemanager.Retention - time.Second).Format(time.RFC3339)
	_, e = h.call("launch", r)
	if e = h.expectCode("operation-expiry", "session_operation_expired", e); e != nil {
		return e
	}
	r["operation_id"] = operationID()
	r["submitted_at"] = now.Add(-fakemanager.Retention).Format(time.RFC3339)
	_, e = h.call("launch", r)
	if err := check("operation-expiry", e == nil, "exact seven-day boundary must still admit"); err != nil {
		return err
	}
	return nil
}
func resumeExpiryCase(h *harness) error {
	const p = "resume-operation-expiry"
	now := time.Now().UTC().Truncate(time.Second)
	if _, e := h.control(fakemanager.FixtureRequest{Operation: "set_clock", Now: now.Format(time.RFC3339)}); e != nil {
		return setup(e)
	}
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	if _, e = h.stop(s, "force", "none"); e != nil {
		return setup(e)
	}
	resume := h.resume(s)
	resume["submitted_at"] = now.Add(-fakemanager.Retention - time.Second).Format(time.RFC3339)
	_, e = h.call("resume", resume)
	if e = h.expectCode(p, "session_operation_expired", e); e != nil {
		return e
	}
	resume["submitted_at"] = now.Add(-fakemanager.Retention).Format(time.RFC3339)
	resume["operation_id"] = operationID()
	v, e := h.call("resume", resume)
	return check(p, e == nil && v.Session.Session == s.Session, "resume boundary must still admit the same session")
}
func retentionCase(h *harness) error {
	r := h.launch()
	s, e := h.start(r)
	if e != nil {
		return e
	}

	cancelled := operationID()
	if _, e = h.call("cancel", map[string]any{"operation_id": cancelled}); e != nil {
		return setup(e)
	}
	failed := h.launch()
	failed["profile"].(map[string]any)["fragment_digest"] = "sha256:" + strings.Repeat("f", 64)
	_, e = h.call("launch", failed)
	if !client.IsCode(e, "policy_refused") {
		return setup(errors.New("failed outcome control"))
	}
	_, e = h.control(fakemanager.FixtureRequest{Operation: "set_clock", Now: retainedUntil(r["submitted_at"].(string))})
	if e != nil {
		return setup(e)
	}
	v, e := h.call("status", map[string]any{"operation_id": r["operation_id"]})
	if err := check("ledger-retention", e == nil && v.Outcome.Outcome == "started" && v.Outcome.Session.Session == s.Session, "ledger must survive seven days"); err != nil {
		return err
	}
	v, e = h.call("launch", r)
	if err := check("ledger-retention", e == nil && reflect.DeepEqual(v.Session, s), "retained request must replay original evidence"); err != nil {
		return err
	}
	for _, entry := range []struct {
		ID            any
		Outcome, Code string
	}{{cancelled, "cancelled", ""}, {failed["operation_id"], "failed", "policy_refused"}} {
		v, e = h.call("status", map[string]any{"operation_id": entry.ID})
		if err := check("ledger-retention", e == nil && v.Outcome.Outcome == entry.Outcome && v.Outcome.Code == entry.Code, "failed outcome/tombstone expired too early"); err != nil {
			return err
		}
	}
	return nil
}
func snapshotCase(h *harness) error {
	r := h.launch()
	a, e := h.start(r)
	if e != nil {
		return e
	}
	b, e := h.start(h.launch())
	if e != nil {
		return e
	}
	v, e := h.call("list", map[string]any{"labels": r["labels"], "limit": 1})
	if e != nil {
		return setup(e)
	}
	if v.Page.NextCursor == nil {
		return check("snapshot", false, "two-row positive control needs cursor")
	}
	first := v.Page.Sessions[0]
	_, e = h.start(h.launch())
	if e != nil {
		return e
	}
	_, _ = h.stop(a, "force", "none")
	_, _ = h.stop(b, "force", "none")
	v, e = h.call("list", map[string]any{"labels": r["labels"], "limit": 1, "cursor": *v.Page.NextCursor})
	return check("snapshot", e == nil && len(v.Page.Sessions) == 1 && v.Page.NextCursor == nil && v.Page.Sessions[0].Session != first.Session && v.Page.Sessions[0].State != "down", "continuation must preserve original membership and values")
}
func unreadableCase(h *harness) error {
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	if e = h.corrupt(s, "unreadable"); e != nil {
		return e
	}
	rows, e := h.all(h.template["labels"], 1)
	if err := check("unreadable-row", e == nil && len(rows) == 1 && !rows[0].Readable && rows[0].Session == s.Session && rows[0].Generation == 0 && rows[0].State == "unknown" && rows[0].Doorbell.State == "off", "unreadable row must retain indexed handle and labels with null lifecycle"); err != nil {
		return err
	}
	v, e := h.status(s)
	if err := check("unreadable-row", e == nil && !v.Readable, "direct status must use same projection"); err != nil {
		return err
	}
	_, e = h.call("resume", h.resume(s))
	return h.expectCode("unreadable-row", "session_resume_unreadable", e)
}
func indexCase(kind string) func(*harness) error {
	return func(h *harness) error {
		p := "unreadable-index"
		if kind == "missing-labels" {
			p = "filtered-index"
		}
		s, e := h.start(h.launch())
		if e != nil {
			return e
		}
		if rows, e := h.all(h.template["labels"], 1); e != nil || len(rows) != 1 {
			return setup(errors.New("positive page control"))
		}
		if e = h.corrupt(s, kind); e != nil {
			return e
		}
		_, e = h.all(h.template["labels"], 1)
		var typed *client.Error
		if !errors.As(e, &typed) {
			return check(p, false, "index refusal must be typed")
		}
		return check(p, typed.Code == "session_host_unavailable" && typed.Details["reason"] == "index_unreadable", "whole page must refuse specifically index_unreadable")
	}
}
func unreadableOutcomeCase(h *harness) error {
	r := h.launch()
	s, e := h.start(r)
	if e != nil {
		return e
	}
	v, e := h.call("status", map[string]any{"operation_id": r["operation_id"]})
	if e != nil || v.Outcome.Outcome != "started" {
		return setup(errors.New("started positive control"))
	}
	if e = h.corrupt(s, "unreadable"); e != nil {
		return e
	}
	v, e = h.call("status", map[string]any{"operation_id": r["operation_id"]})
	return check("started-unreadable", e == nil && v.Outcome.Outcome == "unknown" && v.Outcome.Session == nil, "unreadable outcome must never claim started")
}
func crashCase(h *harness) error {
	r := h.launch()
	r["admission_key"] = "werkstatt:crash"
	if e := h.fault("crash-after-start"); e != nil {
		return e
	}
	_, _ = h.call("launch", r)
	if e := h.fault(""); e != nil {
		return e
	}
	v, e := h.call("status", map[string]any{"operation_id": r["operation_id"]})
	if err := check("unknown-start", e == nil && v.Outcome.Outcome == "unknown", "crash window must retain unknown start evidence"); err != nil {
		return err
	}
	next := h.launch()
	next["admission_key"] = r["admission_key"]
	_, e = h.call("launch", next)
	if e = h.expectCode("unknown-start", "werkstatt_already_up", e); e != nil {
		return e
	}
	o, e := h.observe()
	return check("unknown-start", e == nil && o.Starts == 1, "uncertain start holds admission and forbids duplicates")
}
func armedLaunch(h *harness) map[string]any {
	r := h.launch()
	if r["doorbell"] == nil {
		r["doorbell"] = map[string]any{"identity": "conformance", "heartbeat": "30s", "max_wakes_per_hour": 1}
	}
	h.template["doorbell"] = r["doorbell"]
	return r
}
func sameArmedGeneration(a, b *client.Session) bool {
	return a.Doorbell.Generation != nil && b.Doorbell.Generation != nil && *a.Doorbell.Generation == *b.Doorbell.Generation
}

// An armed control supplies a non-null generation; gate closure and completed
// retirement retain it while preventing wakes, even while the provider lives.
func doorbellGateCase(h *harness) error {
	const p = "doorbell-gate-closure"
	s, e := h.start(armedLaunch(h))
	if e != nil {
		return e
	}
	if e = check(p, s.Doorbell.State == "armed" && s.Doorbell.Generation != nil && !s.Doorbell.Uncertain && s.Doorbell.Reason == nil, "armed positive control"); e != nil {
		return e
	}
	if e = h.activity(s, "graceful-hang"); e != nil {
		return e
	}
	v, e := h.stop(s, "graceful", "none")
	if e = check(p, e == nil && v.Session.State == "draining" && v.Session.Doorbell.State == "off" && sameArmedGeneration(s, v.Session) && !v.Session.Doorbell.Uncertain && v.Session.Doorbell.Reason == nil, "closed gate must project off with armed generation"); e != nil {
		return e
	}
	o, e := h.observe()
	if e = check(p, e == nil && o.Alive[s.Session] && !o.Gates[s.Session] && o.Retirement[s.Session] == 1, "gate closure must retain live draining provider"); e != nil {
		return e
	}
	v, e = h.stop(s, "force", "none")
	return check(p, e == nil && v.Session.State == "down" && v.Session.Doorbell.State == "off" && sameArmedGeneration(s, v.Session), "completed retirement retains armed generation")
}
func doorbellCase(h *harness) error {
	s, e := h.start(armedLaunch(h))
	if e != nil {
		return e
	}
	if err := check("doorbell-uncertain", s.Doorbell.State == "armed" && s.Doorbell.Generation != nil && !s.Doorbell.Uncertain, "armed positive control"); err != nil {
		return err
	}
	if e = h.activity(s, "uncertain"); e != nil {
		return e
	}
	v, e := h.stop(s, "force", "none")
	if err := check("doorbell-uncertain", e == nil && v.Session.Doorbell.State == "off" && v.Session.Doorbell.Uncertain && sameArmedGeneration(s, v.Session), "retirement must preserve uncertainty and armed generation"); err != nil {
		return err
	}
	v, e = h.call("resume", h.resume(s))
	return check("doorbell-uncertain", e == nil && v.Session.Doorbell.Uncertain && v.Session.Doorbell.State == "off" && sameArmedGeneration(s, v.Session) && v.Session.Doorbell.Reason != nil && *v.Session.Doorbell.Reason == "previous_generation_uncertain", "rebind must retain uncertainty and last armed generation")
}
func gracefulCase(h *harness) error {
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	if e = h.activity(s, "graceful-hang"); e != nil {
		return e
	}
	v, e := h.stop(s, "graceful", "none")
	if err := check("graceful-retirement", e == nil && v.Session.State == "draining", "alive graceful timeout must remain draining"); err != nil {
		return err
	}
	o, e := h.observe()
	if e != nil {
		return setup(e)
	}
	if err := check("graceful-retirement", o.Alive[s.Session] && !o.Gates[s.Session], "draining must fence turns, keep provider"); err != nil {
		return err
	}
	if e = h.activity(s, "provider-gone"); e != nil {
		return e
	}
	v.Session, e = h.status(s)
	if err := check("graceful-retirement", e == nil && v.Session.State == "down", "status must finish persisted retirement once provider gone"); err != nil {
		return err
	}
	o, e = h.observe()
	return check("graceful-retirement", e == nil && !o.Alive[s.Session] && !o.Gates[s.Session] && o.Retirement[s.Session] == 4, "all retirement steps finished")
}
func credentialCase(h *harness) error {
	if _, e := h.start(h.launch()); e != nil {
		return e
	}
	r := h.launch()
	r["child_env"] = map[string]string{"SAFE_OPTION": "sk-conformance-sentinel"}
	w, e := h.exec(h.plugin, "launch", jsonBytes(r), false)
	if e != nil {
		return setup(e)
	}
	_, err := client.DecodeResponse("launch", jsonBytes(r), w.Body, w.Exit, h.id)
	if e = h.expectCode("credential-sentinel", "secret_policy_violation", err); e != nil {
		return e
	}
	o, e := h.observe()
	if e != nil {
		return setup(e)
	}
	return check("credential-sentinel", !w.Leak && !bytes.Contains(w.Body, []byte("sk-conformance-sentinel")) && !bytes.Contains(w.Stderr, []byte("sk-conformance-sentinel")) && !bytes.Contains(o.Records, []byte("sk-conformance-sentinel")), "sentinel reached stdout, stderr, response or records")
}
func attachCase(plain bool) func(*harness) error {
	return func(h *harness) error {
		p := "attach-framing"
		s, e := h.start(h.launch())
		if e != nil {
			return e
		}
		code, exit, e := h.attach(s, s.Generation)
		if e != nil || code != "" || exit != 0 {
			return setup(errors.New("attach positive control"))
		}
		if plain {
			p = "attach-plain-exit"
			if e = h.fault("plain-attach-exit"); e != nil {
				return e
			}
			code, exit, e = h.attach(s, s.Generation)
			return check(p, e == nil && code == "" && exit == 7, "plain exit must have no typed refusal")
		}
		code, exit, e = h.attach(s, s.Generation+1)
		if err := check(p, e == nil && code == "session_generation_stale" && exit == 1, "fd4 refusal must retain typed registry classification"); err != nil {
			return err
		}
		w, e := h.exec(h.plugin, "launch-exact", []byte("{}"), true)
		if e != nil {
			return setup(e)
		}
		exactCode := ""
		framing := hosted.DrainStatusRecords(bytes.NewReader(w.Frames), hosted.NormalizeStatusRecord, func(r hosted.StatusRefusal) { exactCode = r.Code })
		return check(p, framing == nil && w.Exit == 6 && exactCode == "session_host_capability_missing", "unadvertised launch-exact must refuse on fd4")
	}
}
func admissionCase(pair string) func(*harness) error {
	return func(h *harness) error {
		p := "admission-" + pair
		seed, e := h.start(h.launch())
		if e != nil {
			return e
		}
		if _, e = h.stop(seed, "force", "none"); e != nil {
			return setup(e)
		}
		ops := strings.Split(pair, "-")
		requests := []map[string]any{h.launch(), h.launch()}
		for i, op := range ops {
			if op == "resume" {
				requests[i] = h.resume(seed)
			}
			requests[i]["admission_key"] = "werkstatt:concurrent"
		}
		var wg sync.WaitGroup
		results := make([]client.Response, 2)
		errs := make([]error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) { defer wg.Done(); results[i], errs[i] = h.call(ops[i], requests[i]) }(i)
		}
		wg.Wait()
		success := 0
		refused := 0
		for i, e := range errs {
			if e == nil && results[i].Session != nil {
				success++
			} else if client.IsCode(e, "werkstatt_already_up") {
				refused++
			} else {
				return check(p, false, fmt.Sprintf("unexpected concurrent outcome: %v", e))
			}
		}
		if err := check(p, success == 1 && refused == 1, "exactly one concurrent admission must succeed"); err != nil {
			return err
		}
		rows, e := h.all(h.template["labels"], 1)
		if e != nil {
			return setup(e)
		}
		active := 0
		for _, s := range rows {
			if s.State != "down" {
				active++
			}
		}
		return check(p, active == 1, "only one active generation may remain")
	}
}
func lostCase(fault string) func(*harness) error {
	return func(h *harness) error {
		p := "lost-" + fault
		r := h.launch()
		if e := h.fault(fault); e != nil {
			return e
		}
		_, _ = h.call("launch", r)
		if e := h.fault(""); e != nil {
			return e
		}
		v, e := h.call("status", map[string]any{"operation_id": r["operation_id"]})
		want := map[string]string{"before-admission": "not_found", "after-admission": "started", "failed-start": "failed"}[fault]
		if err := check(p, e == nil && v.Outcome.Outcome == want, "lost reply reconciled to wrong ledger outcome"); err != nil {
			return err
		}
		o, e := h.observe()
		if e != nil {
			return setup(e)
		}
		starts := 0
		if fault == "after-admission" {
			starts = 1
		}
		if err := check(p, o.Starts == starts, "wrong provider count after lost reply"); err != nil {
			return err
		}
		if fault == "before-admission" {
			_, e = h.start(r)
			return e
		}
		if fault == "failed-start" {
			return check(p, v.Outcome.Code == "policy_refused", "failed start must retain typed failure")
		}
		again, e := h.call("launch", r)
		return check(p, e == nil && again.Session.Session == v.Outcome.Session.Session, "retry recovers original result")
	}
}
func poisonCase(fault string) func(*harness) error {
	return func(h *harness) error {
		p := "poison-" + fault
		r := h.launch()
		if _, e := h.start(r); e != nil {
			return e
		}
		id := map[string]any{"operation_id": r["operation_id"]}
		if _, e := h.call("status", id); e != nil {
			return check(p, false, "ordinary status wire is invalid")
		}
		if e := h.fault(fault); e != nil {
			return e
		}
		data := jsonBytes(id)
		w, e := h.exec(h.plugin, "status", data, false)
		if e != nil {
			return setup(e)
		}
		if !faultApplied(fault, w) {
			return setup(errors.New("fault plant was not applied: " + fault))
		}
		_, err := client.DecodeResponse("status", data, w.Body, w.Exit, h.id)
		if e = h.expectCode(p, "session_host_protocol_error", err); e != nil {
			return e
		}
		var typed *client.Error
		if !errors.As(err, &typed) {
			return check(p, false, "missing typed protocol refusal")
		}
		return check(p, typed.ExitCode() == 6 && !strings.Contains(typed.Diagnostic(), "sentinel") && !strings.Contains(typed.Diagnostic(), "arbitrary") && !strings.Contains(typed.Diagnostic(), "\n"), "poison must exit 6 without retaining hostile details")
	}
}

// Confirm the intended wire violation actually reached the shared validator.
// A missing plant or unrelated setup failure never establishes poison coverage.
func faultApplied(name string, w wire) bool {
	if name == "malformed" {
		return w.Exit == 0 && bytes.Equal(w.Body, []byte("{"))
	}
	if name == "over-bound" {
		return w.Exit == 0 && len(w.Body) > client.Limit
	}
	var v struct {
		Protocol string
		OK       bool
		Details  map[string]string
		Outcome  string
		Session  *client.Session
	}
	if json.Unmarshal(w.Body, &v) != nil {
		return false
	}
	switch name {
	case "protocol":
		return w.Exit == 0 && v.Protocol == "other/1"
	case "exit-ok":
		return w.Exit == 1 && v.OK && v.Protocol == client.Protocol
	case "detail-text":
		return w.Exit != 0 && v.Details["reason"] == "arbitrary text"
	case "detail-control":
		return w.Exit != 0 && v.Details["reason"] == "\n"
	case "detail-secret":
		return w.Exit != 0 && v.Details["reason"] == "sk-conformance-sentinel"
	case "started-unreadable":
		return w.Exit == 0 && v.Outcome == "started" && v.Session != nil && !v.Session.Readable
	}
	return false
}

// The deliberately invalid started packet must poison the whole response;
// returning a partial unreadable session would turn uncertainty into success.
func unreadablePoisonCase(h *harness) error {
	r := h.launch()
	s, e := h.start(r)
	if e != nil {
		return e
	}
	if e = h.corrupt(s, "unreadable"); e != nil {
		return e
	}
	id := map[string]any{"operation_id": r["operation_id"]}
	ordinary, e := h.call("status", id)
	if err := check("started-unreadable-poison", e == nil && ordinary.Outcome.Outcome == "unknown", "ordinary unreadable outcome wire is invalid"); err != nil {
		return err
	}
	if e = h.fault("started-unreadable"); e != nil {
		return e
	}
	data := jsonBytes(id)
	w, e := h.exec(h.plugin, "status", data, false)
	if e != nil {
		return setup(e)
	}
	if !faultApplied("started-unreadable", w) {
		return setup(errors.New("unreadable-started plant was not applied"))
	}
	v, e := client.DecodeResponse("status", data, w.Body, w.Exit, h.id)
	if err := h.expectCode("started-unreadable-poison", "session_host_protocol_error", e); err != nil {
		return err
	}
	return check("started-unreadable-poison", v.Outcome == nil && v.Session == nil, "poisoned result must retain no fields")
}

func retainedUntil(stamp string) string {
	t, _ := time.Parse(time.RFC3339, stamp)
	return t.Add(fakemanager.Retention).Format(time.RFC3339)
}

// The subprocess exits at the reservation boundary, bypassing normal response
// handling and its final save. This proves durable exclusion before start.
func reservationCase(h *harness) error {
	r := h.launch()
	r["admission_key"] = "werkstatt:reservation"
	if e := h.fault("crash-after-reservation"); e != nil {
		return e
	}
	_, _ = h.call("launch", r)
	if e := h.fault(""); e != nil {
		return e
	}
	v, e := h.call("status", map[string]any{"operation_id": r["operation_id"]})
	if err := check("reserved-crash", e == nil && v.Outcome.Outcome == "admitted", "reservation lost across process crash"); err != nil {
		return err
	}
	next := h.launch()
	next["admission_key"] = r["admission_key"]
	_, e = h.call("launch", next)
	if e = h.expectCode("reserved-crash", "werkstatt_already_up", e); e != nil {
		return e
	}
	o, e := h.observe()
	return check("reserved-crash", e == nil && o.Starts == 0, "provider must not start before durable reservation")
}

// Re-stopping an old down handle must not release a newer handle's key. The
// positive control also proves key reuse increments the admission generation.
func recycleCase(h *harness) error {
	r := h.launch()
	r["admission_key"] = "werkstatt:recycle"
	a, e := h.start(r)
	if e != nil {
		return e
	}
	if _, e = h.stop(a, "force", "none"); e != nil {
		return setup(e)
	}
	r["operation_id"] = operationID()
	b, e := h.start(r)
	if e != nil {
		return e
	}
	if err := check("admission-recycle", a.Session != b.Session && b.Generation > a.Generation, "key reuse must create a new monotonic generation"); err != nil {
		return err
	}
	if _, e = h.stop(a, "force", "none"); e != nil {
		return setup(e)
	}
	r["operation_id"] = operationID()
	_, e = h.call("launch", r)
	return h.expectCode("admission-recycle", "werkstatt_already_up", e)
}

type notApplicable struct{ Reason string }

func (n *notApplicable) Error() string { return "not_applicable: " + n.Reason }

// A busy event supplies T; clock-only polling/listing and unknown evidence retain
// T, while a subsequent idle event supplies a new timestamp.
func activityTimestampCase(h *harness) error {
	const p = "activity-timestamp"
	s, e := h.start(h.launch())
	if e != nil {
		return e
	}
	now := time.Now().UTC().Truncate(time.Second)
	clock := func(v time.Time) error {
		_, e := h.control(fakemanager.FixtureRequest{Operation: "set_clock", Now: v.Format(time.RFC3339)})
		return setup(e)
	}
	if e = clock(now); e != nil {
		return e
	}
	if e = h.activity(s, "busy"); e != nil {
		return e
	}
	v, e := h.status(s)
	if e != nil {
		return setup(e)
	}
	if e = check(p, v.State == "busy" && v.LastActivityAt != nil && *v.LastActivityAt == now.Format(time.RFC3339), "busy event must supply its timestamp"); e != nil {
		return e
	}
	if e = clock(now.Add(time.Hour)); e != nil {
		return e
	}
	for i := 0; i < 2; i++ {
		v, e = h.status(s)
		if e = check(p, e == nil && v.LastActivityAt != nil && *v.LastActivityAt == now.Format(time.RFC3339), "read manufactured activity"); e != nil {
			return e
		}
	}
	rows, e := h.all(h.template["labels"], 1)
	if e = check(p, e == nil && len(rows) == 1 && rows[0].LastActivityAt != nil && *rows[0].LastActivityAt == now.Format(time.RFC3339), "list manufactured activity"); e != nil {
		return e
	}
	if e = h.activity(s, "unknown"); e != nil {
		return e
	}
	v, e = h.status(s)
	if e = check(p, e == nil && v.State == "unknown" && v.LastActivityAt != nil && *v.LastActivityAt == now.Format(time.RFC3339), "unknown evidence erased last activity"); e != nil {
		return e
	}
	if e = h.activity(s, "idle"); e != nil {
		return e
	}
	v, e = h.status(s)
	return check(p, e == nil && v.State == "idle" && v.LastActivityAt != nil && *v.LastActivityAt == now.Add(time.Hour).Format(time.RFC3339), "new activity event must advance timestamp")
}
