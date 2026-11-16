package m0adapter

import (
	"encoding/json"
	"sort"
	"testing"
)

// controlClass says how each forbidden control of the fixture is covered.
type controlClass int

const (
	// classGrammar: executed here by at least one TestGrammarNegatives case
	// tagged with the control id.
	classGrammar controlClass = iota + 1
	// classAdapter: executed by the named adapter test in this package
	// (process tests are Linux-only).
	classAdapter
	// classShapeValid: the change is shape-valid; the adapter admits it and
	// D refuses it against protected originals (TestDOwnedChangesAreShapeValid).
	classShapeValid
	// classExternal: owned by D, manager, broker or qualification evidence;
	// the adapter has no executable negative for it.
	classExternal
)

type controlCoverage struct {
	class controlClass
	where string
}

// forbiddenCoverage classifies every one of the 53 fixture controls exactly
// once. The process tests named here are in process_linux_test.go.
var forbiddenCoverage = map[string]controlCoverage{
	"missing":           {classGrammar, "TestGrammarNegatives/missing"},
	"extra":             {classGrammar, "TestGrammarNegatives/array-member, extra-scalar"},
	"null":              {classGrammar, "TestGrammarNegatives/null"},
	"duplicate":         {classGrammar, "TestGrammarNegatives/duplicate"},
	"escaped-duplicate": {classGrammar, "TestGrammarNegatives/escaped-duplicate"},
	"invalid-utf8":      {classGrammar, "TestGrammarNegatives/invalid-utf8"},
	"bom":               {classGrammar, "TestGrammarNegatives/bom"},
	"surrogate":         {classGrammar, "TestGrammarNegatives/lone-high-surrogate"},
	"noninteger":        {classGrammar, "TestGrammarNegatives/noninteger-*"},
	"negative-zero":     {classGrammar, "TestGrammarNegatives/negative-zero"},
	"range":             {classGrammar, "TestGrammarNegatives/range"},
	"depth":             {classGrammar, "TestGrammarNegatives/depth; TestDepthBound"},
	"size":              {classGrammar, "TestGrammarNegatives/size; TestEncodedBound"},
	"escape-size":       {classGrammar, "TestGrammarNegatives/escape-size; TestEscapeExpansionBound"},
	"opaque":            {classGrammar, "TestGrammarNegatives/opaque"},
	"selectors":         {classGrammar, "TestGrammarNegatives/selector-option, catalog-option"},
	"injected-member":   {classGrammar, "TestGrammarNegatives/injected-*"},
	"missing-timing":    {classGrammar, "TestGrammarNegatives/missing-timing-*"},

	"clock":                        {classAdapter, "TestTimeoutVectors (cap 240, 900 ms refuses, no upward rounding)"},
	"timing-reset-rounding":        {classAdapter, "TestTimeoutVectors; TestRefreshTimeoutOnlyLowers"},
	"clock-coordinate":             {classAdapter, "TestSeamsRefuse: qualifiedNowMs refuses every profile"},
	"before-c":                     {classAdapter, "TestSeamsRefuse; TestProcessProductionMainRefuses (c_unpublished before any decoding)"},
	"fd":                           {classAdapter, "TestProcessDescriptorInventory (extra fd 4, mutable/oversized FD3)"},
	"fd3-left-open":                {classAdapter, "TestProcessPlanDuplicate; TestProcessSpecimenExecsRunner (runner lacks the plan object)"},
	"environment":                  {classAdapter, "TestEnvironment; TestProcessSpecimenExecsRunner (environment passed unchanged)"},
	"home-prompt":                  {classAdapter, "TestProcessSpecimenExecsRunner (stdin is exactly the FD3 prompt; no file lookup exists)"},
	"stdin-pipe-lifecycle":         {classAdapter, "TestProcessPromptPipe (one non-blocking whole write, write end closed, runner sees EOF); plants M7/M8"},
	"stdin-pipe-capacity-short":    {classAdapter, "TestPromptPipeCapacity (shrunk pipe refuses before any write); plant M6"},
	"runner-variant-selection":     {classAdapter, "TestLiterals (the runner command comes only from the -X literal); TestMappingRefusals (run-claude refuses runner_variant_unpinned, never falls back to run-codex)"},
	"claude-credential-in-adapter": {classAdapter, "TestEnvironment (Claude token, base URL and config variables refuse env_unexpected); the adapter builds no Claude argv until the variant is pinned"},
	"prompt-replay":                {classAdapter, "TestProcessPromptPipe (one prompt then EOF)"},
	"status-collision":             {classAdapter, "TestProcessRunnerStatusPassThrough; TestReasonTable"},

	"identity":        {classShapeValid, "TestDOwnedChangesAreShapeValid/identity"},
	"generation":      {classShapeValid, "TestDOwnedChangesAreShapeValid/generation"},
	"association":     {classShapeValid, "TestDOwnedChangesAreShapeValid/association"},
	"pins":            {classShapeValid, "TestDOwnedChangesAreShapeValid/pins"},
	"catalog":         {classShapeValid, "TestDOwnedChangesAreShapeValid/catalog"},
	"pin-drift":       {classShapeValid, "TestDOwnedChangesAreShapeValid/pin-drift"},
	"tampered-timing": {classShapeValid, "TestDOwnedChangesAreShapeValid/tampered-timing"},

	"second-start":                    {classExternal, "D replay refusal after unknown effect"},
	"unqualified":                     {classExternal, "broker protected Resolve (69 lease_harness_unqualified)"},
	"config":                          {classExternal, "broker protected configuration read"},
	"result":                          {classExternal, "D Result/read authority"},
	"qualification-measurement-order": {classExternal, "broker qualification order evidence"},
	"unknown-publication":             {classExternal, "D publication/Result debt"},
	"late-stop":                       {classExternal, "D disposition after runner rename"},
	"native-adapter":                  {classExternal, "manager/installation qualification (this artifact is a native Go binary)"},
	"d-path":                          {classExternal, "D protected profile/plan equality; adapter adds no PATH rewrite"},
	"transitive-exec":                 {classExternal, "R-EXEC OS allowlist qualification"},
	"drop":                            {classExternal, "manager verified drop before target code"},
	"result-overflow":                 {classExternal, "D capture/debt accounting"},
	"agent-handle-reexec":             {classExternal, "broker app-server handle fix and native trace"},
	"numeric-fd-false-positive":       {classExternal, "evidence inference rule; adapter sweeps compare (dev, ino), not numbers"},
	"path-kernel-conflation":          {classExternal, "evidence inference rule for OS exec enforcement"},
	"worker-protected-read":           {classExternal, "structural: the adapter has no protected read path; D owns equality"},
	"status-attribution":              {classExternal, "D original fate evidence; numeric 78 authenticates nothing"},
}

// Every fixture control is classified exactly once and no classification
// names a control that is not in the fixture; every grammar-class control
// is executed by at least one tagged grammar negative.
func TestForbiddenControlsClassified(t *testing.T) {
	var controls []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(readTestdata(t, "forbidden-controls.json"), &controls); err != nil {
		t.Fatal(err)
	}
	if len(controls) != 56 {
		t.Fatalf("fixture has %d controls, want 56", len(controls))
	}
	inFixture := map[string]bool{}
	for _, c := range controls {
		if inFixture[c.ID] {
			t.Errorf("duplicate fixture id %s", c.ID)
		}
		inFixture[c.ID] = true
		if _, ok := forbiddenCoverage[c.ID]; !ok {
			t.Errorf("control %s is not classified", c.ID)
		}
	}
	var extra []string
	for id := range forbiddenCoverage {
		if !inFixture[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("classified ids not in the fixture: %v", extra)
	}
	tagged := map[string]bool{}
	for _, c := range grammarNegatives() {
		if c.control != "" {
			if !inFixture[c.control] {
				t.Errorf("grammar case %s tags unknown control %s", c.name, c.control)
			}
			tagged[c.control] = true
		}
	}
	for id, cov := range forbiddenCoverage {
		if cov.class == classGrammar && !tagged[id] {
			t.Errorf("grammar-class control %s has no tagged negative", id)
		}
		if cov.class != classGrammar && tagged[id] {
			t.Errorf("control %s is tagged by a grammar case but classified %d", id, cov.class)
		}
	}
}
