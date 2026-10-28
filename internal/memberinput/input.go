// Package memberinput takes bounded, untrusted byte observations of explicit
// request and authorization handles. It is not the six-descriptor custody gate,
// structural decoding, authorization, admission, or permission to execute.
package memberinput

import (
	"os"

	"github.com/relux-works/curator-agent-launcher/internal/memberwire"
)

const (
	RequestLimit       = memberwire.RequestLimit
	AuthorizationLimit = memberwire.AuthorizationLimit
)

// Error exposes only a closed static protocol tag, never an OS error or input.
type Error struct{ stage, code string }

func (e *Error) Error() string { return e.stage + "/" + e.code }
func (e *Error) Stage() string { return e.stage }
func (e *Error) Code() string  { return e.code }
func descriptorError() *Error  { return &Error{"bootstrap", "descriptor_invalid"} }
func boundsError() *Error      { return &Error{"parse", "bounds_exceeded"} }

// Snapshot retains the exact observed bytes, not a parsed or authorized record.
// It owns no descriptor; Raw returns a fresh copy, and SHA256 returns a value.
type Snapshot struct {
	raw    []byte
	digest [32]byte
}

func (s *Snapshot) Raw() []byte      { return append([]byte(nil), s.raw...) }
func (s *Snapshot) Length() int      { return len(s.raw) }
func (s *Snapshot) SHA256() [32]byte { return s.digest }

// UntrustedPair is all-or-nothing. Neither snapshot is a custody capability.
type UntrustedPair struct{ request, authorization *Snapshot }

func (p *UntrustedPair) Request() *Snapshot       { return p.request }
func (p *UntrustedPair) Authorization() *Snapshot { return p.authorization }

// ReadPair observes already-open N/A files on Darwin/Linux. Both must be distinct
// read-only regular files, current UID, exact mode 0600, one link, within their
// respective byte ceilings. Both metadata checks precede either content read.
// Offset-independent reads and before/after metadata checks reject observed
// short reads, growth and changes. Empty raw bytes are allowed; no JSON is parsed.
// Other OSes refuse without accessing either handle.
//
// The caller owns both handles and must keep their descriptors, identity and
// access flags stable for this call (including no raw close/dup2 behind os.File).
// Control pins the os.File lifetimes while observing; no descriptor is created,
// reopened, owned or closed here, and no finalizer is installed. Returned snapshots
// survive caller closure. Reads do not change bytes, modes or shared offsets;
// the filesystem may update access time. Disk I/O has no promised time deadline.
//
// Cooperative frozen files, no other writer/mapping, and continued custody are
// caller preconditions, not verified facts. Metadata checks cannot detect every
// same-size transient write or prove any future immutability of the source file.
// Success proves only these bounded observations, not full descriptor validation,
// binding/expiry, origin, freeze, admission, compatibility or execution support.
func ReadPair(request, authorization *os.File) (*UntrustedPair, *Error) {
	return readPairPlatform(request, authorization)
}
