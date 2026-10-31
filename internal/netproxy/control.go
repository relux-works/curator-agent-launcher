// Package netproxy is the launcher's client for the network-proxy control
// API, netproxy-control/1: health, register, activate, renew and unregister
// over the service's unix socket. It holds the launcher side of the frozen
// contract and nothing else: every reply must carry the exact version
// header, replies are bounded, request bodies hold exactly the listed string
// members, and register reply fields are copied as given, never assembled.
//
// The launch token is a capability. No error, diagnostic or String method of
// this package renders it; callers carry it only into a launch-private
// environment or a 0600 launch-private file.
package netproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIVersion is the only control API this client speaks. A reply with
// another value, or none, is treated as the service being down.
const APIVersion = "netproxy-control/1"

// VersionHeader names the response header that carries APIVersion.
const VersionHeader = "Netproxy-Control-Api"

// MaxReply bounds every reply body the client reads. The largest v1 reply,
// register, is well under it; a longer body is a protocol error.
const MaxReply = 4 << 10

// DefaultTimeout bounds one call, inside the contract's 5–10 s range.
const DefaultTimeout = 8 * time.Second

// Kind classifies a failed call for the launcher's refusal mapping.
type Kind int

const (
	// Unreachable is "service down": no socket, a refused or lost
	// connection, a timeout, a missing or foreign version header, or a
	// reply that does not decode. The outcome of a mutating call is
	// unknown.
	Unreachable Kind = iota + 1
	// Refused is a well-formed v1 failure reply carrying an error code.
	Refused
)

// Error is a failed control call. It names the operation, the HTTP status
// and the service's error code, never a request or reply body.
type Error struct {
	Op     string
	Kind   Kind
	Status int
	Code   string
	Cause  error
}

func (e *Error) Error() string {
	switch e.Kind {
	case Refused:
		return fmt.Sprintf("netproxy %s refused: status=%d error=%s", e.Op, e.Status, e.Code)
	default:
		if e.Cause != nil {
			return fmt.Sprintf("netproxy %s: service unreachable: %v", e.Op, e.Cause)
		}
		return fmt.Sprintf("netproxy %s: service unreachable", e.Op)
	}
}

func (e *Error) Unwrap() error { return e.Cause }

// IsCode reports whether err is a Refused reply with the given status and
// error code.
func IsCode(err error, status int, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == Refused && e.Status == status && e.Code == code
}

// Client calls one service's control socket.
type Client struct {
	Socket  string
	Timeout time.Duration
	http    *http.Client
}

// New returns a client for the socket at path.
func New(socket string) *Client {
	c := &Client{Socket: socket, Timeout: DefaultTimeout}
	c.http = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", c.Socket)
			},
			DisableKeepAlives: true,
		},
		// The control API never redirects; following one would leave the socket.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return c
}

// Health is GET /v1/health. Only 200 with "ready": true is ready; anything
// else, a 503 store_unavailable or persist_failed included, is an error.
func (c *Client) Health(ctx context.Context) error {
	status, body, err := c.call(ctx, "health", http.MethodGet, "/v1/health", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return refused("health", status, body)
	}
	var reply struct {
		Ready *bool `json:"ready"`
	}
	if err := json.Unmarshal(body, &reply); err != nil || reply.Ready == nil {
		return &Error{Op: "health", Kind: Unreachable, Cause: errors.New("malformed health reply")}
	}
	if !*reply.Ready {
		return &Error{Op: "health", Kind: Unreachable, Cause: errors.New("service not ready")}
	}
	return nil
}

// RegisterRequest is the register body. Validate checks it against the
// contract before any call, so a launch that cannot register refuses
// without reaching the service.
type RegisterRequest struct {
	Root    string
	Session string
	Runtime string
}

// Validate applies the contract's register member rules.
func (r RegisterRequest) Validate() error {
	if err := validRoot(r.Root); err != nil {
		return err
	}
	if n := len(r.Session); n < 1 || n > 128 {
		return errors.New("session id must be 1 to 128 characters")
	}
	for i := 0; i < len(r.Session); i++ {
		if r.Session[i] <= ' ' || r.Session[i] > '~' {
			return errors.New("session id must be printable ASCII without spaces")
		}
	}
	if !validRuntime(r.Runtime) {
		return errors.New("runtime name must match [a-z0-9][a-z0-9-]{0,31}")
	}
	return nil
}

// Registration is a validated register reply. Token is the launch
// capability; String and the error paths never render it.
type Registration struct {
	Token             string
	TokenHeader       string
	AnthropicBase     string
	OpenAIBase        string
	ChatGPTBase       string
	ActivationSeconds int
}

// String names the registration without its token.
func (r Registration) String() string {
	return fmt.Sprintf("netproxy registration (header %s, activation %ds)", r.TokenHeader, r.ActivationSeconds)
}

// Register is POST /v1/register. A reply without token_header, with a
// malformed token, or with a base URL that is not a token-free loopback
// http URL is refused as a protocol error, so no runtime starts with it.
func (c *Client) Register(ctx context.Context, req RegisterRequest) (Registration, error) {
	if err := req.Validate(); err != nil {
		return Registration{}, &Error{Op: "register", Kind: Unreachable, Cause: fmt.Errorf("request not sent: %w", err)}
	}
	body, err := encodeRegister(req)
	if err != nil {
		return Registration{}, &Error{Op: "register", Kind: Unreachable, Cause: err}
	}
	status, reply, err := c.call(ctx, "register", http.MethodPost, "/v1/register", body)
	if err != nil {
		return Registration{}, err
	}
	if status != http.StatusOK {
		return Registration{}, refused("register", status, reply)
	}
	var raw struct {
		Token             *string `json:"token"`
		TokenHeader       *string `json:"token_header"`
		AnthropicBase     *string `json:"anthropic_base"`
		OpenAIBase        *string `json:"openai_base"`
		ChatGPTBase       *string `json:"chatgpt_base"`
		ActivationSeconds *int    `json:"activation_seconds"`
	}
	if err := json.Unmarshal(reply, &raw); err != nil {
		return Registration{}, protocol("register", "malformed register reply")
	}
	if raw.Token == nil || raw.TokenHeader == nil || raw.AnthropicBase == nil || raw.OpenAIBase == nil || raw.ChatGPTBase == nil || raw.ActivationSeconds == nil {
		return Registration{}, protocol("register", "register reply misses a required field")
	}
	reg := Registration{
		Token:             *raw.Token,
		TokenHeader:       *raw.TokenHeader,
		AnthropicBase:     *raw.AnthropicBase,
		OpenAIBase:        *raw.OpenAIBase,
		ChatGPTBase:       *raw.ChatGPTBase,
		ActivationSeconds: *raw.ActivationSeconds,
	}
	if !validToken(reg.Token) {
		return Registration{}, protocol("register", "register token is not 64 lowercase hex characters")
	}
	if !validHeaderName(reg.TokenHeader) {
		return Registration{}, protocol("register", "register token_header is not a usable header name")
	}
	for _, base := range []string{reg.AnthropicBase, reg.OpenAIBase, reg.ChatGPTBase} {
		if err := validBase(base, reg.Token); err != nil {
			return Registration{}, protocol("register", err.Error())
		}
	}
	if reg.ActivationSeconds < 1 {
		return Registration{}, protocol("register", "register activation_seconds is not positive")
	}
	return reg, nil
}

// Activate is POST /v1/activate: 200 {"active": true}, or an error.
func (c *Client) Activate(ctx context.Context, token string) error {
	return c.tokenCall(ctx, "activate", "/v1/activate", token, http.StatusOK)
}

// Renew is POST /v1/renew: 200 {"active": true}, or an error. 409
// not_active means the token is still pending; 404 unknown_token means the
// session is over.
func (c *Client) Renew(ctx context.Context, token string) error {
	return c.tokenCall(ctx, "renew", "/v1/renew", token, http.StatusOK)
}

// Unregister is POST /v1/unregister. Both 204 and 404 unknown_token mean
// the session has ended, so both return nil; any other outcome is an error
// whose token state is unknown.
func (c *Client) Unregister(ctx context.Context, token string) error {
	err := c.tokenCall(ctx, "unregister", "/v1/unregister", token, http.StatusNoContent)
	if IsCode(err, http.StatusNotFound, "unknown_token") {
		return nil
	}
	return err
}

func (c *Client) tokenCall(ctx context.Context, op, path, token string, want int) error {
	if !validToken(token) {
		return &Error{Op: op, Kind: Unreachable, Cause: errors.New("request not sent: malformed token")}
	}
	body, err := encodeToken(token)
	if err != nil {
		return &Error{Op: op, Kind: Unreachable, Cause: err}
	}
	status, reply, err := c.call(ctx, op, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if status != want {
		return refused(op, status, reply)
	}
	if want == http.StatusNoContent {
		if len(reply) != 0 {
			return protocol(op, "non-empty 204 reply")
		}
		return nil
	}
	var active struct {
		Active *bool `json:"active"`
	}
	if err := json.Unmarshal(reply, &active); err != nil || active.Active == nil || !*active.Active {
		return protocol(op, "reply is not {\"active\": true}")
	}
	return nil
}

// call performs one bounded request and returns the status and a body of
// at most MaxReply bytes. Every failure before a version-checked reply is
// Unreachable.
func (c *Client) call(ctx context.Context, op, method, path string, body []byte) (int, []byte, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://control"+path, rd)
	if err != nil {
		return 0, nil, &Error{Op: op, Kind: Unreachable, Cause: err}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, &Error{Op: op, Kind: Unreachable, Cause: scrub(err)}
	}
	defer resp.Body.Close()
	if got := resp.Header.Values(VersionHeader); len(got) != 1 || got[0] != APIVersion {
		return 0, nil, &Error{Op: op, Kind: Unreachable, Cause: errors.New("missing or unsupported control API version")}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxReply+1))
	if err != nil {
		return 0, nil, &Error{Op: op, Kind: Unreachable, Cause: errors.New("reply not read")}
	}
	if len(data) > MaxReply {
		return 0, nil, &Error{Op: op, Kind: Unreachable, Cause: errors.New("reply exceeds bound")}
	}
	if len(data) > 0 {
		var env struct {
			API *string `json:"api"`
		}
		if json.Unmarshal(data, &env) != nil || env.API == nil || *env.API != APIVersion {
			return 0, nil, &Error{Op: op, Kind: Unreachable, Cause: errors.New("reply body lacks the control API version")}
		}
	}
	return resp.StatusCode, data, nil
}

func refused(op string, status int, body []byte) error {
	var reply struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &reply)
	code := reply.Error
	if !validCode(code) {
		code = "unknown"
	}
	return &Error{Op: op, Kind: Refused, Status: status, Code: code}
}

func protocol(op, detail string) error {
	return &Error{Op: op, Kind: Unreachable, Cause: errors.New(detail)}
}

// scrub drops transport detail that could carry request content; the URL
// is the fixed "http://control/..." form, so only the class remains.
func scrub(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("timeout")
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errors.New("timeout")
	}
	return errors.New("connection failed")
}

// encodeRegister writes exactly {"root","session","runtime"}, all strings,
// in that order.
func encodeRegister(r RegisterRequest) ([]byte, error) {
	return json.Marshal(struct {
		Root    string `json:"root"`
		Session string `json:"session"`
		Runtime string `json:"runtime"`
	}{r.Root, r.Session, r.Runtime})
}

// encodeToken writes exactly {"token"}.
func encodeToken(token string) ([]byte, error) {
	return json.Marshal(struct {
		Token string `json:"token"`
	}{token})
}

func validRoot(root string) error {
	switch {
	case root == "" || root[0] != '/':
		return errors.New("project root must be absolute")
	case len(root) > 200:
		return errors.New("project root exceeds 200 bytes")
	case strings.IndexByte(root, 0) >= 0:
		return errors.New("project root contains NUL")
	case root != "/" && strings.HasSuffix(root, "/"):
		return errors.New("project root has a trailing slash")
	}
	for _, part := range strings.Split(root[1:], "/") {
		if root != "/" && (part == "" || part == "." || part == "..") {
			return errors.New("project root is not clean")
		}
	}
	return nil
}

func validRuntime(s string) bool {
	if len(s) < 1 || len(s) > 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		lower := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if !(lower || i > 0 && c == '-') {
			return false
		}
	}
	return true
}

func validToken(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune("0123456789abcdef", rune(s[i])) {
			return false
		}
	}
	return true
}

// validHeaderName admits an RFC 9110 field name that is not a credential,
// hop or framing header: the token never travels as a provider credential.
func validHeaderName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0) {
			return false
		}
	}
	switch strings.ToLower(s) {
	case "authorization", "proxy-authorization", "cookie", "host", "content-length", "transfer-encoding", "connection", "x-api-key":
		return false
	}
	return true
}

// validBase admits an http URL on a loopback IP with no credentials, query,
// fragment or token anywhere in it.
func validBase(base, token string) error {
	if strings.Contains(base, token) {
		return errors.New("register base URL carries the token")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errors.New("register base URL is not a plain http URL")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() || u.Port() == "" {
		return errors.New("register base URL is not a loopback IP with a port")
	}
	return nil
}

// validCode admits the v1 error code shape. Codes hold no digits, so a
// reflected 64-hex token can never pass as a code and reach a diagnostic.
func validCode(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c == '_') {
			return false
		}
	}
	return true
}
