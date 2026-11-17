package m0adapter

import (
	"unicode/utf8"
)

// Hard parser and work ceilings (spec §2) plus mapping ceilings (spec §4).
const (
	maxEncoded      = 49152 // encoded FD3 bytes
	maxCanonical    = 49152 // CCJ-1 bytes of the entire entry
	maxTokens       = 512   // lexical tokens incl. member names and punctuation
	maxDepth        = 8     // object nesting; stricter than upstream 16
	maxNameBytes    = 32    // each member name, ASCII
	maxStringBytes  = 128   // every non-prompt string value
	maxDecodedTotal = 32768 // decoded string values, excluding member names
	maxPromptBytes  = 24576 // decoded Prompt UTF-8 bytes
	maxSafeInt      = 1<<53 - 1
	maxPathBytes    = 1024 // each of E/H/J/L/W/O
	maxArgvElement  = 1024
	maxArgvStorage  = 8192 // sum of element bytes plus one NUL each
)

type valueKind uint8

const (
	kindString valueKind = iota + 1
	kindInt
	kindObject
)

// jvalue is one admitted JSON value. Arrays, booleans and null never reach
// this representation: the lexer refuses them.
type jvalue struct {
	kind valueKind
	str  string
	num  int64
	obj  int // index into parser.objects
}

type jmember struct {
	name string
	val  jvalue
}

type jobject struct {
	members []jmember
}

// parser is one bounded, linear pass over at most maxEncoded bytes and
// maxTokens tokens. Depth is bounded by maxDepth, so recursion is bounded.
type parser struct {
	buf     []byte
	pos     int
	tokens  int
	depth   int
	decoded int
	objects []jobject
	members int
}

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// parse applies the lexical and finite-work rules and returns the parsed
// root object. It checks raw bytes before any value allocation.
func parse(raw []byte) (*parser, jvalue, error) {
	if len(raw) > maxEncoded {
		return nil, jvalue{}, refuse(ReasonEntrySizeLimit)
	}
	if len(raw) >= 3 && raw[0] == utf8BOM[0] && raw[1] == utf8BOM[1] && raw[2] == utf8BOM[2] {
		return nil, jvalue{}, refuse(ReasonEntryBOM)
	}
	// Outside strings only ASCII is valid JSON, so whole-buffer validation is
	// exactly the string-content rule. utf8.Valid also refuses encoded
	// surrogate code points.
	if !utf8.Valid(raw) {
		return nil, jvalue{}, refuse(ReasonEntryUTF8)
	}
	p := &parser{buf: raw}
	p.skipSpace()
	if p.pos >= len(p.buf) {
		return nil, jvalue{}, refuse(ReasonEntrySyntax)
	}
	if c := p.buf[p.pos]; c != '{' {
		if c == '[' {
			return nil, jvalue{}, refuse(ReasonEntryArray)
		}
		return nil, jvalue{}, refuse(ReasonEntrySyntax)
	}
	root, err := p.value()
	if err != nil {
		return nil, jvalue{}, err
	}
	p.skipSpace()
	if p.pos != len(p.buf) {
		return nil, jvalue{}, refuse(ReasonEntryTrailing)
	}
	return p, root, nil
}

func (p *parser) skipSpace() {
	for p.pos < len(p.buf) {
		switch p.buf[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

// token counts one lexical token and refuses the 513th.
func (p *parser) token() error {
	p.tokens++
	if p.tokens > maxTokens {
		return refuse(ReasonEntryTokenLimit)
	}
	return nil
}

func (p *parser) value() (jvalue, error) {
	p.skipSpace()
	if p.pos >= len(p.buf) {
		return jvalue{}, refuse(ReasonEntrySyntax)
	}
	c := p.buf[p.pos]
	switch {
	case c == '{':
		return p.object()
	case c == '"':
		if err := p.token(); err != nil {
			return jvalue{}, err
		}
		s, err := p.str(false)
		if err != nil {
			return jvalue{}, err
		}
		return jvalue{kind: kindString, str: s}, nil
	case c == '-' || c >= '0' && c <= '9':
		if err := p.token(); err != nil {
			return jvalue{}, err
		}
		return p.number()
	case c == '[':
		if err := p.token(); err != nil {
			return jvalue{}, err
		}
		return jvalue{}, refuse(ReasonEntryArray)
	case c == 't' || c == 'f' || c == 'n':
		for _, lit := range []string{"true", "false", "null"} {
			if len(p.buf)-p.pos >= len(lit) && string(p.buf[p.pos:p.pos+len(lit)]) == lit {
				if err := p.token(); err != nil {
					return jvalue{}, err
				}
				return jvalue{}, refuse(ReasonEntryLiteral)
			}
		}
	}
	return jvalue{}, refuse(ReasonEntrySyntax)
}

func (p *parser) object() (jvalue, error) {
	if err := p.token(); err != nil { // '{'
		return jvalue{}, err
	}
	p.pos++
	p.depth++
	if p.depth > maxDepth {
		return jvalue{}, refuse(ReasonEntryDepthLimit)
	}
	idx := len(p.objects)
	p.objects = append(p.objects, jobject{})
	var members []jmember
	p.skipSpace()
	if p.pos < len(p.buf) && p.buf[p.pos] == '}' {
		if err := p.token(); err != nil {
			return jvalue{}, err
		}
		p.pos++
		p.depth--
		return jvalue{kind: kindObject, obj: idx}, nil
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.buf) || p.buf[p.pos] != '"' {
			return jvalue{}, refuse(ReasonEntrySyntax)
		}
		if err := p.token(); err != nil {
			return jvalue{}, err
		}
		name, err := p.str(true)
		if err != nil {
			return jvalue{}, err
		}
		// Names are compared after unescaping: an escaped spelling of "v"
		// duplicates a plain "v".
		for _, m := range members {
			if m.name == name {
				return jvalue{}, refuse(ReasonEntryDuplicate)
			}
		}
		p.skipSpace()
		if p.pos >= len(p.buf) || p.buf[p.pos] != ':' {
			return jvalue{}, refuse(ReasonEntrySyntax)
		}
		if err := p.token(); err != nil {
			return jvalue{}, err
		}
		p.pos++
		v, err := p.value()
		if err != nil {
			return jvalue{}, err
		}
		members = append(members, jmember{name: name, val: v})
		p.members++
		p.skipSpace()
		if p.pos >= len(p.buf) {
			return jvalue{}, refuse(ReasonEntrySyntax)
		}
		switch p.buf[p.pos] {
		case ',':
			if err := p.token(); err != nil {
				return jvalue{}, err
			}
			p.pos++
			continue
		case '}':
			if err := p.token(); err != nil {
				return jvalue{}, err
			}
			p.pos++
			p.objects[idx].members = members
			p.depth--
			return jvalue{kind: kindObject, obj: idx}, nil
		}
		return jvalue{}, refuse(ReasonEntrySyntax)
	}
}

// str decodes one JSON string starting at the opening quote. Member names
// are limited to maxNameBytes ASCII bytes; values count toward
// maxDecodedTotal. Raw control bytes, unknown escapes and lone surrogates
// refuse.
func (p *parser) str(isName bool) (string, error) {
	p.pos++ // opening quote
	var out []byte
	for {
		if p.pos >= len(p.buf) {
			return "", refuse(ReasonEntrySyntax)
		}
		c := p.buf[p.pos]
		switch {
		case c == '"':
			p.pos++
			if isName {
				for i := 0; i < len(out); i++ {
					if out[i] >= 0x80 {
						return "", refuse(ReasonEntryNameLength)
					}
				}
			} else {
				p.decoded += len(out)
			}
			return string(out), nil
		case c == '\\':
			var err error
			out, err = p.escape(out)
			if err != nil {
				return "", err
			}
		case c < 0x20:
			return "", refuse(ReasonEntrySyntax)
		default:
			out = append(out, c)
			p.pos++
		}
		if isName {
			if len(out) > maxNameBytes {
				return "", refuse(ReasonEntryNameLength)
			}
		} else if p.decoded+len(out) > maxDecodedTotal {
			return "", refuse(ReasonEntryStringTotal)
		}
	}
}

// escape decodes one backslash escape at p.pos and appends its UTF-8 bytes.
func (p *parser) escape(out []byte) ([]byte, error) {
	if p.pos+1 >= len(p.buf) {
		return nil, refuse(ReasonEntrySyntax)
	}
	e := p.buf[p.pos+1]
	p.pos += 2
	switch e {
	case '"', '\\', '/':
		return append(out, e), nil
	case 'b':
		return append(out, '\b'), nil
	case 'f':
		return append(out, '\f'), nil
	case 'n':
		return append(out, '\n'), nil
	case 'r':
		return append(out, '\r'), nil
	case 't':
		return append(out, '\t'), nil
	case 'u':
		r, ok := p.hex4()
		if !ok {
			return nil, refuse(ReasonEntrySyntax)
		}
		switch {
		case r >= 0xdc00 && r <= 0xdfff:
			return nil, refuse(ReasonEntrySurrogate)
		case r >= 0xd800 && r <= 0xdbff:
			if p.pos+1 >= len(p.buf) || p.buf[p.pos] != '\\' || p.buf[p.pos+1] != 'u' {
				return nil, refuse(ReasonEntrySurrogate)
			}
			p.pos += 2
			lo, ok := p.hex4()
			if !ok {
				return nil, refuse(ReasonEntrySyntax)
			}
			if lo < 0xdc00 || lo > 0xdfff {
				return nil, refuse(ReasonEntrySurrogate)
			}
			r = 0x10000 + (r-0xd800)<<10 + (lo - 0xdc00)
		}
		return utf8.AppendRune(out, rune(r)), nil
	}
	return nil, refuse(ReasonEntrySyntax)
}

func (p *parser) hex4() (uint32, bool) {
	if p.pos+4 > len(p.buf) {
		return 0, false
	}
	var r uint32
	for i := 0; i < 4; i++ {
		c := p.buf[p.pos+i]
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | uint32(d)
	}
	p.pos += 4
	return r, true
}

// number admits only CCJ-1 integers: no fraction, no exponent, no -0, no
// leading zeros, and magnitude at most 2^53-1.
func (p *parser) number() (jvalue, error) {
	neg := false
	if p.buf[p.pos] == '-' {
		neg = true
		p.pos++
	}
	start := p.pos
	if p.pos >= len(p.buf) {
		return jvalue{}, refuse(ReasonEntrySyntax)
	}
	switch c := p.buf[p.pos]; {
	case c == '0':
		p.pos++
		if p.pos < len(p.buf) && p.buf[p.pos] >= '0' && p.buf[p.pos] <= '9' {
			return jvalue{}, refuse(ReasonEntrySyntax)
		}
	case c >= '1' && c <= '9':
		for p.pos < len(p.buf) && p.buf[p.pos] >= '0' && p.buf[p.pos] <= '9' {
			p.pos++
		}
	default:
		return jvalue{}, refuse(ReasonEntrySyntax)
	}
	digits := p.buf[start:p.pos]
	if p.pos < len(p.buf) {
		switch p.buf[p.pos] {
		case '.', 'e', 'E':
			return jvalue{}, refuse(ReasonEntryNoninteger)
		}
	}
	if neg && len(digits) == 1 && digits[0] == '0' {
		return jvalue{}, refuse(ReasonEntryNegativeZero)
	}
	if len(digits) > 16 {
		return jvalue{}, refuse(ReasonEntryNumberRange)
	}
	var n int64
	for _, d := range digits {
		n = n*10 + int64(d-'0')
	}
	if n > maxSafeInt {
		return jvalue{}, refuse(ReasonEntryNumberRange)
	}
	if neg {
		n = -n
	}
	return jvalue{kind: kindInt, num: n}, nil
}
