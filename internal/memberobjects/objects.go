// Package memberobjects observes an untrusted flat content-addressed collection.
// It implements neither six-descriptor validation nor role policy or admission.
package memberobjects

import (
	"os"
	"sort"
	"sync"
)

const (
	NameLimit   = 128
	ObjectLimit = 4 << 20
	TotalLimit  = 16 << 20
)

// Reference is untrusted data, not permission to consume an object or execute.
// Length is unsigned; syntax and all caps are checked before any descriptor use.
type Reference struct {
	Name   string
	Length uint64
	SHA256 string
}

// Error contains only a static protocol tag. Root contract failures map to
// bootstrap/descriptor_invalid; declarations, collection/file/hash/change and
// closed/failed custody to admission/input_mismatch; explicit caps to
// parse/bounds_exceeded. No OS error or input text is returned or logged.
type Error struct{ stage, code string }

func (e *Error) Error() string { return e.stage + "/" + e.code }
func (e *Error) Stage() string { return e.stage }
func (e *Error) Code() string  { return e.code }
func descriptorError() *Error  { return &Error{"bootstrap", "descriptor_invalid"} }
func mismatchError() *Error    { return &Error{"admission", "input_mismatch"} }
func boundsError() *Error      { return &Error{"parse", "bounds_exceeded"} }

// Snapshot is only an immutable byte observation, never live descriptor custody.
// Copies obtained before Close/refusal remain observations, not usable handles.
type Snapshot struct {
	raw    []byte
	digest [32]byte
}

func (s *Snapshot) Raw() []byte      { return append([]byte(nil), s.raw...) }
func (s *Snapshot) Length() uint64   { return uint64(len(s.raw)) }
func (s *Snapshot) SHA256() [32]byte { return s.digest }

// Collection owns hidden read-only CLOEXEC handles and bounded original bytes.
// Do not copy a Collection. Close it explicitly; there is no collection finalizer.
// Returned byte/reference copies are caller allocations, outside retained caps.
type Collection struct {
	mu        sync.Mutex
	refs      []Reference
	snapshots map[string]*Snapshot
	custody   *retained
	closed    bool
	closeErr  *Error
}

// Acquire reads only an explicit already-open directory and validated digest
// basenames relative to it. Empty refs require an exactly empty directory. Names
// are logical identifiers, never paths. Aliases with equal digest/length retain
// one object and count its bytes once. No instruction/role policy is applied.
//
// Darwin/Linux require current UID, exact0700 readonly O, and current-UID,
// single-link regular0400/0600 objects. Owned handles are CLOEXEC; caller handles,
// offsets and flags are untouched. Every acquisition failure closes all owned
// descriptors and returns no collection. Other OSes refuse without input access.
//
// Stable caller handles and cooperative frozen directory/files, with no other
// writer or mapping, are PRECONDITIONS. Native before/after observations cannot
// prove hostile same-UID exclusion, a coherent concurrent snapshot, every
// transient same-size write or future freeze. Reads may change atime; no disk
// deadline is promised. No path/HOME/env discovery, content policy or materializing.
// Later entry integration must validate ALL six descriptors before any content.
func Acquire(directory *os.File, refs []Reference) (*Collection, *Error) {
	return acquirePlatform(directory, refs)
}

// References returns an independent projection copy only while custody is live.
func (c *Collection) References() ([]Reference, *Error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.custody == nil {
		return nil, mismatchError()
	}
	return append([]Reference(nil), c.refs...), nil
}

// Snapshot returns the original immutable observation, not a refreshed source.
// Immediately before each admitted consuming phase the caller must Recheck,
// retaining this Collection through that use and explicitly Close afterwards.
// Recheck is an observation, not a watch/fence or authority to admit that use.
func (c *Collection) Snapshot(name string) (*Snapshot, *Error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.custody == nil {
		return nil, mismatchError()
	}
	for _, r := range c.refs {
		if r.Name == name {
			s := c.snapshots[r.SHA256]
			// The caller can overwrite an exported Snapshot value wholesale; never
			// expose the retained struct itself, even though its fields are private.
			return &Snapshot{raw: s.raw, digest: s.digest}, nil
		}
	}
	return nil, mismatchError()
}

// Recheck checks retained handles, original bytes/hash/metadata, directory-entry
// identities and the exact flat set. It never replaces original snapshots or
// repairs/retries input. ANY refusal invalidates custody and closes all owned
// handles; prior Snapshot values remain only immutable observations.
func (c *Collection) Recheck() *Error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.custody == nil {
		return mismatchError()
	}
	if err := c.custody.recheck(c.snapshots); err != nil {
		c.closed = true
		c.closeErr = c.custody.close()
		return err
	}
	return nil
}

// Close attempts every owned close once, even after an earlier close fails.
// It invalidates custody BEFORE closing and never retries a descriptor number
// (including EINTR/ambiguous close), avoiding accidental closure after FD reuse.
// A partial-close error is static input_mismatch; the caller must treat custody
// as unavailable, not retry raw FDs. Repeated Close returns the first close
// result without syscalls. Caller originals are never closed.
func (c *Collection) Close() *Error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		if c.custody != nil {
			c.closeErr = c.custody.close()
		}
	}
	return c.closeErr
}

type declaration struct {
	digest string
	length uint64
}

func project(refs []Reference) ([]Reference, []declaration, *Error) {
	if len(refs) > NameLimit {
		return nil, nil, boundsError()
	}
	names := make(map[string]bool, len(refs))
	lengths := make(map[string]uint64, len(refs))
	var total uint64
	for _, r := range refs {
		if !validName(r.Name) || !validDigest(r.SHA256) || names[r.Name] {
			return nil, nil, mismatchError()
		}
		names[r.Name] = true
		if r.Length > ObjectLimit {
			return nil, nil, boundsError()
		}
		if n, ok := lengths[r.SHA256]; ok {
			if n != r.Length {
				return nil, nil, mismatchError()
			}
			continue
		}
		if r.Length > TotalLimit-total {
			return nil, nil, boundsError()
		}
		total += r.Length
		lengths[r.SHA256] = r.Length
	}
	ds := make([]declaration, 0, len(lengths))
	for digest, length := range lengths {
		ds = append(ds, declaration{digest, length})
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].digest < ds[j].digest })
	return append([]Reference(nil), refs...), ds, nil
}
func validName(s string) bool {
	if len(s) < 1 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			continue
		}
		if i == 0 || (b != '.' && b != '_' && b != '-') {
			return false
		}
	}
	return true
}
func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !(b >= '0' && b <= '9') && !(b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
