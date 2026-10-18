package selector

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"unicode/utf8"

	"github.com/relux-works/curator-agent-launcher/internal/defaults"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/skill-agents-management/pkg/vendorplugin"
)

type Task struct {
	ID          *string `json:"id"`
	Class       string  `json:"class"`
	Difficulty  *string `json:"difficulty"`
	Sensitivity *string `json:"sensitivity"`
	Language    *string `json:"language"`
	Platform    *string `json:"platform"`
}
type Flags struct {
	Model  *string `json:"model"`
	Effort *string `json:"effort"`
}
type Request struct {
	EnvID           string                        `json:"env_id"`
	Profile         string                        `json:"profile"`
	CWD             string                        `json:"cwd"`
	Role            *string                       `json:"role"`
	Project         *string                       `json:"project"`
	Werkstatt       *string                       `json:"werkstatt"`
	Task            *Task                         `json:"task"`
	Flags           Flags                         `json:"flags"`
	Candidates      []defaults.SelectionCandidate `json:"candidates"`
	AdmissionSource string                        `json:"admission_source"`
}

// BuildRequest exposes the plain-launch shape for later dispatch enrichment.
// Admission is the same registry/lineup used by defaults.Complete; no board or
// alternative catalog is consulted. Call Encode after filling dispatch fields.
func BuildRequest(env, profile, cwd string, flags Flags, reg *vendorplugin.Registry) (Request, error) {
	candidates, err := defaults.SelectionCandidates(env, reg)
	if err != nil {
		return Request{}, err
	}
	return Request{EnvID: env, Profile: profile, CWD: cwd, Flags: flags, Candidates: candidates, AdmissionSource: defaults.AdmissionSource()}, nil
}
func Encode(r Request) ([]byte, string, error) {
	if !filepath.IsAbs(r.CWD) || !validRequestUTF8(r) {
		return nil, "", unavailable("invalid")
	}

	data, err := json.Marshal(r)
	if err != nil {
		return nil, "", unavailable("invalid")
	}
	v, err := fragment.ParseJSON(data)
	if err != nil {
		return nil, "", unavailable("invalid")
	}
	data = fragment.Canonical(v)
	if len(data) > Limit {
		return nil, "", unavailable("invalid")
	}
	digest := fragment.Digest(v)
	return data, digest, nil
}

type Decision struct {
	DecisionID string  `json:"decision_id"`
	Runtime    string  `json:"runtime"`
	Model      string  `json:"model"`
	Effort     *string `json:"effort"`
	WeightsID  *string `json:"weights_id"`
}

var valuePattern = regexp.MustCompile(`^[A-Za-z0-9._:/@+-]{1,128}$`)
var weightsPattern = regexp.MustCompile(`^gguf-sha256:[0-9a-f]{64}$`)
var refusalPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

func ParseResponse(data []byte, candidates []defaults.SelectionCandidate) (Decision, error) {
	bad := func() (Decision, error) { return Decision{}, unavailable("invalid") }
	if len(data) > Limit {
		return bad()
	}
	v, err := fragment.ParseJSON(data)
	if err != nil || !closed(v, []string{"decision_id", "runtime", "model", "effort", "weights_id", "refusal"}, nil) {
		return bad()
	}
	refusal, _ := v.Get("refusal")
	// Validate all members before returning an abstention: a poisoned choice or
	// weights field cannot be smuggled through a valid refusal code.
	var d Decision
	for _, row := range []struct {
		name string
		dst  *string
	}{{"decision_id", &d.DecisionID}, {"runtime", &d.Runtime}, {"model", &d.Model}} {
		x, _ := v.Get(row.name)
		if refusal.Kind != fragment.KindNull && x.Kind == fragment.KindNull {
			continue
		}
		if x.Kind != fragment.KindString || !valuePattern.MatchString(x.Str) {
			return bad()
		}
		*row.dst = x.Str
	}
	effort, _ := v.Get("effort")
	if effort.Kind != fragment.KindNull {
		if effort.Kind != fragment.KindString || !valuePattern.MatchString(effort.Str) {
			return bad()
		}
		d.Effort = &effort.Str
	}
	weights, _ := v.Get("weights_id")
	if weights.Kind != fragment.KindNull {
		if weights.Kind != fragment.KindString || !weightsPattern.MatchString(weights.Str) {
			return bad()
		}
		d.WeightsID = &weights.Str
	}
	if refusal.Kind != fragment.KindNull {
		if !closed(refusal, []string{"code"}, nil) {
			return bad()
		}
		code, _ := refusal.Get("code")
		if code.Kind != fragment.KindString || !refusalPattern.MatchString(code.Str) {
			return bad()
		}
		return Decision{}, &Failure{Code: CodeAbstained, RefusalCode: code.Str}
	}
	found := false
	for _, candidate := range candidates {
		if candidate.Equal(defaults.SelectionCandidate{Runtime: d.Runtime, Model: d.Model, Effort: d.Effort}) {
			found = true
			break
		}
	}
	if !found {
		return bad()
	}
	return d, nil
}

// encoding/json repairs invalid UTF-8. Reject it before encoding so the CCJ-1
// digest and selector input describe the caller's actual bytes without repair.
func validRequestUTF8(r Request) bool {
	values := []string{r.EnvID, r.Profile, r.CWD, r.AdmissionSource}
	pointers := []*string{r.Role, r.Project, r.Werkstatt, r.Flags.Model, r.Flags.Effort}
	if r.Task != nil {
		values = append(values, r.Task.Class)
		pointers = append(pointers, r.Task.ID, r.Task.Difficulty, r.Task.Sensitivity, r.Task.Language, r.Task.Platform)
	}
	for _, c := range r.Candidates {
		values = append(values, c.Runtime, c.Model)
		pointers = append(pointers, c.Effort)
	}
	for _, p := range pointers {
		if p != nil {
			values = append(values, *p)
		}
	}
	for _, value := range values {
		if !utf8.ValidString(value) {
			return false
		}
	}
	return true
}
