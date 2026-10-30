package netproxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

const registerOK = `{"api":"netproxy-control/1","token":"` + testToken + `","token_header":"X-Netproxy-Token",` +
	`"anthropic_base":"http://127.0.0.1:47811/anthropic","openai_base":"http://127.0.0.1:47811/openai/v1",` +
	`"chatgpt_base":"http://127.0.0.1:47811/chatgpt/backend-api/codex","activation_seconds":120}`

// fake serves the control API on a unix socket. reply decides each answer;
// every request's method, path and body are recorded.
type fake struct {
	socket string
	mu     sync.Mutex
	seen   []seenRequest
	reply  func(w http.ResponseWriter, r *http.Request, body []byte)
}

type seenRequest struct {
	method, path string
	body         []byte
}

func newFake(t *testing.T, reply func(w http.ResponseWriter, r *http.Request, body []byte)) *fake {
	t.Helper()
	// Unix socket paths are short on darwin; t.TempDir can exceed the limit.
	dir, err := os.MkdirTemp("", "np")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &fake{socket: filepath.Join(dir, "control.sock"), reply: reply}
	ln, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.seen = append(f.seen, seenRequest{r.Method, r.URL.Path, body})
		f.mu.Unlock()
		f.reply(w, r, body)
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return f
}

func (f *fake) requests() []seenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seenRequest(nil), f.seen...)
}

func answer(status int, body string) func(http.ResponseWriter, *http.Request, []byte) {
	return func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set(VersionHeader, APIVersion)
		w.WriteHeader(status)
		io.WriteString(w, body)
	}
}

// v1 is the service as specified: health ready, register with all six
// fields, activate and renew active, unregister 204.
func v1(w http.ResponseWriter, r *http.Request, _ []byte) {
	switch r.URL.Path {
	case "/v1/health":
		answer(200, `{"api":"netproxy-control/1","ready":true,"rule_set":"h","key_mode":"pilot-file"}`)(w, r, nil)
	case "/v1/register":
		answer(200, registerOK)(w, r, nil)
	case "/v1/activate", "/v1/renew":
		answer(200, `{"api":"netproxy-control/1","active":true}`)(w, r, nil)
	case "/v1/unregister":
		w.Header().Set(VersionHeader, APIVersion)
		w.WriteHeader(204)
	default:
		answer(404, `{"api":"netproxy-control/1","error":"not_found"}`)(w, r, nil)
	}
}

func validRequest() RegisterRequest {
	return RegisterRequest{Root: "/Users/op/project", Session: "sess-1", Runtime: "codex"}
}

func kindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}

// Positive control (conformance 1, 2, 4, 6, 19): a v1 service answers every
// lifecycle call, and register yields the six fields exactly as sent.
func TestLifecycleAgainstV1Service(t *testing.T) {
	f := newFake(t, v1)
	c := New(f.socket)
	ctx := context.Background()
	if err := c.Health(ctx); err != nil {
		t.Fatal(err)
	}
	reg, err := c.Register(ctx, validRequest())
	if err != nil {
		t.Fatal(err)
	}
	want := Registration{testToken, "X-Netproxy-Token", "http://127.0.0.1:47811/anthropic", "http://127.0.0.1:47811/openai/v1", "http://127.0.0.1:47811/chatgpt/backend-api/codex", 120}
	if reg != want {
		t.Fatalf("registration = %+v", reg)
	}
	for _, call := range []func(context.Context, string) error{c.Activate, c.Renew, c.Unregister} {
		if err := call(ctx, reg.Token); err != nil {
			t.Fatal(err)
		}
	}
}

// Conformance 18 from the client side: request bodies are exactly the
// listed string members, in a fixed order, with nothing after the object.
func TestRequestBodiesAreExact(t *testing.T) {
	f := newFake(t, v1)
	c := New(f.socket)
	ctx := context.Background()
	if _, err := c.Register(ctx, validRequest()); err != nil {
		t.Fatal(err)
	}
	if err := c.Activate(ctx, testToken); err != nil {
		t.Fatal(err)
	}
	got := f.requests()
	if len(got) != 2 {
		t.Fatalf("requests = %d", len(got))
	}
	if got[0].method != "POST" || got[0].path != "/v1/register" || string(got[0].body) != `{"root":"/Users/op/project","session":"sess-1","runtime":"codex"}` {
		t.Fatalf("register request = %s %s %s", got[0].method, got[0].path, got[0].body)
	}
	if got[1].method != "POST" || got[1].path != "/v1/activate" || string(got[1].body) != `{"token":"`+testToken+`"}` {
		t.Fatalf("activate request = %s %s", got[1].method, got[1].path)
	}
}

// Conformance 7, 8, 16, 21: anything but a version-checked ready health is
// a not-ready service. Each case is paired with the ready control above.
func TestHealthRefusals(t *testing.T) {
	cases := map[string]struct {
		reply func(http.ResponseWriter, *http.Request, []byte)
		kind  Kind
		code  string
	}{
		"store_unavailable":  {answer(503, `{"api":"netproxy-control/1","ready":false,"error":"store_unavailable"}`), Refused, "store_unavailable"},
		"persist_failed":     {answer(503, `{"api":"netproxy-control/1","ready":false,"error":"persist_failed"}`), Refused, "persist_failed"},
		"ready false at 200": {answer(200, `{"api":"netproxy-control/1","ready":false}`), Unreachable, ""},
		"ready missing":      {answer(200, `{"api":"netproxy-control/1"}`), Unreachable, ""},
		"no version header": {func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			io.WriteString(w, `{"api":"netproxy-control/1","ready":true}`)
		}, Unreachable, ""},
		"other major version": {func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			w.Header().Set(VersionHeader, "netproxy-control/2")
			io.WriteString(w, `{"api":"netproxy-control/2","ready":true}`)
		}, Unreachable, ""},
		"version header twice": {func(w http.ResponseWriter, _ *http.Request, _ []byte) {
			w.Header().Add(VersionHeader, APIVersion)
			w.Header().Add(VersionHeader, APIVersion)
			io.WriteString(w, `{"api":"netproxy-control/1","ready":true}`)
		}, Unreachable, ""},
		"body without api": {answer(200, `{"ready":true}`), Unreachable, ""},
		"body over bound":  {answer(200, `{"api":"netproxy-control/1","ready":true,"pad":"`+strings.Repeat("x", MaxReply)+`"}`), Unreachable, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t, tc.reply)
			err := New(f.socket).Health(context.Background())
			if err == nil || kindOf(err) != tc.kind {
				t.Fatalf("err = %v, want kind %d", err, tc.kind)
			}
			if tc.code != "" && !IsCode(err, 503, tc.code) {
				t.Fatalf("err = %v, want code %s", err, tc.code)
			}
			// The bound refuses on its own; a truncated body must not merely
			// fail to decode.
			if name == "body over bound" && !strings.Contains(err.Error(), "exceeds bound") {
				t.Fatalf("err = %v, want the reply bound", err)
			}
		})
	}
}

// Conformance 7: no socket, a refused connection and a timeout are all
// "service down".
func TestServiceDown(t *testing.T) {
	t.Run("no socket", func(t *testing.T) {
		err := New(filepath.Join(t.TempDir(), "missing.sock")).Health(context.Background())
		if kindOf(err) != Unreachable {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		release := make(chan struct{})
		f := newFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		})
		defer close(release)
		c := New(f.socket)
		c.Timeout = 150 * time.Millisecond
		start := time.Now()
		err := c.Health(context.Background())
		if kindOf(err) != Unreachable || !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("err = %v", err)
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("timeout not bounded")
		}
	})
}

// Conformance 9, 10, 11: a register failure reply is a refusal that names
// the service's code, and the client keeps no token.
func TestRegisterRefusals(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{{400, "bad_root"}, {400, "bad_request"}, {422, "config_invalid"}, {503, "store_unavailable"}} {
		t.Run(tc.code, func(t *testing.T) {
			f := newFake(t, answer(tc.status, `{"api":"netproxy-control/1","error":"`+tc.code+`"}`))
			reg, err := New(f.socket).Register(context.Background(), validRequest())
			if !IsCode(err, tc.status, tc.code) || reg.Token != "" {
				t.Fatalf("reg = %v err = %v", reg, err)
			}
		})
	}
}

// A register reply the launcher cannot use safely refuses as a protocol
// error: a missing token_header (the r2 requirement), a credential header
// name, a malformed token, and a base that is not a token-free loopback
// http URL. The v1 positive control is TestLifecycleAgainstV1Service.
func TestRegisterReplyValidation(t *testing.T) {
	mutate := func(from, to string) string { return strings.Replace(registerOK, from, to, 1) }
	cases := map[string]string{
		"no token_header":       mutate(`"token_header":"X-Netproxy-Token",`, ``),
		"null token_header":     mutate(`"X-Netproxy-Token"`, `null`),
		"authorization header":  mutate(`"X-Netproxy-Token"`, `"Authorization"`),
		"header with space":     mutate(`"X-Netproxy-Token"`, `"X Netproxy"`),
		"short token":           mutate(testToken, testToken[:63]),
		"uppercase token":       mutate(testToken, strings.ToUpper(testToken)),
		"token in a base":       mutate(`/anthropic"`, `/anthropic/`+testToken+`"`),
		"https base":            mutate(`http://127.0.0.1:47811/openai/v1`, `https://127.0.0.1:47811/openai/v1`),
		"remote base":           mutate(`http://127.0.0.1:47811/chatgpt`, `http://10.0.0.1:47811/chatgpt`),
		"hostname base":         mutate(`http://127.0.0.1:47811/anthropic`, `http://localhost:47811/anthropic`),
		"base with query":       mutate(`/anthropic"`, `/anthropic?k=v"`),
		"base with userinfo":    mutate(`http://127.0.0.1:47811/anthropic`, `http://u:p@127.0.0.1:47811/anthropic`),
		"no activation_seconds": mutate(`,"activation_seconds":120`, ``),
		"zero activation":       mutate(`"activation_seconds":120`, `"activation_seconds":0`),
		"string activation":     mutate(`"activation_seconds":120`, `"activation_seconds":"120"`),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t, answer(200, body))
			reg, err := New(f.socket).Register(context.Background(), validRequest())
			if kindOf(err) != Unreachable || reg.Token != "" {
				t.Fatalf("reg = %v err = %v", reg, err)
			}
			if strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), strings.ToUpper(testToken)) {
				t.Fatal("error renders the token")
			}
		})
	}
}

// A register request that breaks the member rules is refused before any
// byte reaches the service; the valid request is the control.
func TestRegisterRequestRulesBeforeCall(t *testing.T) {
	bad := []RegisterRequest{
		{Root: "relative/p", Session: "s", Runtime: "codex"},
		{Root: "/p/", Session: "s", Runtime: "codex"},
		{Root: "/p/./q", Session: "s", Runtime: "codex"},
		{Root: "/p/../q", Session: "s", Runtime: "codex"},
		{Root: "/p//q", Session: "s", Runtime: "codex"},
		{Root: "/" + strings.Repeat("a", 200), Session: "s", Runtime: "codex"},
		{Root: "/p\x00q", Session: "s", Runtime: "codex"},
		{Root: "/p", Session: "", Runtime: "codex"},
		{Root: "/p", Session: "a b", Runtime: "codex"},
		{Root: "/p", Session: strings.Repeat("s", 129), Runtime: "codex"},
		{Root: "/p", Session: "s", Runtime: "Codex"},
		{Root: "/p", Session: "s", Runtime: "-codex"},
		{Root: "/p", Session: "s", Runtime: ""},
	}
	f := newFake(t, v1)
	c := New(f.socket)
	for _, req := range bad {
		if _, err := c.Register(context.Background(), req); kindOf(err) != Unreachable {
			t.Fatalf("%q admitted: %v", req.Root, err)
		}
	}
	if n := len(f.requests()); n != 0 {
		t.Fatalf("%d invalid requests reached the service", n)
	}
	for _, req := range []RegisterRequest{validRequest(), {Root: "/", Session: "~", Runtime: "claude-code"}, {Root: "/" + strings.Repeat("a", 199), Session: strings.Repeat("s", 128), Runtime: "a"}} {
		if _, err := c.Register(context.Background(), req); err != nil {
			t.Fatalf("%+v refused: %v", req, err)
		}
	}
}

// Conformance 12, 14, 19, 21 from the client side: unregister 404 is done,
// unregister 503 persist_failed is not, renew 409 and 404 keep their codes,
// and activate 404 is a refusal.
func TestTokenCallOutcomes(t *testing.T) {
	ctx := context.Background()
	gone := answer(404, `{"api":"netproxy-control/1","error":"unknown_token"}`)
	if err := New(newFake(t, gone).socket).Unregister(ctx, testToken); err != nil {
		t.Fatalf("unregister 404 = %v, want done", err)
	}
	persist := answer(503, `{"api":"netproxy-control/1","error":"persist_failed"}`)
	if err := New(newFake(t, persist).socket).Unregister(ctx, testToken); !IsCode(err, 503, "persist_failed") {
		t.Fatalf("unregister 503 = %v", err)
	}
	pending := answer(409, `{"api":"netproxy-control/1","error":"not_active"}`)
	if err := New(newFake(t, pending).socket).Renew(ctx, testToken); !IsCode(err, 409, "not_active") {
		t.Fatalf("renew 409 = %v", err)
	}
	if err := New(newFake(t, gone).socket).Renew(ctx, testToken); !IsCode(err, 404, "unknown_token") {
		t.Fatalf("renew 404 = %v", err)
	}
	if err := New(newFake(t, gone).socket).Activate(ctx, testToken); !IsCode(err, 404, "unknown_token") {
		t.Fatalf("activate 404 = %v", err)
	}
	inactive := answer(200, `{"api":"netproxy-control/1","active":false}`)
	if err := New(newFake(t, inactive).socket).Activate(ctx, testToken); kindOf(err) != Unreachable {
		t.Fatalf("activate active=false = %v", err)
	}
	noisy := func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.Header().Set(VersionHeader, APIVersion)
		w.WriteHeader(204)
	}
	if err := New(newFake(t, noisy).socket).Unregister(ctx, testToken); err != nil {
		t.Fatalf("unregister 204 = %v", err)
	}
	if err := New(newFake(t, v1).socket).Activate(ctx, "not-a-token"); kindOf(err) != Unreachable {
		t.Fatalf("malformed token sent: %v", err)
	}
}

// Conformance 23 for this package: neither errors nor the registration's
// string form render the token, on success or on any failure path.
func TestTokenNeverRendered(t *testing.T) {
	reg := Registration{Token: testToken, TokenHeader: "X-Netproxy-Token", ActivationSeconds: 120}
	if strings.Contains(reg.String(), testToken) {
		t.Fatal("String renders the token")
	}
	// A reflected token, whole or as a 32-character piece that fits the code
	// length, never becomes the rendered error code.
	for _, reflected := range []string{testToken, testToken[:32]} {
		echo := answer(400, `{"api":"netproxy-control/1","error":"`+reflected+`"}`)
		err := New(newFake(t, echo).socket).Activate(context.Background(), testToken)
		if err == nil || strings.Contains(err.Error(), reflected) || !IsCode(err, 400, "unknown") {
			t.Fatalf("err = %v", err)
		}
	}
	down := New(filepath.Join(t.TempDir(), "missing.sock")).Unregister(context.Background(), testToken)
	if down == nil || strings.Contains(down.Error(), testToken) {
		t.Fatalf("err = %v", down)
	}
}
