package netproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func recoveryEnvelopeFixture() RecoveryEnvelope {
	return RecoveryEnvelope{"/fixture/project", "environment-20261104T020000Z-123-attempt", "codex", strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), "fixture-boot", "23", "120000000023"}
}
func recoveryReplyFixture(op string) map[string]any {
	e := recoveryEnvelopeFixture()
	f := map[string]any{"api": RecoveryAPI}
	switch op {
	case "health":
		f["ready"] = true
		f["rule_set"] = "fixture"
		f["key_mode"] = "fixture"
		f["registry_id"] = e.RegistryID
		f["boot"] = e.Boot
		f["now_ns"] = e.IssuedNS
		f["recovery"] = RecoveryProtocol
	case "register":
		f["attempt_id"] = e.AttemptID
		f["launch_id"] = strings.Repeat("d", 64)
		f["token"] = testToken
		f["token_header"] = "X-Netproxy-Token"
		f["anthropic_base"] = "http://127.0.0.1:1234/anthropic"
		f["openai_base"] = "http://[::1]:1234/openai/v1"
		f["chatgpt_base"] = "http://127.0.0.1:1234/chatgpt"
		f["activation_seconds"] = 120
	case "recover":
		f["attempt_id"] = e.AttemptID
		f["launch_id"] = strings.Repeat("d", 64)
		f["outcome"] = "revoked"
		f["durable"] = true
	}
	return f
}
func recoveryJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type recoveryRoundTrip func(*http.Request) (*http.Response, error)

func (f recoveryRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func recoveryTestClient(t *testing.T, status int, raw string, headers []string) *RecoveryClient {
	t.Helper()
	c, err := NewRecovery("/fixture/control.sock")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport = recoveryRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{VersionHeader: headers}, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})
	return c
}
func recoveryInvoke(c *RecoveryClient, op string) error {
	e := recoveryEnvelopeFixture()
	switch op {
	case "health":
		_, err := c.Health(context.Background())
		return err
	case "register":
		_, err := c.Register(context.Background(), e)
		return err
	default:
		_, err := c.Recover(context.Background(), e)
		return err
	}
}
func requireUnknown(t *testing.T, err error) {
	t.Helper()
	var typed *RecoveryError
	if !errors.As(err, &typed) || typed.kind != "outcome_unknown" {
		t.Fatalf("RECOVERY WIRE ALARM: unconfirmed response accepted: %v", err)
	}
}

// A real synthetic Unix socket receives the exact closed v2 requests. The
// initial token is separate from the proof, and recovery yields only a result.
func TestRecoveryV2UnixLifecycle(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Header().Set(VersionHeader, RecoveryAPI)
		op := strings.TrimPrefix(r.URL.Path, "/v2/")
		io.WriteString(w, recoveryJSON(t, recoveryReplyFixture(op)))
	})
	c, err := NewRecovery(f.socket)
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Health(t.Context())
	if err != nil || h.RegistryID != recoveryEnvelopeFixture().RegistryID || h.NowNS != "23" {
		t.Fatal("valid health refused")
	}
	e := recoveryEnvelopeFixture()
	reg, err := c.Register(t.Context(), e)
	if err != nil || reg.Registration.Token != testToken || reg.AttemptID != e.AttemptID {
		t.Fatal("valid first issuance refused")
	}
	result, err := c.Recover(t.Context(), e)
	if err != nil || result == nil || result.AttemptID() != e.AttemptID || result.Outcome() != "revoked" {
		t.Fatal("valid authenticated result refused")
	}
	if launch, ok := result.LaunchID(); !ok || launch != reg.LaunchID {
		t.Fatal("launch correlation changed")
	}
	seen := f.requests()
	if len(seen) != 3 || seen[0].method != "GET" || seen[0].path != "/v2/health" || len(seen[0].body) != 0 {
		t.Fatal("wrong health/request count")
	}
	want, _ := json.Marshal(e)
	for i, op := range []string{"register", "recover"} {
		r := seen[i+1]
		if r.method != "POST" || r.path != "/v2/"+op || !bytes.Equal(r.body, want) {
			t.Fatal("original envelope changed on wire")
		}
		var fields map[string]any
		if json.Unmarshal(r.body, &fields) != nil || len(fields) != 9 {
			t.Fatal("wrong request fields")
		}
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		s := fmt.Sprintf(format, reg) + fmt.Sprintf(format, e)
		if strings.Contains(s, testToken) || strings.Contains(s, e.RecoveryProof) || strings.Contains(s, e.Root) {
			t.Fatal("private wire value rendered")
		}
	}
}

// Both success outcomes have an exact generation/null pairing; a false durable
// bit, foreign attempt, or any other shape never becomes a confirmation.
func TestRecoveryV2OutcomePredicates(t *testing.T) {
	for _, kind := range []string{"revoked", "cancelled", "false", "null-durable", "foreign", "empty-launch", "proof-launch", "cancelled-with-launch", "revoked-null", "unknown-outcome"} {
		t.Run(kind, func(t *testing.T) {
			f := recoveryReplyFixture("recover")
			switch kind {
			case "cancelled":
				f["outcome"] = "cancelled_before_issue"
				f["launch_id"] = nil
			case "false":
				f["durable"] = false
			case "null-durable":
				f["durable"] = nil
			case "foreign":
				f["attempt_id"] = strings.Repeat("e", 64)
			case "empty-launch":
				f["launch_id"] = ""
			case "proof-launch":
				f["launch_id"] = recoveryEnvelopeFixture().RecoveryProof
			case "cancelled-with-launch":
				f["outcome"] = "cancelled_before_issue"
			case "revoked-null":
				f["launch_id"] = nil
			case "unknown-outcome":
				f["outcome"] = "unknown"
			}
			c := recoveryTestClient(t, 200, recoveryJSON(t, f), []string{RecoveryAPI})
			r, err := c.Recover(t.Context(), recoveryEnvelopeFixture())
			if kind == "revoked" || kind == "cancelled" {
				if err != nil || r == nil {
					t.Fatal("legitimate result rejected")
				}
				_, has := r.LaunchID()
				if has != (kind == "revoked") {
					t.Fatal("null generation pairing wrong")
				}
			} else {
				requireUnknown(t, err)
				if r != nil {
					t.Fatal("failed response retained a confirmation")
				}
			}
		})
	}
}

// Every v2 operation uses exact decoded-key/version/type closure. This tests
// reply parsing, not the service's proof-verification implementation.
func TestRecoveryV2ClosedReplies(t *testing.T) {
	for _, op := range []string{"health", "register", "recover"} {
		for _, kind := range []string{"valid", "unknown", "omitted", "duplicate", "escaped-duplicate", "case", "null", "array", "trailing", "api-null", "api-v1", "version-none", "version-double", "version-v1", "oversize"} {
			t.Run(op+"/"+kind, func(t *testing.T) {
				f := recoveryReplyFixture(op)
				headers := []string{RecoveryAPI}
				raw := recoveryJSON(t, f)
				switch kind {
				case "unknown":
					f["token_echo"] = "secret"
					raw = recoveryJSON(t, f)
				case "omitted":
					delete(f, "api")
					raw = recoveryJSON(t, f)
				case "duplicate":
					raw = `{"api":"wrong",` + raw[1:]
				case "escaped-duplicate":
					raw = `{"\u0061pi":"wrong",` + raw[1:]
				case "case":
					raw = strings.Replace(raw, `"api"`, `"API"`, 1)
				case "null":
					raw = "null"
				case "array":
					raw = "[" + raw + "]"
				case "trailing":
					raw += " {}"
				case "api-null":
					f["api"] = nil
					raw = recoveryJSON(t, f)
				case "api-v1":
					f["api"] = APIVersion
					raw = recoveryJSON(t, f)
				case "version-none":
					headers = nil
				case "version-double":
					headers = []string{RecoveryAPI, RecoveryAPI}
				case "version-v1":
					headers = []string{APIVersion}
				case "oversize":
					raw += strings.Repeat(" ", RecoveryMaxBody)
				}
				err := recoveryInvoke(recoveryTestClient(t, 200, raw, headers), op)
				if kind == "valid" {
					if err != nil {
						t.Fatal("canonical valid response rejected")
					}
				} else {
					requireUnknown(t, err)
				}
			})
		}
	}
}

// Error status/code pairs are a positive allowlist. Legacy safe404, arbitrary
// HTTP failures and reflected diagnostics cannot certify recovery or leak data.
func TestRecoveryV2Refusals(t *testing.T) {
	for status, codes := range map[int][]string{400: {"bad_request", "bad_root", "bad_issuance_window"}, 403: {"recovery_refused"}, 409: {"service_mismatch", "attempt_mismatch", "issuance_recorded"}, 410: {"recovery_expired"}, 422: {"config_invalid"}, 429: {"recovery_capacity"}, 503: {"store_unavailable", "persist_failed", "clock_unavailable"}, 404: {"not_found"}, 405: {"method_not_allowed"}} {
		for _, code := range codes {
			t.Run(code, func(t *testing.T) {
				for _, op := range []string{"health", "register", "recover"} {
					c := recoveryTestClient(t, status, recoveryJSON(t, map[string]any{"api": RecoveryAPI, "error": code}), []string{RecoveryAPI})
					err := recoveryInvoke(c, op)
					if !IsRecoveryCode(err, status, code) {
						t.Fatal("documented refusal lost")
					}
				}
			})
		}
	}
	for _, kind := range []string{"v1-404", "unknown-token", "wrong-status", "echo", "extra", "duplicate", "redirect", "204"} {
		t.Run(kind, func(t *testing.T) {
			status := 503
			f := map[string]any{"api": RecoveryAPI, "error": "persist_failed"}
			headers := []string{RecoveryAPI}
			switch kind {
			case "v1-404":
				status = 404
				headers = []string{APIVersion}
				f["api"] = APIVersion
				f["error"] = "unknown_token"
			case "unknown-token":
				status = 404
				f["error"] = "unknown_token"
			case "wrong-status":
				status = 403
			case "echo":
				f["error"] = recoveryEnvelopeFixture().RecoveryProof
			case "extra":
				f["raw"] = recoveryEnvelopeFixture()
			case "redirect":
				status = 302
			case "204":
				status = 204
			}
			raw := recoveryJSON(t, f)
			if kind == "duplicate" {
				raw = `{"error":"echo",` + raw[1:]
			}
			c := recoveryTestClient(t, status, raw, headers)
			result, err := c.Recover(t.Context(), recoveryEnvelopeFixture())
			requireUnknown(t, err)
			if result != nil {
				t.Fatal("unconfirmed effect certified")
			}
			for _, format := range []string{"%v", "%+v", "%#v"} {
				s := fmt.Sprintf(format, err)
				if strings.Contains(s, recoveryEnvelopeFixture().RecoveryProof) || strings.Contains(s, "/fixture") {
					t.Fatal("reflected error or endpoint leaked")
				}
			}
		})
	}
}

// Invalid/oversize E refuses before any request. Valid historical registration
// forms remain admitted, using the same v1 root/session/runtime predicates.
func TestRecoveryV2RequestAndHealthBounds(t *testing.T) {
	for _, edit := range []func(*RecoveryEnvelope){func(e *RecoveryEnvelope) { e.Root = "relative" }, func(e *RecoveryEnvelope) { e.RecoveryProof = e.AttemptID }, func(e *RecoveryEnvelope) { e.IssuedNS = "023" }, func(e *RecoveryEnvelope) { e.Boot = strings.Repeat("x", RecoveryMaxBody) }} {
		c := recoveryTestClient(t, 200, "", []string{RecoveryAPI})
		var calls int
		c.http.Transport = recoveryRoundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("never") })
		e := recoveryEnvelopeFixture()
		edit(&e)
		_, err := c.Register(t.Context(), e)
		if err == nil || calls != 0 {
			t.Fatal("invalid request was sent")
		}
		_, err = c.Recover(t.Context(), e)
		if err == nil || calls != 0 {
			t.Fatal("invalid recovery was sent")
		}
	}
	for _, runtime := range []string{"codex", "claude", "a0", "custom-runtime-1"} {
		e := recoveryEnvelopeFixture()
		e.Runtime = runtime
		e.Session = "env-20261104T020000Z-123-SAM!~"
		if e.Validate() != nil {
			t.Fatal("legitimate registration form narrowed")
		}
	}
	for _, kind := range []string{"canonical", "max-body", "overflow", "leading-zero", "not-ready", "bad-registry", "boot-empty", "wrong-protocol", "wrong-string"} {
		t.Run(kind, func(t *testing.T) {
			f := recoveryReplyFixture("health")
			switch kind {
			case "max-body":
				f["rule_set"] = strings.Repeat("x", RecoveryMaxBody-len(recoveryJSON(t, f))+len(f["rule_set"].(string)))
			case "overflow":
				f["now_ns"] = "18446744073709551616"
			case "leading-zero":
				f["now_ns"] = "023"
			case "not-ready":
				f["ready"] = false
			case "bad-registry":
				f["registry_id"] = "short"
			case "boot-empty":
				f["boot"] = ""
			case "wrong-protocol":
				f["recovery"] = "member-recovery/2"
			case "wrong-string":
				f["rule_set"] = nil
			}
			raw := recoveryJSON(t, f)
			if kind == "max-body" && len(raw) != RecoveryMaxBody {
				t.Fatal("boundary fixture wrong")
			}
			err := recoveryInvoke(recoveryTestClient(t, 200, raw, []string{RecoveryAPI}), "health")
			if kind == "canonical" || kind == "max-body" {
				if err != nil {
					t.Fatal("valid health bound rejected")
				}
			} else {
				requireUnknown(t, err)
			}
		})
	}
	for _, path := range []string{"relative", "/fixture/../other", "/fixture/", "/", "/fixture\x00sock"} {
		if c, err := NewRecovery(path); c != nil || err == nil {
			t.Fatal("invalid endpoint admitted")
		}
	}
}

// Register retains the v1 token/base predicates and exact v2 header, while
// preventing the distinct recovery proof from entering any child-facing field.
func TestRecoveryV2RegistrationPredicates(t *testing.T) {
	for _, kind := range []string{"canonical", "foreign-attempt", "bad-launch", "bad-token", "proof-token", "proof-launch", "token-launch", "header", "base-remote", "base-credential", "base-query", "base-fragment", "base-token", "base-proof", "activation", "activation-null", "activation-string"} {
		t.Run(kind, func(t *testing.T) {
			f := recoveryReplyFixture("register")
			switch kind {
			case "foreign-attempt":
				f["attempt_id"] = strings.Repeat("f", 64)
			case "bad-launch":
				f["launch_id"] = "bad"
			case "bad-token":
				f["token"] = "bad"
			case "proof-token":
				f["token"] = recoveryEnvelopeFixture().RecoveryProof
			case "proof-launch":
				f["launch_id"] = recoveryEnvelopeFixture().RecoveryProof
			case "token-launch":
				f["launch_id"] = testToken
			case "header":
				f["token_header"] = "Authorization"
			case "base-remote":
				f["openai_base"] = "http://example.test:80/v1"
			case "base-credential":
				f["openai_base"] = "http://user:pass@127.0.0.1:1234/v1"
			case "base-query":
				f["openai_base"] = "http://127.0.0.1:1234/v1?x=1"
			case "base-fragment":
				f["openai_base"] = "http://127.0.0.1:1234/v1#x"
			case "base-token":
				f["openai_base"] = "http://127.0.0.1:1234/" + testToken
			case "base-proof":
				f["openai_base"] = "http://127.0.0.1:1234/" + recoveryEnvelopeFixture().RecoveryProof
			case "activation":
				f["activation_seconds"] = 0
			case "activation-null":
				f["activation_seconds"] = nil
			case "activation-string":
				f["activation_seconds"] = "120"
			}
			_, err := recoveryTestClient(t, 200, recoveryJSON(t, f), []string{RecoveryAPI}).Register(t.Context(), recoveryEnvelopeFixture())
			if kind == "canonical" {
				if err != nil {
					t.Fatal("legitimate register refused")
				}
			} else {
				requireUnknown(t, err)
			}
		})
	}
}

type recoveryBadBody struct {
	io.Reader
	failRead, failClose bool
}

func (b recoveryBadBody) Read(p []byte) (int, error) {
	if b.failRead {
		return 0, errors.New("private body diagnostic")
	}
	return b.Reader.Read(p)
}
func (b recoveryBadBody) Close() error {
	if b.failClose {
		return errors.New("private close diagnostic")
	}
	return nil
}

// Transport loss, read/close failure and cancellation return no result. The
// client performs one operation only; retry/reconciliation policy is upstream.
func TestRecoveryV2Uncertainty(t *testing.T) {
	for _, kind := range []string{"transport", "read", "close", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			c, err := NewRecovery("/fixture/control.sock")
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.http.Transport = recoveryRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				if kind == "transport" {
					return nil, errors.New("private transport diagnostic")
				}
				if kind == "cancel" {
					cancel()
				}
				return &http.Response{StatusCode: 200, Header: http.Header{VersionHeader: []string{RecoveryAPI}}, Body: recoveryBadBody{strings.NewReader(recoveryJSON(t, recoveryReplyFixture("recover"))), kind == "read", kind == "close"}}, nil
			})
			r, err := c.Recover(ctx, recoveryEnvelopeFixture())
			requireUnknown(t, err)
			if r != nil || calls != 1 || strings.Contains(fmt.Sprintf("%#v", err), "private ") {
				t.Fatal("transport uncertainty retried/certified/leaked")
			}
		})
	}
}

// A lost initial response never triggers a second register. Explicit later
// reconciliation carries the byte-identical original E, with no token replay.
func TestRecoveryV2LostReplyAndNoRedirect(t *testing.T) {
	for _, kind := range []string{"lost-register", "redirect"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			f := newFake(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
				calls.Add(1)
				if r.URL.Path == "/v2/register" {
					if kind == "redirect" {
						w.Header().Set("Location", "http://example.test/foreign")
						w.WriteHeader(302)
						return
					}
					h, ok := w.(http.Hijacker)
					if !ok {
						t.Error("missing hijacker")
						return
					}
					conn, _, err := h.Hijack()
					if err == nil {
						conn.Close()
					}
					return
				}
				w.Header().Set(VersionHeader, RecoveryAPI)
				io.WriteString(w, recoveryJSON(t, recoveryReplyFixture("recover")))
			})
			c, err := NewRecovery(f.socket)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			e := recoveryEnvelopeFixture()
			_, err = c.Register(ctx, e)
			requireUnknown(t, err)
			if calls.Load() != 1 {
				t.Fatal("RECOVERY WIRE ALARM: automatic duplicate registration/redirect")
			}
			if kind == "lost-register" {
				if r, err := c.Recover(ctx, e); err != nil || r == nil || calls.Load() != 2 {
					t.Fatal("explicit recovery failed")
				}
				seen := f.requests()
				if !bytes.Equal(seen[0].body, seen[1].body) {
					t.Fatal("frozen E changed after lost reply")
				}
			}
		})
	}
}
