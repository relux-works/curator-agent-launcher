package m0adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

type forbiddenControl struct {
	ID, Change, Obligation, Expected string
	Neighbors                        []string
	ReachableAfter                   string
}

// fixtureValue rejects duplicate names, including escaped aliases, before the
// typed fixture decoder can silently overwrite them. Arrays are documentary.
func fixtureValue(d *json.Decoder) (any, error) {
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		m := map[string]any{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("fixture_name")
			}
			if _, exists := m[name]; exists {
				return nil, fmt.Errorf("fixture_duplicate_member")
			}
			v, err := fixtureValue(d)
			if err != nil {
				return nil, err
			}
			m[name] = v
		}
		_, err = d.Token()
		return m, err
	case json.Delim('['):
		var a []any
		for d.More() {
			v, err := fixtureValue(d)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		_, err = d.Token()
		return a, err
	default:
		return tok, nil
	}
}

func fixtureDocument(b []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	v, err := fixtureValue(d)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("fixture_trailing")
	}
	return v, nil
}

// boundaryCases indexes every case section, not just valid_neighbors.
func boundaryCases(b []byte) (map[string]string, error) {
	v, err := fixtureDocument(b)
	if err != nil {
		return nil, err
	}
	ids := map[string]string{}
	var visit func(any) error
	visit = func(v any) error {
		switch x := v.(type) {
		case map[string]any:
			if id, ok := x["id"].(string); ok {
				if _, exists := ids[id]; exists {
					return fmt.Errorf("boundary_duplicate_id")
				}
				ids[id], _ = x["expected"].(string)
			}
			for _, child := range x {
				if err := visit(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range x {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(v); err != nil {
		return nil, err
	}
	if len(ids) != 59 {
		return nil, fmt.Errorf("boundary_count")
	}
	return ids, nil
}

func decodeForbidden(b []byte, neighbors map[string]string) ([]forbiddenControl, error) {
	v, err := fixtureDocument(b)
	if err != nil {
		return nil, err
	}
	rows, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("controls_type")
	}
	if len(rows) != 88 {
		return nil, fmt.Errorf("controls_count")
	}
	seen := map[string]bool{}
	var out []forbiddenControl
	masked := 0
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("control_type")
		}
		for k := range m {
			switch k {
			case "id", "change", "obligation", "expected", "neighbors", "reachable_after":
			default:
				return nil, fmt.Errorf("control_unknown_member")
			}
		}
		var c forbiddenControl
		for k, dest := range map[string]*string{"id": &c.ID, "change": &c.Change, "obligation": &c.Obligation, "expected": &c.Expected} {
			text, ok := m[k].(string)
			if !ok || text == "" {
				return nil, fmt.Errorf("control_required_member")
			}
			*dest = text
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("control_duplicate_id")
		}
		seen[c.ID] = true
		ns, ok := m["neighbors"].([]any)
		if !ok || len(ns) == 0 {
			return nil, fmt.Errorf("control_required_member")
		}
		hypothetical := false
		for _, n := range ns {
			id, ok := n.(string)
			if !ok {
				return nil, fmt.Errorf("control_neighbor")
			}
			expected, found := neighbors[id]
			if !found {
				return nil, fmt.Errorf("control_neighbor")
			}
			c.Neighbors = append(c.Neighbors, id)
			hypothetical = hypothetical || strings.Contains(expected, "Hypothetical after")
		}
		r, present := m["reachable_after"]
		if present != hypothetical {
			return nil, fmt.Errorf("control_reachable_after")
		}
		if present {
			c.ReachableAfter, ok = r.(string)
			if !ok || c.ReachableAfter != "run-claude pin recorded in spec §4" {
				return nil, fmt.Errorf("control_reachable_after")
			}
			masked++
		}
		out = append(out, c)
	}
	if masked != 7 {
		return nil, fmt.Errorf("controls_masked_count")
	}
	return out, nil
}

// TestForbiddenDecoder proves the valid 88-row file passes beside each exact closed-decoder refusal.
func TestForbiddenDecoder(t *testing.T) {
	base := readTestdata(t, "forbidden-controls.json")
	neighbors, err := boundaryCases(readTestdata(t, "boundary.witnesses.json"))
	if err != nil {
		t.Fatal(err)
	}
	positive := func(t *testing.T) {
		t.Helper()
		if rows, err := decodeForbidden(base, neighbors); err != nil || len(rows) != 88 {
			t.Fatalf("valid controls refused: %v", err)
		}
	}
	for _, c := range []struct{ name, old, new, code string }{
		{"unknown", `"id": "missing"`, `"unknown": 1, "id": "missing"`, "control_unknown_member"},
		{"case-alias", `"id": "missing"`, `"ID": "missing"`, "control_unknown_member"},
		{"duplicate-id", `"id": "extra"`, `"id": "missing"`, "control_duplicate_id"},
		{"duplicate-member", `"id": "missing"`, `"id": "missing", "\u0069d": "missing"`, "fixture_duplicate_member"},
		{"neighbor", `"matching-control"`, `"nonexistent-case"`, "control_neighbor"},
		{"null-neighbors", "\"neighbors\": [\n      \"matching-control\"\n    ]", `"neighbors": null`, "control_required_member"},
		{"empty-neighbors", "\"neighbors\": [\n      \"matching-control\"\n    ]", `"neighbors": []`, "control_required_member"},
		{"reachable-missing", ",\n    \"reachable_after\": \"run-claude pin recorded in spec §4\"", "", "control_reachable_after"},
		{"reachable-extra", `"id": "missing"`, `"reachable_after": "run-claude pin recorded in spec §4", "id": "missing"`, "control_reachable_after"},
		{"reachable-value", `"reachable_after": "run-claude pin recorded in spec §4"`, `"reachable_after": "later"`, "control_reachable_after"},
	} {
		t.Run(c.name, func(t *testing.T) {
			positive(t)
			bad := strings.Replace(string(base), c.old, c.new, 1)
			if bad == string(base) {
				t.Fatal("mutation not applied")
			}
			if _, err := decodeForbidden([]byte(bad), neighbors); err == nil || err.Error() != c.code {
				t.Fatalf("got %v, want %s", err, c.code)
			}
		})
	}
	for _, member := range []string{"id", "change", "obligation", "expected", "neighbors"} {
		t.Run("missing-"+member, func(t *testing.T) {
			positive(t)
			var rows []map[string]json.RawMessage
			if err := json.Unmarshal(base, &rows); err != nil {
				t.Fatal(err)
			}
			delete(rows[0], member)
			bad, err := json.Marshal(rows)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeForbidden(bad, neighbors); err == nil || err.Error() != "control_required_member" {
				t.Fatalf("got %v, want control_required_member", err)
			}
		})
	}
	for _, n := range []int{87, 89} {
		t.Run(fmt.Sprintf("count-%d", n), func(t *testing.T) {
			positive(t)
			var rows []json.RawMessage
			if err := json.Unmarshal(base, &rows); err != nil {
				t.Fatal(err)
			}
			if n == 87 {
				rows = rows[:87]
			} else {
				rows = append(rows, rows[0])
			}
			bad, err := json.Marshal(rows)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeForbidden(bad, neighbors); err == nil || err.Error() != "controls_count" {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("trailing", func(t *testing.T) {
		positive(t)
		if _, err := decodeForbidden(append(append([]byte(nil), base...), []byte("{}")...), neighbors); err == nil || err.Error() != "fixture_trailing" {
			t.Fatalf("got %v", err)
		}
	})
}

type controlCoverage struct {
	// Exactly one own named test for adapter controls; otherwise an owning
	// component and a one-line reason, never an implicit skip.
	test, external string
}

// forbiddenCoverage classifies all 88 controls exactly once.
var forbiddenCoverage = map[string]controlCoverage{
	"missing":                              {test: "TestAdapterControls/missing"},
	"extra":                                {test: "TestAdapterControls/extra"},
	"null":                                 {test: "TestAdapterControls/null"},
	"duplicate":                            {test: "TestAdapterControls/duplicate"},
	"escaped-duplicate":                    {test: "TestAdapterControls/escaped-duplicate"},
	"invalid-utf8":                         {test: "TestAdapterControls/invalid-utf8"},
	"bom":                                  {test: "TestAdapterControls/bom"},
	"surrogate":                            {test: "TestAdapterControls/surrogate"},
	"noninteger":                           {test: "TestAdapterControls/noninteger"},
	"negative-zero":                        {test: "TestAdapterControls/negative-zero"},
	"range":                                {test: "TestAdapterControls/range"},
	"depth":                                {test: "TestAdapterControls/depth"},
	"size":                                 {test: "TestAdapterControls/size"},
	"escape-size":                          {test: "TestAdapterControls/escape-size"},
	"identity":                             {external: "D checks worker identity against authenticated originals; worker shape is not authority."},
	"generation":                           {external: "D checks the current protected worker generation before handoff."},
	"association":                          {external: "D checks the protected association revision before handoff."},
	"pins":                                 {external: "D checks complete typed catalog and plan pins against originals."},
	"opaque":                               {external: "D owns typed admission; TestGrammarNegatives/opaque also rejects this malformed body."},
	"selectors":                            {test: "TestAdapterControls/selectors"},
	"injected-member":                      {test: "TestAdapterControls/injected-member"},
	"catalog":                              {external: "D owns catalog equality; printable strings alone grant nothing."},
	"home-prompt":                          {test: "TestAdapterControls/home-prompt"},
	"fd":                                   {external: "Manager admits only 0-3 before target code; adapter inventory is defense in depth."},
	"clock":                                {test: "TestAdapterControls/clock"},
	"second-start":                         {external: "D reconciles the original unknown start without a second physical start."},
	"unqualified":                          {external: "Broker protected qualification owns refusal 69 and no lease admission."},
	"config":                               {external: "Broker owns the protected configuration read; no public-copy fallback."},
	"result":                               {external: "D owns Result and read authority; output mode or bytes grant neither."},
	"qualification-measurement-order":      {external: "Broker Resolve may measure before absent or malformed qualified.json."},
	"unknown-publication":                  {external: "D retains publication debt after unobserved rename for old and absent output."},
	"late-stop":                            {external: "D keeps runner terminal 0 after rename separate from its own disposition."},
	"native-adapter":                       {external: "Manager and installation qualification must pin native code, not a script interpreter."},
	"fd3-left-open":                        {test: "TestAdapterControls/fd3-left-open"},
	"environment":                          {test: "TestAdapterControls/environment"},
	"d-path":                               {external: "D and profile owners must select the proposed vendor PATH narrowing."},
	"transitive-exec":                      {external: "OS R-EXEC qualification owns the full transitive invocation inventory."},
	"drop":                                 {external: "Manager must verify complete group and UID/GID drop before target code."},
	"before-c":                             {external: "Receiver qualification must put C before decoder, loader and constructor; a Go seam cannot prove it."},
	"pin-drift":                            {external: "D checks every plan pin, effect, epoch and authenticated original before handoff."},
	"result-overflow":                      {external: "D owns finite capture and retained original debt on overflow."},
	"agent-handle-reexec":                  {external: "Broker must fix executable-handle transfer and provide native re-exec traces."},
	"numeric-fd-false-positive":            {external: "Native evidence must distinguish kernel objects from reused descriptor numbers."},
	"path-kernel-conflation":               {external: "OS forbidden-exec evidence is separate from PATH conformance."},
	"worker-protected-read":                {external: "D alone resolves protected metadata; worker lookup is forbidden by architecture."},
	"missing-timing":                       {test: "TestAdapterControls/missing-timing"},
	"tampered-timing":                      {external: "D checks both deadline and slack equality and provenance against originals."},
	"clock-coordinate":                     {test: "TestAdapterControls/clock-coordinate"},
	"timing-reset-rounding":                {test: "TestAdapterControls/timing-reset-rounding"},
	"status-attribution":                   {external: "D needs installed original fate evidence; numbers, signals and lost observations imply no absence."},
	"status-collision":                     {external: "D and tuple qualification reject a runner status set containing 78."},
	"stdin-pipe-blocking-write":            {test: "TestPromptPipeNonblocking"},
	"stdin-pipe-partial-write":             {test: "TestPromptPipePartialWrite"},
	"stdin-pipe-capacity-raise":            {test: "TestPromptPipeNoCapacityRaise"},
	"stdin-pipe-write-end-inherited":       {test: "TestPromptPipeInheritedWriter"},
	"stdin-pipe-second-descriptor":         {test: "TestPromptPipeSecondDescriptor"},
	"stdin-fd0-read-write":                 {test: "TestPromptPipeReadOnly"},
	"stdin-feeder":                         {external: "Native qualification must show no feeder beside the directly exec-replaced adapter."},
	"stdin-pipe-capacity-below-length":     {test: "TestPromptPipeCapacity"},
	"stdin-pipe-getpipesz-failure":         {test: "TestPromptPipeGetSizeFailure"},
	"prompt-replay":                        {test: "TestAdapterControls/prompt-replay"},
	"runner-variant-selection":             {test: "TestAdapterControls/runner-variant-selection"},
	"adapter-env-name-outside-six":         {test: "TestAdapterControls/adapter-env-name-outside-six"},
	"adapter-denied-literal-occurrence":    {test: "TestAdapterDeniedLiteralOccurrence"},
	"prompt-decoded-size":                  {test: "TestProcessPromptDecodedSize"},
	"runner-variant-unpinned":              {test: "TestRunClaudeRefusesUntilBrokerContractPinned"},
	"claude-vendor-valid-credential":       {external: "D and broker qualification own reach evidence; masked until the run-claude pin."},
	"claude-placeholder-not-run-bound":     {external: "Gateway owns execution/generation/lease/stop/expiry/release placeholder refusal; masked until pin."},
	"claude-worker-settings":               {external: "D and broker qualification own non-worker-redirectable endpoint/settings evidence; masked until pin."},
	"status-collision-run-claude":          {external: "D requires a pinned complete runner status set excluding 78; masked until pin."},
	"claude-credential-same-uid-reachable": {external: "D and broker qualification require different-UID credential custody; masked until pin."},
	"claude-credential-helper-prints":      {external: "D and broker qualification exclude helpers returning vendor credentials; masked until pin."},
	"claude-credential-file-under-home":    {external: "D and broker qualification cover the whole readable home; masked until pin."},
	"denied-literal-member-missing":        {test: "TestRequiredDeniedLiterals"},
	"denied-literal-self-collision":        {external: "D refuses artifact self-collisions before C and claim consumption; adapter occurrence checks are only a second line."},
	"runner-unpinned-bound-by-d":           {external: "D refuses unpinned variants before handoff, C and claim consumption."},
	"runner-literal-unknown":               {test: "TestUnknownRunnerLiteralRefuses"},
	"status-remap":                         {test: "TestAdapterControls/status-remap"},
	"stdin-pipe-eintr-retry-bound":         {test: "TestPromptPipeEINTRBound"},
	"stdin-pipe-step-failure-ignored":      {test: "TestProcessPipeStepFailures"},
	"fd3-mutable":                          {external: "Manager admits an immutable plan before target code; read-only access or stable size alone cannot prove immutability."},
	"fd3-truncated":                        {external: "Manager binds complete bytes to the original admitted digest before target code."},
	"result-stdout-as-result":              {external: "D treats stdout as capture only, not authenticated durable Result."},
	"vendor-wrapper-binary":                {external: "Broker qualification pins a native vendor binary, not an npm/node wrapper."},
	"transitive-exec-empty-inference":      {external: "OS qualification requires actual minimum inventory even for a text-only turn."},
	"worker-hash-equality-inference":       {external: "D protected joins cannot be replaced by worker-computed hash agreement."},
	"timing-reset-on-reread":               {test: "TestAdapterControls/timing-reset-on-reread"},
	"timing-arithmetic-wrap":               {test: "TestAdapterControls/timing-arithmetic-wrap"},
}

// TestForbiddenControlsClassified proves all 88 controls have distinct adapter tests or explicit owning-component reasons.
func TestForbiddenControlsClassified(t *testing.T) {
	neighbors, err := boundaryCases(readTestdata(t, "boundary.witnesses.json"))
	if err != nil {
		t.Fatal(err)
	}
	controls, err := decodeForbidden(readTestdata(t, "forbidden-controls.json"), neighbors)
	if err != nil {
		t.Fatal(err)
	}
	if len(forbiddenCoverage) != len(controls) {
		t.Fatalf("classified %d, want %d", len(forbiddenCoverage), len(controls))
	}
	seenTests := map[string]bool{}
	for _, c := range controls {
		cov, ok := forbiddenCoverage[c.ID]
		if !ok || (cov.test == "") == (cov.external == "") {
			t.Errorf("control %s needs exactly one classification", c.ID)
			continue
		}
		if cov.test != "" {
			if seenTests[cov.test] {
				t.Errorf("bundled mapping: %s", cov.test)
			}
			seenTests[cov.test] = true
			if strings.HasPrefix(cov.test, "TestAdapterControls/") && adapterControlTests()[c.ID] == nil {
				t.Errorf("missing executable case %s", c.ID)
			}
		}
	}
	for id := range adapterControlTests() {
		if forbiddenCoverage[id].test != "TestAdapterControls/"+id {
			t.Errorf("unclassified executable case %s", id)
		}
	}
}
