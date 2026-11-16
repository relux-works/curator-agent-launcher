package m0adapter

import (
	"crypto/sha256"
	"encoding/hex"
)

// Closed discriminators and constants of the DRAFT grammar (spec §2).
const (
	entryKind      = "swarma-dispatcher m0-executor-entry/1"
	requestKind    = "swarma-dispatcher m0-request/1"
	requestOp      = "dispatch"
	inputPlacement = "station-one-shot-v1"
	hashPrefix     = "sha256:"
	keyPrefix      = "ed25519:"
	executionIDPfx = "EX-"
	uidMin         = 30000
	uidMax         = 59999
)

// Entry keeps only the bounded decoded values needed for mapping and audit.
// Every other member is validated for shape and then discarded: the adapter
// holds no protected authority, and shape success grants none.
type Entry struct {
	ExecutionID     string
	DeadlineMs      int64
	SlackMs         int64
	Account         string
	Profile         string
	Model           string
	ReasoningEffort string
	Prompt          []byte

	// Audit identities: "sha256:" + hex(SHA-256(CCJ-1(...))).
	EntryDigest string
	InputDigest string

	// Bounded work observed while decoding.
	EncodedBytes        int
	CanonicalBytes      int
	InputCanonicalBytes int
	Tokens              int
	Objects             int
	Members             int
}

// Decode applies the complete closed grammar to the raw FD3 bytes: lexical
// rules, finite bounds, the 9-object/43-member closed table, exact value
// shapes, CCJ-1 within its bound and the worker_plan consistency check.
// It reads no file, path, environment or protected metadata.
func Decode(raw []byte) (*Entry, error) {
	p, root, err := parse(raw)
	if err != nil {
		return nil, err
	}
	e, workerPlan, err := p.shape(root)
	if err != nil {
		return nil, err
	}
	c, err := canonicalize(p, root, maxCanonical)
	if err != nil {
		return nil, err
	}
	if c.inputStart < 0 {
		return nil, refuse(ReasonInternal)
	}
	entrySum := sha256.Sum256(c.out)
	inputSum := sha256.Sum256(c.out[c.inputStart:c.inputEnd])
	e.EntryDigest = hashPrefix + hex.EncodeToString(entrySum[:])
	e.InputDigest = hashPrefix + hex.EncodeToString(inputSum[:])
	// request.plan.worker_plan = HASH(CCJ-1(input)) is a consistency check
	// only (spec §3); D's protected equality is the authority.
	if workerPlan != e.InputDigest {
		return nil, refuse(ReasonEntryWorkerPlan)
	}
	e.EncodedBytes = len(raw)
	e.CanonicalBytes = len(c.out)
	e.InputCanonicalBytes = c.inputEnd - c.inputStart
	e.Tokens = p.tokens
	e.Objects = len(p.objects)
	e.Members = p.members
	return e, nil
}

// closed returns the object at v if it has exactly the named members. With
// duplicates already refused, "no unknown name" plus "equal count" means
// every required member is present exactly once.
func (p *parser) closed(v jvalue, names ...string) (*jobject, error) {
	if v.kind != kindObject {
		return nil, refuse(ReasonEntryType)
	}
	o := &p.objects[v.obj]
	for _, m := range o.members {
		known := false
		for _, n := range names {
			if m.name == n {
				known = true
				break
			}
		}
		if !known {
			return nil, refuse(ReasonEntryUnknown)
		}
	}
	if len(o.members) != len(names) {
		return nil, refuse(ReasonEntryMissing)
	}
	return o, nil
}

func member(o *jobject, name string) jvalue {
	for _, m := range o.members {
		if m.name == name {
			return m.val
		}
	}
	return jvalue{}
}

// text returns a non-prompt string member, bounded by maxStringBytes.
func text(o *jobject, name string) (string, error) {
	v := member(o, name)
	if v.kind != kindString {
		return "", refuse(ReasonEntryType)
	}
	if len(v.str) > maxStringBytes {
		return "", refuse(ReasonEntryStringLength)
	}
	return v.str, nil
}

func integer(o *jobject, name string) (int64, error) {
	v := member(o, name)
	if v.kind != kindInt {
		return 0, refuse(ReasonEntryType)
	}
	return v.num, nil
}

func object(p *parser, o *jobject, name string, names ...string) (*jobject, error) {
	return p.closed(member(o, name), names...)
}

// checked runs one value read plus one shape predicate.
func checkedText(o *jobject, name string, ok func(string) bool, r Reason) (string, error) {
	s, err := text(o, name)
	if err != nil {
		return "", err
	}
	if !ok(s) {
		return "", refuse(r)
	}
	return s, nil
}

func checkedInt(o *jobject, name string, lo, hi int64, r Reason) (int64, error) {
	n, err := integer(o, name)
	if err != nil {
		return 0, err
	}
	if n < lo || n > hi {
		return 0, refuse(r)
	}
	return n, nil
}

func constText(o *jobject, name, want string) error {
	s, err := text(o, name)
	if err != nil {
		return err
	}
	if s != want {
		return refuse(ReasonEntryConstant)
	}
	return nil
}

func constOne(o *jobject, name string) error {
	n, err := integer(o, name)
	if err != nil {
		return err
	}
	if n != 1 {
		return refuse(ReasonEntryConstant)
	}
	return nil
}

func isID(s string) bool { return len(s) == 32 && lowerHexOnly(s) }

func isHash(s string) bool {
	return len(s) == len(hashPrefix)+64 && s[:len(hashPrefix)] == hashPrefix && lowerHexOnly(s[len(hashPrefix):])
}

func isExecutionID(s string) bool {
	return len(s) == len(executionIDPfx)+32 && s[:len(executionIDPfx)] == executionIDPfx && lowerHexOnly(s[len(executionIDPfx):])
}

func lowerHexOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// isKey is "ed25519:" + 42 base64url characters + one final character whose
// low four bits are zero, i.e. the canonical unpadded spelling of 32 bytes
// (signing-plan schema pattern ^ed25519:[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$).
func isKey(s string) bool {
	if len(s) != len(keyPrefix)+43 || s[:len(keyPrefix)] != keyPrefix {
		return false
	}
	body := s[len(keyPrefix):]
	for i := 0; i < 42; i++ {
		c := body[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	switch body[42] {
	case 'A', 'E', 'I', 'M', 'Q', 'U', 'Y', 'c', 'g', 'k', 'o', 's', 'w', '0', '4', '8':
		return true
	}
	return false
}

// isSelector is 1..64 ASCII bytes: first [a-z0-9], then [a-z0-9_-].
func isSelector(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if i > 0 {
			ok = ok || c == '_' || c == '-'
		}
		if !ok {
			return false
		}
	}
	return true
}

// isCatalogString is 1..128 printable ASCII bytes U+0021..U+007E, first byte
// not '-'. Equality with the qualified catalog is D's check, not this one.
func isCatalogString(s string) bool {
	return printableASCII(s, maxStringBytes) && s[0] != '-'
}

// shape applies the closed member table of spec §2 in table order.
func (p *parser) shape(root jvalue) (*Entry, string, error) {
	e := &Entry{}
	top, err := p.closed(root, "v", "kind", "request", "execution", "catalog", "qualification", "timing", "input")
	if err != nil {
		return nil, "", err
	}
	if err := constOne(top, "v"); err != nil {
		return nil, "", err
	}
	if err := constText(top, "kind", entryKind); err != nil {
		return nil, "", err
	}

	req, err := object(p, top, "request", "v", "kind", "request_id", "receiver", "policy", "association", "worker", "plan", "op")
	if err != nil {
		return nil, "", err
	}
	if err := constOne(req, "v"); err != nil {
		return nil, "", err
	}
	if err := constText(req, "kind", requestKind); err != nil {
		return nil, "", err
	}
	for _, n := range []string{"request_id", "receiver"} {
		if _, err := checkedText(req, n, isID, ReasonEntryID); err != nil {
			return nil, "", err
		}
	}
	pol, err := object(p, req, "policy", "id", "revision")
	if err != nil {
		return nil, "", err
	}
	if _, err := checkedText(pol, "id", isID, ReasonEntryID); err != nil {
		return nil, "", err
	}
	if _, err := checkedInt(pol, "revision", 1, maxSafeInt, ReasonEntrySerial); err != nil {
		return nil, "", err
	}
	assoc, err := object(p, req, "association", "id", "revision", "uid", "generation", "key", "registration_number", "master_epoch")
	if err != nil {
		return nil, "", err
	}
	if _, err := checkedText(assoc, "id", isID, ReasonEntryID); err != nil {
		return nil, "", err
	}
	if _, err := checkedInt(assoc, "revision", 1, maxSafeInt, ReasonEntrySerial); err != nil {
		return nil, "", err
	}
	if _, err := checkedInt(assoc, "uid", uidMin, uidMax, ReasonEntryUID); err != nil {
		return nil, "", err
	}
	if _, err := checkedText(assoc, "generation", isID, ReasonEntryID); err != nil {
		return nil, "", err
	}
	if _, err := checkedText(assoc, "key", isKey, ReasonEntryKey); err != nil {
		return nil, "", err
	}
	for _, n := range []string{"registration_number", "master_epoch"} {
		if _, err := checkedInt(assoc, n, 1, maxSafeInt, ReasonEntrySerial); err != nil {
			return nil, "", err
		}
	}
	wrk, err := object(p, req, "worker", "uid", "generation")
	if err != nil {
		return nil, "", err
	}
	if _, err := checkedInt(wrk, "uid", uidMin, uidMax, ReasonEntryUID); err != nil {
		return nil, "", err
	}
	if _, err := checkedText(wrk, "generation", isID, ReasonEntryID); err != nil {
		return nil, "", err
	}
	plan, err := object(p, req, "plan", "worker_plan", "profile", "harness", "exec_profile")
	if err != nil {
		return nil, "", err
	}
	workerPlan, err := checkedText(plan, "worker_plan", isHash, ReasonEntryHash)
	if err != nil {
		return nil, "", err
	}
	for _, n := range []string{"profile", "harness", "exec_profile"} {
		if _, err := checkedText(plan, n, isHash, ReasonEntryHash); err != nil {
			return nil, "", err
		}
	}
	if err := constText(req, "op", requestOp); err != nil {
		return nil, "", err
	}

	exe, err := object(p, top, "execution", "effect_id", "execution_id", "controller_epoch")
	if err != nil {
		return nil, "", err
	}
	if _, err := checkedText(exe, "effect_id", isID, ReasonEntryID); err != nil {
		return nil, "", err
	}
	if e.ExecutionID, err = checkedText(exe, "execution_id", isExecutionID, ReasonEntryExecutionID); err != nil {
		return nil, "", err
	}
	if _, err := checkedInt(exe, "controller_epoch", 1, maxSafeInt, ReasonEntrySerial); err != nil {
		return nil, "", err
	}

	for _, n := range []string{"catalog", "qualification"} {
		if _, err := checkedText(top, n, isHash, ReasonEntryHash); err != nil {
			return nil, "", err
		}
	}

	tim, err := object(p, top, "timing", "deadline_ms", "slack_ms")
	if err != nil {
		return nil, "", err
	}
	if e.DeadlineMs, err = checkedInt(tim, "deadline_ms", 1, maxSafeInt, ReasonEntrySerial); err != nil {
		return nil, "", err
	}
	if e.SlackMs, err = checkedInt(tim, "slack_ms", 1, maxSafeInt, ReasonEntrySerial); err != nil {
		return nil, "", err
	}

	in, err := object(p, top, "input", "account", "profile", "model", "reasoning_effort", "placement", "prompt")
	if err != nil {
		return nil, "", err
	}
	if e.Account, err = checkedText(in, "account", isSelector, ReasonEntrySelector); err != nil {
		return nil, "", err
	}
	if e.Profile, err = checkedText(in, "profile", isSelector, ReasonEntrySelector); err != nil {
		return nil, "", err
	}
	if e.Model, err = checkedText(in, "model", isCatalogString, ReasonEntryCatalogString); err != nil {
		return nil, "", err
	}
	if e.ReasoningEffort, err = checkedText(in, "reasoning_effort", isCatalogString, ReasonEntryCatalogString); err != nil {
		return nil, "", err
	}
	if err := constText(in, "placement", inputPlacement); err != nil {
		return nil, "", err
	}
	pv := member(in, "prompt")
	if pv.kind != kindString {
		return nil, "", refuse(ReasonEntryType)
	}
	if len(pv.str) < 1 || len(pv.str) > maxPromptBytes {
		return nil, "", refuse(ReasonEntryPromptLength)
	}
	for i := 0; i < len(pv.str); i++ {
		if pv.str[i] == 0 {
			return nil, "", refuse(ReasonEntryPromptNUL)
		}
	}
	e.Prompt = []byte(pv.str)
	return e, workerPlan, nil
}
