package netproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

const RecoveryAPI = "netproxy-control/2"
const RecoveryProtocol = "member-recovery/1"
const RecoveryMaxBody = 16 << 10
const IssuanceWindowNS = uint64(120_000_000_000)

var ErrRecoveryEnvelope = errors.New("netproxy recovery: invalid issuance envelope")

// RecoveryEnvelope is the frozen issuance/revocation request. Its proof is a
// capability; diagnostic formatting never exposes any of its fields.
type RecoveryEnvelope struct {
	Root          string `json:"root"`
	Session       string `json:"session"`
	Runtime       string `json:"runtime"`
	AttemptID     string `json:"attempt_id"`
	RecoveryProof string `json:"recovery_proof"`
	RegistryID    string `json:"registry_id"`
	Boot          string `json:"boot"`
	IssuedNS      string `json:"issued_ns"`
	ExpiresNS     string `json:"expires_ns"`
}

func (RecoveryEnvelope) String() string   { return "<private recovery envelope>" }
func (RecoveryEnvelope) GoString() string { return "<private recovery envelope>" }

func (e RecoveryEnvelope) Validate() error {
	issued, ok := canonicalNS(e.IssuedNS)
	expires, expiryOK := canonicalNS(e.ExpiresNS)
	if (RegisterRequest{e.Root, e.Session, e.Runtime}).Validate() != nil || !validToken(e.AttemptID) || !validToken(e.RecoveryProof) || !validToken(e.RegistryID) || e.AttemptID == e.RecoveryProof || e.Boot == "" || !ok || !expiryOK || issued > math.MaxUint64-IssuanceWindowNS || expires != issued+IssuanceWindowNS {
		return ErrRecoveryEnvelope
	}
	return nil
}

func canonicalNS(s string) (uint64, bool) {
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil && strconv.FormatUint(n, 10) == s
}

// RecoveryHealth contains public authority/clock observations, not a grant.
type RecoveryHealth struct {
	RegistryID, Boot, NowNS, RuleSet, KeyMode string
	Ready                                     bool
}

// RecoveryRegistration carries a data-plane capability only on the initial
// accepted issuance. It never contains or reconstructs the recovery proof.
type RecoveryRegistration struct {
	AttemptID    string
	LaunchID     string
	Registration Registration
}

func (RecoveryRegistration) String() string   { return "<private recovery registration>" }
func (RecoveryRegistration) GoString() string { return "<private recovery registration>" }

// RecoveryResult exists only after an exact closed durable service response.
// It makes no claim about local receipt persistence or earlier caller history.
type RecoveryResult struct{ attempt, launch, outcome string }

func (r *RecoveryResult) AttemptID() string        { return r.attempt }
func (r *RecoveryResult) LaunchID() (string, bool) { return r.launch, r.launch != "" }
func (r *RecoveryResult) Outcome() string          { return r.outcome }

// RecoveryError exposes fixed classifications only. Raw transport/parser/body
// diagnostics and the endpoint are deliberately not retained or unwrapped.
type RecoveryError struct {
	op, kind, code string
	status         int
}

func (e *RecoveryError) Error() string {
	return fmt.Sprintf("netproxy recovery %s: %s status=%d code=%s", e.op, e.kind, e.status, e.code)
}
func (e *RecoveryError) GoString() string { return e.Error() }
func recoveryFailure(op, kind string, status int, code string) error {
	return &RecoveryError{op, kind, code, status}
}
func IsRecoveryCode(err error, status int, code string) bool {
	e, ok := err.(*RecoveryError)
	return ok && e.kind == "refused" && e.status == status && e.code == code
}

// RecoveryClient binds a canonical original Unix endpoint at construction.
// It has no environment proxy, redirect, downgrade, retry or endpoint override.
// The caller owns the durable prepare-before-register and original provenance.
type RecoveryClient struct{ http *http.Client }

func NewRecovery(socket string) (*RecoveryClient, error) {
	if !filepath.IsAbs(socket) || filepath.Clean(socket) != socket || socket == string(filepath.Separator) || strings.ContainsRune(socket, 0) {
		return nil, recoveryFailure("client", "request_not_sent", 0, "")
	}
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", socket)
	}}
	return &RecoveryClient{&http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *RecoveryClient) Health(ctx context.Context) (RecoveryHealth, error) {
	status, raw, err := c.call(ctx, "health", http.MethodGet, "/v2/health", nil)
	if err != nil {
		return RecoveryHealth{}, err
	}
	if status != 200 {
		return RecoveryHealth{}, recoveryRefusal("health", status, raw)
	}
	f, ok := recoveryObject(raw, "api", "ready", "rule_set", "key_mode", "registry_id", "boot", "now_ns", "recovery")
	ready := bytes.TrimSpace(f["ready"])
	if !ok || !recoveryVersion(f) || (!bytes.Equal(ready, []byte("true")) && !bytes.Equal(ready, []byte("false"))) {
		return RecoveryHealth{}, recoveryFailure("health", "outcome_unknown", 0, "")
	}
	strings_, ok := recoveryStrings(f, "rule_set", "key_mode", "registry_id", "boot", "now_ns", "recovery")
	_, clockOK := canonicalNS(strings_["now_ns"])
	if !ok || !validToken(strings_["registry_id"]) || strings_["boot"] == "" || !clockOK || strings_["recovery"] != RecoveryProtocol {
		return RecoveryHealth{}, recoveryFailure("health", "outcome_unknown", 0, "")
	}
	return RecoveryHealth{RegistryID: strings_["registry_id"], Boot: strings_["boot"], NowNS: strings_["now_ns"], RuleSet: strings_["rule_set"], KeyMode: strings_["key_mode"], Ready: rawTrue(ready)}, nil
}

func (c *RecoveryClient) Register(ctx context.Context, e RecoveryEnvelope) (RecoveryRegistration, error) {
	status, raw, err := c.envelopeCall(ctx, "register", e)
	if err != nil {
		return RecoveryRegistration{}, err
	}
	if status != 200 {
		return RecoveryRegistration{}, recoveryRefusal("register", status, raw)
	}
	f, ok := recoveryObject(raw, "api", "attempt_id", "launch_id", "token", "token_header", "anthropic_base", "openai_base", "chatgpt_base", "activation_seconds")
	if !ok || !recoveryVersion(f) {
		return RecoveryRegistration{}, recoveryFailure("register", "outcome_unknown", 0, "")
	}
	s, ok := recoveryStrings(f, "attempt_id", "launch_id", "token", "token_header", "anthropic_base", "openai_base", "chatgpt_base")
	var activation int
	if !ok || json.Unmarshal(f["activation_seconds"], &activation) != nil || activation < 1 ||
		s["attempt_id"] != e.AttemptID || !validToken(s["launch_id"]) || !validToken(s["token"]) ||
		s["token"] == e.RecoveryProof || s["launch_id"] == e.RecoveryProof || s["launch_id"] == s["token"] || s["token_header"] != "X-Netproxy-Token" {
		return RecoveryRegistration{}, recoveryFailure("register", "outcome_unknown", 0, "")
	}
	for _, base := range []string{s["anthropic_base"], s["openai_base"], s["chatgpt_base"]} {
		if validBase(base, s["token"]) != nil || strings.Contains(base, e.RecoveryProof) {
			return RecoveryRegistration{}, recoveryFailure("register", "outcome_unknown", 0, "")
		}
	}
	reg := Registration{s["token"], s["token_header"], s["anthropic_base"], s["openai_base"], s["chatgpt_base"], activation}
	return RecoveryRegistration{e.AttemptID, s["launch_id"], reg}, nil
}

func (c *RecoveryClient) Recover(ctx context.Context, e RecoveryEnvelope) (*RecoveryResult, error) {
	status, raw, err := c.envelopeCall(ctx, "recover", e)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, recoveryRefusal("recover", status, raw)
	}
	f, ok := recoveryObject(raw, "api", "attempt_id", "launch_id", "outcome", "durable")
	if !ok || !recoveryVersion(f) || !rawTrue(f["durable"]) {
		return nil, recoveryFailure("recover", "outcome_unknown", 0, "")
	}
	s, ok := recoveryStrings(f, "attempt_id", "outcome")
	if !ok || s["attempt_id"] != e.AttemptID {
		return nil, recoveryFailure("recover", "outcome_unknown", 0, "")
	}
	launch := ""
	switch s["outcome"] {
	case "revoked":
		var v string
		if json.Unmarshal(f["launch_id"], &v) != nil || !validToken(v) || v == e.RecoveryProof {
			return nil, recoveryFailure("recover", "outcome_unknown", 0, "")
		}
		launch = v
	case "cancelled_before_issue":
		if !bytes.Equal(bytes.TrimSpace(f["launch_id"]), []byte("null")) {
			return nil, recoveryFailure("recover", "outcome_unknown", 0, "")
		}
	default:
		return nil, recoveryFailure("recover", "outcome_unknown", 0, "")
	}
	return &RecoveryResult{e.AttemptID, launch, s["outcome"]}, nil
}

func (c *RecoveryClient) envelopeCall(ctx context.Context, op string, e RecoveryEnvelope) (int, []byte, error) {
	if e.Validate() != nil {
		return 0, nil, recoveryFailure(op, "request_not_sent", 0, "")
	}
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > RecoveryMaxBody {
		return 0, nil, recoveryFailure(op, "request_not_sent", 0, "")
	}
	return c.call(ctx, op, http.MethodPost, "/v2/"+op, raw)
}

func (c *RecoveryClient) call(ctx context.Context, op, method, path string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, "http://control"+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, recoveryFailure(op, "request_not_sent", 0, "")
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return 0, nil, recoveryFailure(op, "outcome_unknown", 0, "")
	}
	version := response.Header.Values(VersionHeader)
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, RecoveryMaxBody+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil || ctx.Err() != nil || len(raw) > RecoveryMaxBody || len(version) != 1 || version[0] != RecoveryAPI {
		return 0, nil, recoveryFailure(op, "outcome_unknown", 0, "")
	}
	return response.StatusCode, raw, nil
}

func recoveryRefusal(op string, status int, raw []byte) error {
	f, ok := recoveryObject(raw, "api", "error")
	if !ok || !recoveryVersion(f) {
		return recoveryFailure(op, "outcome_unknown", 0, "")
	}
	s, ok := recoveryStrings(f, "error")
	code := s["error"]
	allowed := map[int][]string{400: {"bad_request", "bad_root", "bad_issuance_window"}, 403: {"recovery_refused"}, 409: {"service_mismatch", "attempt_mismatch", "issuance_recorded"}, 410: {"recovery_expired"}, 422: {"config_invalid"}, 429: {"recovery_capacity"}, 503: {"store_unavailable", "persist_failed", "clock_unavailable"}, 404: {"not_found"}, 405: {"method_not_allowed"}}
	if ok {
		for _, known := range allowed[status] {
			if code == known {
				return recoveryFailure(op, "refused", status, code)
			}
		}
	}
	return recoveryFailure(op, "outcome_unknown", 0, "")
}

func rawTrue(raw []byte) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("true")) }
func recoveryVersion(f map[string]json.RawMessage) bool {
	strings_, ok := recoveryStrings(f, "api")
	return ok && strings_["api"] == RecoveryAPI
}
func recoveryStrings(f map[string]json.RawMessage, keys ...string) (map[string]string, bool) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		raw, ok := f[key]
		var value string
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &value) != nil {
			return nil, false
		}
		out[key] = value
	}
	return out, true
}

// Strict decoded-key allowlist: all required, no duplicates (including escape
// aliases), unknown or case-folded names, null object or trailing JSON.
func recoveryObject(raw []byte, keys ...string) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || len(raw) > RecoveryMaxBody || !json.Valid(raw) {
		return nil, false
	}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return nil, false
	}
	f := make(map[string]json.RawMessage, len(keys))
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || !allowed[key] {
			return nil, false
		}
		if _, duplicate := f[key]; duplicate {
			return nil, false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, false
		}
		f[key] = value
	}
	last, err := d.Token()
	if err != nil || last != json.Delim('}') || len(f) != len(keys) {
		return nil, false
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, false
	}
	return f, true
}
