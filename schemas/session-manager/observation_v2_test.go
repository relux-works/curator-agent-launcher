package managerschema

import (
	"encoding/json"
	"strings"
	"testing"
)

type fixture struct {
	Name     string         `json:"name"`
	Valid    bool           `json:"valid"`
	Document map[string]any `json:"document"`
}

func fixtures(t *testing.T, schema string) []fixture {
	t.Helper()
	b, err := Documents.ReadFile("vectors/" + schema + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []fixture
	if err := json.Unmarshal(b, &rows); err != nil || len(rows) == 0 {
		t.Fatal("missing vectors", err)
	}
	return rows
}

func clone(t *testing.T, value any) map[string]any {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func verdict(t *testing.T, schema string, value any, valid bool) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{schema + ".schema.json", catalogIdentity(t, schema)} {
		err := Validate(identity, b)
		if (err == nil) != valid {
			t.Fatalf("%s valid=%t: %v", identity, valid, err)
		}
		// A refusal is either a schema violation or the strict reader's own
		// refusal of the bytes (for example a non-integer number such as 1.5,
		// which is never admitted as an integer); any other error class is a
		// harness or validator fault.
		if err != nil && !strings.HasPrefix(err.Error(), "schema violation at $") && err.Error() != "invalid protocol JSON" {
			t.Fatalf("unexpected refusal class through %s: %v", identity, err)
		}
	}
}

func catalogIdentity(t *testing.T, schema string) string {
	t.Helper()
	for id, file := range catalog {
		if file == schema+".schema.json" {
			return id
		}
	}
	t.Fatalf("no published identity for %s", schema)
	return ""
}

func hasExtension(document map[string]any) bool {
	_, process := document["process"]
	_, termination := document["termination"]
	return process || termination
}

func sampleProcess() map[string]any {
	return map[string]any{
		"pid": 4242, "uid": 1001, "boot_id": "6f1c0e2a-8b44-4d1e-9a51-2f0c3d7e9b10",
		"start_time": map[string]any{"kind": "linux-stat-starttime", "ticks": "123456789", "hz": 100},
	}
}

func readableRecord(t *testing.T, state string) map[string]any {
	t.Helper()
	row := clone(t, fixtures(t, "session-record")[0].Document)
	row["state"] = state
	return row
}

func observation(t *testing.T, version string, session any, processObservation bool) map[string]any {
	t.Helper()
	doc := clone(t, fixtures(t, "session-observation")[0].Document)
	doc["session"] = session
	if version == "v2" {
		doc["schema"] = "curator-run-session-observation-v2"
		doc["capabilities"] = map[string]any{"process_observation": processObservation}
	}
	return doc
}

// The published identities and titles are the d5 revisions: session-record,
// the enveloped session (in lockstep) and outcome keep their URNs, and
// observation v2 is a new closed schema registered by $id beside the
// unchanged v1 identity.
func TestObservationV2Identities(t *testing.T) {
	for file, want := range map[string][2]string{
		"session.schema.json":                {"urn:relux:curator-run:session", "session 1.4.0"},
		"session-record.schema.json":         {"urn:relux:curator-run:session-record", "session-record 1.4.0"},
		"outcome.schema.json":                {"urn:relux:curator-run:outcome", "outcome 1.1.0"},
		"session-observation.schema.json":    {"urn:relux:curator-run:session-observation", "curator-run-session-observation-v1"},
		"session-observation-v2.schema.json": {"urn:relux:curator-run:session-observation-v2", "curator-run-session-observation-v2"},
	} {
		b, err := Documents.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var meta struct {
			ID    string `json:"$id"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(b, &meta); err != nil {
			t.Fatal(err)
		}
		if meta.ID != want[0] || meta.Title != want[1] || catalog[want[0]] != file {
			t.Fatalf("%s identity=%q title=%q", file, meta.ID, meta.Title)
		}
	}
}

// Backward compatibility: every session-record vector that carries neither
// 1.3.0 member (the 1.2.0 corpus and the absent-member controls) keeps its
// listed verdict, and every such valid record is also a valid session inside
// both observation v1 and an incapable observation v2. This covers only the
// published corpus, not every conceivable 1.2.0 document.
func TestSessionRecordLegacyCorpus(t *testing.T) {
	legacy := 0
	for _, v := range fixtures(t, "session-record") {
		if v.Document == nil || hasExtension(v.Document) {
			continue
		}
		legacy++
		t.Run(v.Name, func(t *testing.T) {
			verdict(t, "session-record", v.Document, v.Valid)
			if v.Valid {
				verdict(t, "session-observation", observation(t, "v1", v.Document, false), true)
				verdict(t, "session-observation-v2", observation(t, "v2", v.Document, false), true)
				verdict(t, "session-observation-v2", observation(t, "v2", v.Document, true), true)
			}
		})
	}
	if legacy < 44 {
		t.Fatalf("legacy corpus shrank to %d vectors", legacy)
	}
}

// process is admitted only on readable starting/idle/busy/draining records and
// termination only on readable down records; neither is admitted on the
// unreadable variant. Each refusal has the permitted placement as its control.
func TestExtensionPlacement(t *testing.T) {
	termination := map[string]any{"scope": "linux-cgroup", "witness": "populated-0"}
	for _, state := range []string{"starting", "idle", "busy", "draining", "down", "unknown"} {
		t.Run(state, func(t *testing.T) {
			row := readableRecord(t, state)
			verdict(t, "session-record", row, true)
			row["process"] = sampleProcess()
			verdict(t, "session-record", row, state != "down" && state != "unknown")
			row = readableRecord(t, state)
			row["termination"] = termination
			verdict(t, "session-record", row, state == "down")
		})
	}
	unreadable := fixtures(t, "session-record")[1].Document
	verdict(t, "session-record", unreadable, true)
	for key, value := range map[string]any{"process": sampleProcess(), "termination": termination} {
		row := clone(t, unreadable)
		row[key] = value
		verdict(t, "session-record", row, false)
	}
}

// Each process field enforces exactly its d5 range or grammar, with the
// boundary values as accepted controls. Decimal strings admit "0" and up to
// 20 digits without a leading zero; the two start_time variants never mix.
func TestProcessFieldBounds(t *testing.T) {
	type edit struct {
		name  string
		apply func(map[string]any)
		valid bool
	}
	set := func(key string, value any) func(map[string]any) {
		return func(p map[string]any) { p[key] = value }
	}
	start := func(variant map[string]any) func(map[string]any) {
		return func(p map[string]any) { p["start_time"] = variant }
	}
	linux := func(ticks any, hz any) map[string]any {
		return map[string]any{"kind": "linux-stat-starttime", "ticks": ticks, "hz": hz}
	}
	darwin := func(sec any, usec any) map[string]any {
		return map[string]any{"kind": "darwin-pbi-start", "sec": sec, "usec": usec}
	}
	cases := []edit{
		{"control", func(map[string]any) {}, true},
		{"pid-1", set("pid", 1), true},
		{"pid-max", set("pid", 2147483647), true},
		{"pid-0", set("pid", 0), false},
		{"pid-over", set("pid", 2147483648), false},
		{"uid-0", set("uid", 0), true},
		{"uid-max", set("uid", 4294967294), true},
		{"uid-minus-one", set("uid", 4294967295), false},
		{"uid-negative", set("uid", -1), false},
		{"boot-upper", set("boot_id", "6F1C0E2A-8B44-4D1E-9A51-2F0C3D7E9B10"), false},
		{"boot-short", set("boot_id", "6f1c0e2a-8b44-4d1e-9a51-2f0c3d7e9b1"), false},
		{"boot-any-version", set("boot_id", "00000000-0000-0000-0000-000000000000"), true},
	}
	for _, field := range []string{"ticks", "sec"} {
		for value, valid := range map[string]bool{
			"0": true, "1": true, strings.Repeat("9", 20): true,
			"00": false, "01": false, "1" + strings.Repeat("0", 20): false, "": false, "+1": false, "1 ": false,
		} {
			variant := linux(value, 100)
			if field == "sec" {
				variant = darwin(value, 0)
			}
			cases = append(cases, edit{field + "-" + value, start(variant), valid})
		}
	}
	cases = append(cases,
		edit{"hz-1", start(linux("1", 1)), true},
		edit{"hz-max", start(linux("1", 100000)), true},
		edit{"hz-0", start(linux("1", 0)), false},
		edit{"hz-over", start(linux("1", 100001)), false},
		edit{"usec-0", start(darwin("1", 0)), true},
		edit{"usec-max", start(darwin("1", 999999)), true},
		edit{"usec-over", start(darwin("1", 1000000)), false},
		edit{"ticks-number", start(linux(1, 100)), false},
		edit{"linux-kind-darwin-fields", start(map[string]any{"kind": "linux-stat-starttime", "sec": "1", "usec": 0}), false},
		edit{"darwin-kind-linux-fields", start(map[string]any{"kind": "darwin-pbi-start", "ticks": "1", "hz": 100}), false},
		edit{"unknown-kind", start(map[string]any{"kind": "wall-clock", "ticks": "1", "hz": 100}), false},
		edit{"extra-member", set("started_at", "2026-01-01T00:00:00Z"), false},
	)
	for _, key := range []string{"pid", "uid", "boot_id", "start_time"} {
		cases = append(cases, edit{"missing-" + key, func(p map[string]any) { delete(p, key) }, false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := sampleProcess()
			tc.apply(p)
			row := readableRecord(t, "idle")
			row["process"] = p
			verdict(t, "session-record", row, tc.valid)
		})
	}
}

// d5 §5 projection pairs over the published corpus: a valid 1.3.0 record with
// an extension is refused inside observation v1 and accepted after removing
// exactly process/termination; observation v2 accepts it only when
// process_observation is true. Records with a malformed extension stay invalid
// in every wrapper, so a malformed member is never made valid by the schema.
func TestObservationProjectionPairs(t *testing.T) {
	positive, negative := 0, 0
	for _, v := range fixtures(t, "session-record") {
		if v.Document == nil || !hasExtension(v.Document) {
			continue
		}
		t.Run(v.Name, func(t *testing.T) {
			verdict(t, "session-record", v.Document, v.Valid)
			verdict(t, "session-observation", observation(t, "v1", v.Document, false), false)
			verdict(t, "session-observation-v2", observation(t, "v2", v.Document, true), v.Valid)
			verdict(t, "session-observation-v2", observation(t, "v2", v.Document, false), false)
			if !v.Valid {
				negative++
				return
			}
			positive++
			projected := clone(t, v.Document)
			delete(projected, "process")
			delete(projected, "termination")
			verdict(t, "session-observation", observation(t, "v1", projected, false), true)
			verdict(t, "session-record", projected, true)
		})
	}
	if positive == 0 || negative == 0 {
		t.Fatalf("projection pairs need both verdicts: positive=%d negative=%d", positive, negative)
	}
}

// Observation v2 is v1 plus the closed capabilities member: every v1 vector
// rewritten to v2 with an incapable capability keeps its verdict, and the
// rewrite without capabilities is always refused.
func TestObservationV2AlignsWithV1(t *testing.T) {
	for _, v := range fixtures(t, "session-observation") {
		if v.Document == nil {
			continue
		}
		t.Run(v.Name, func(t *testing.T) {
			doc := clone(t, v.Document)
			if doc["schema"] == "curator-run-session-observation-v1" {
				doc["schema"] = "curator-run-session-observation-v2"
			}
			if _, present := doc["capabilities"]; present {
				// v1 refuses this member; in v2 it is the same closed member.
				t.Skip("capabilities vector is v1-only")
			}
			bare := clone(t, doc)
			verdict(t, "session-observation-v2", bare, false)
			doc["capabilities"] = map[string]any{"process_observation": false}
			want := v.Valid
			if v.Name == "v1-v2-schema-name" {
				want = true
			}
			verdict(t, "session-observation-v2", doc, want)
		})
	}
}

// no_start is admitted with exactly "proved" on failed and cancelled outcomes
// only; the other five outcomes refuse it, and the controls stay valid.
func TestOutcomeNoStart(t *testing.T) {
	record := fixtures(t, "session-record")[0].Document
	for _, tc := range []struct {
		outcome string
		extra   map[string]any
	}{
		{"not_found", nil}, {"prepared", nil}, {"admitted", nil},
		{"started", map[string]any{"session": record}},
		{"failed", map[string]any{"code": "policy_refused"}},
		{"cancelled", nil}, {"unknown", nil},
	} {
		t.Run(tc.outcome, func(t *testing.T) {
			doc := map[string]any{"protocol": "curator-session-manager/1", "ok": true, "outcome": tc.outcome}
			for k, v := range tc.extra {
				doc[k] = v
			}
			verdict(t, "outcome", doc, true)
			allowed := tc.outcome == "failed" || tc.outcome == "cancelled"
			for _, value := range []any{"proved", true, nil, "", "PROVED"} {
				with := clone(t, doc)
				with["no_start"] = value
				verdict(t, "outcome", with, allowed && value == "proved")
			}
		})
	}
}

// The published reuse vectors are well-formed records that differ from the
// base incarnation in exactly one tuple member (start_time within one boot,
// boot_id across boots). This documents the A2/Core byte comparison; the
// schema itself cannot detect reuse.
func TestIncarnationReuseVectors(t *testing.T) {
	byName := map[string]map[string]any{}
	for _, v := range fixtures(t, "session-record") {
		byName[v.Name] = v.Document
	}
	base := byName["process-linux-stat-starttime"]["process"].(map[string]any)
	for name, member := range map[string]string{
		"incarnation-pid-reuse-within-boot":  "start_time",
		"incarnation-pid-reuse-across-boots": "boot_id",
	} {
		t.Run(name, func(t *testing.T) {
			doc := byName[name]
			verdict(t, "session-record", doc, true)
			other := doc["process"].(map[string]any)
			for _, key := range []string{"pid", "uid", "boot_id", "start_time"} {
				a, _ := json.Marshal(base[key])
				b, _ := json.Marshal(other[key])
				if (string(a) == string(b)) == (key == member) {
					t.Fatalf("%s: member %s equality is wrong", name, key)
				}
			}
		})
	}
}

func schemaDocument(t *testing.T, file string) map[string]any {
	t.Helper()
	b, err := Documents.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func encodedForm(t *testing.T, value any) string {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The enveloped session and envelope-free session-record 1.4.0 definitions are
// duplicated (the validator resolves only whole-document references): both
// variants must stay identical apart from the protocol/ok envelope members,
// including the process/termination shapes and the state placement rules.
// Behaviourally, every envelope-free record vector keeps its verdict when the
// envelope is added. Vectors that already carry an envelope member are skipped.
func TestEnvelopedRecordAgreement(t *testing.T) {
	session := schemaDocument(t, "session.schema.json")["oneOf"].([]any)
	record := schemaDocument(t, "session-record.schema.json")["oneOf"].([]any)
	if len(session) != 2 || len(record) != 2 {
		t.Fatal("record variants changed")
	}
	for i := range record {
		a := clone(t, session[i])
		b := clone(t, record[i])
		for _, key := range []string{"protocol", "ok"} {
			delete(a["properties"].(map[string]any), key)
		}
		required := []any{}
		for _, key := range a["required"].([]any) {
			if key != "protocol" && key != "ok" {
				required = append(required, key)
			}
		}
		a["required"] = required
		if encodedForm(t, a) != encodedForm(t, b) {
			t.Fatalf("variant %d differs between session and session-record", i)
		}
	}
	readable := session[0].(map[string]any)["properties"].(map[string]any)
	if _, ok := readable["process"]; !ok {
		t.Fatal("enveloped session lacks process")
	}
	checked := 0
	for _, v := range fixtures(t, "session-record") {
		_, protocol := v.Document["protocol"]
		_, ok := v.Document["ok"]
		if protocol || ok {
			continue
		}
		checked++
		t.Run(v.Name, func(t *testing.T) {
			doc := clone(t, v.Document)
			doc["protocol"], doc["ok"] = "curator-session-manager/1", true
			verdict(t, "session", doc, v.Valid)
		})
	}
	if checked < 150 {
		t.Fatalf("agreement covered only %d vectors", checked)
	}
}

// Backward compatibility of the enveloped form: every session vector without a
// 1.3.0 member (the 1.2.0 corpus and the legacy control) keeps its verdict.
func TestSessionLegacyCorpus(t *testing.T) {
	legacy := 0
	for _, v := range fixtures(t, "session") {
		if v.Document == nil || hasExtension(v.Document) {
			continue
		}
		legacy++
		t.Run(v.Name, func(t *testing.T) {
			verdict(t, "session", v.Document, v.Valid)
		})
	}
	if legacy < 391 {
		t.Fatalf("legacy session corpus shrank to %d vectors", legacy)
	}
}

// werkstatt-status is launcher output that d5 does not extend: a manager
// session carrying process or termination is refused there, while the same
// record without those members (its projection) and every valid legacy
// session remain accepted rows.
func TestWerkstattStatusRefusesExtensions(t *testing.T) {
	b, err := Documents.ReadFile("vectors/werkstatt-status.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw []struct {
		Name     string `json:"name"`
		Document any    `json:"document"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	var template map[string]any
	for _, v := range raw {
		if v.Name == "stale-without-liveness" {
			template = v.Document.([]any)[0].(map[string]any)
		}
	}
	if template == nil {
		t.Fatal("missing werkstatt-status row template")
	}
	status := func(session map[string]any) []any {
		row := clone(t, template)
		row["manager_session"] = session
		return []any{row}
	}
	extended, legacy := 0, 0
	for _, v := range fixtures(t, "session") {
		if !v.Valid {
			continue
		}
		t.Run(v.Name, func(t *testing.T) {
			if !hasExtension(v.Document) {
				legacy++
				verdict(t, "werkstatt-status", status(v.Document), true)
				return
			}
			extended++
			verdict(t, "session", v.Document, true)
			verdict(t, "werkstatt-status", status(v.Document), false)
			projected := clone(t, v.Document)
			delete(projected, "process")
			delete(projected, "termination")
			verdict(t, "werkstatt-status", status(projected), true)
		})
	}
	if extended == 0 || legacy == 0 {
		t.Fatalf("werkstatt-status pairs need both kinds: extended=%d legacy=%d", extended, legacy)
	}
}
