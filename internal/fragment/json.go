package fragment

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// This file is the strict JSON reader behind Parse and Canonical. The
// standard encoding/json decoder is deliberately not used for the fragment:
// it accepts duplicate object keys (last wins), repairs invalid UTF-8 and
// lone surrogates into U+FFFD instead of rejecting them, and reads numbers
// through float64 — every one of which CCJ-1 (registry.md §1) requires a
// reader to reject before canonicalization. The reader below implements
// RFC 8259 JSON narrowed by those rules and produces an ordered tree, so the
// digest is computed from the parsed object (SPEC §4.1) and never from the
// bytes Curator printed.

// Kind is the JSON value kind of a Value.
type Kind int

const (
	// KindNull is the JSON literal null.
	KindNull Kind = iota
	// KindBool is true or false.
	KindBool
	// KindInt is a CCJ-1 integer: no fraction, no exponent, no negative
	// zero, within ±(2^53-1).
	KindInt
	// KindString is a JSON string holding valid UTF-8.
	KindString
	// KindArray is a JSON array.
	KindArray
	// KindObject is a JSON object with unique keys in source order.
	KindObject
)

// String names the kind for diagnostics.
func (k Kind) String() string {
	switch k {
	case KindNull:
		return "null"
	case KindBool:
		return "boolean"
	case KindInt:
		return "integer"
	case KindString:
		return "string"
	case KindArray:
		return "array"
	case KindObject:
		return "object"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Value is one parsed JSON value. Exactly the members for its Kind are
// meaningful; the rest are zero.
type Value struct {
	Kind Kind
	Bool bool
	Int  int64
	Str  string
	// Arr holds array elements in order.
	Arr []Value
	// Obj holds object members in source order with unique keys.
	Obj []Member
}

// Member is one object member.
type Member struct {
	Key   string
	Value Value
}

// Get returns the member named key of an object value and whether it exists.
func (v Value) Get(key string) (Value, bool) {
	for _, m := range v.Obj {
		if m.Key == key {
			return m.Value, true
		}
	}
	return Value{}, false
}

// maxDepth bounds nesting so a hostile document cannot exhaust the stack.
const maxDepth = 64

// maxSafeInt is the CCJ-1 integer bound, 2^53-1.
const maxSafeInt = 9007199254740991

// SyntaxError reports where and why the input is not CCJ-1-admissible JSON.
type SyntaxError struct {
	Offset int
	Msg    string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("json: %s at byte %d", e.Msg, e.Offset)
}

// ErrInvalidUTF8 is the CCJ-1 pre-canonicalization rejection of input that
// is not valid UTF-8.
var ErrInvalidUTF8 = errors.New("json: input is not valid UTF-8")

// ParseJSON reads exactly one JSON value from data under the CCJ-1 reader
// rules: valid UTF-8 only, unique object keys, integers only (no fraction,
// exponent, or negative zero; magnitude at most 2^53-1), no lone surrogate
// escapes, no raw control characters in strings, and nothing but JSON
// whitespace after the value. Any violation is an error; nothing is
// repaired.
func ParseJSON(data []byte) (Value, error) {
	if !utf8.Valid(data) {
		return Value{}, ErrInvalidUTF8
	}
	p := &parser{data: data}
	p.skipSpace()
	v, err := p.value(0)
	if err != nil {
		return Value{}, err
	}
	p.skipSpace()
	if p.pos != len(p.data) {
		return Value{}, p.errf("trailing content after the JSON value")
	}
	return v, nil
}

type parser struct {
	data []byte
	pos  int
}

func (p *parser) errf(format string, a ...any) error {
	return &SyntaxError{Offset: p.pos, Msg: fmt.Sprintf(format, a...)}
}

func (p *parser) skipSpace() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (Value, error) {
	if depth >= maxDepth {
		return Value{}, p.errf("nesting deeper than %d", maxDepth)
	}
	if p.pos >= len(p.data) {
		return Value{}, p.errf("unexpected end of input")
	}
	switch c := p.data[p.pos]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		s, err := p.str()
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindString, Str: s}, nil
	case c == 't':
		return p.literal("true", Value{Kind: KindBool, Bool: true})
	case c == 'f':
		return p.literal("false", Value{Kind: KindBool})
	case c == 'n':
		return p.literal("null", Value{Kind: KindNull})
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	default:
		return Value{}, p.errf("unexpected byte %q", c)
	}
}

func (p *parser) literal(word string, v Value) (Value, error) {
	if len(p.data)-p.pos < len(word) || string(p.data[p.pos:p.pos+len(word)]) != word {
		return Value{}, p.errf("invalid literal")
	}
	p.pos += len(word)
	return v, nil
}

func (p *parser) object(depth int) (Value, error) {
	p.pos++ // '{'
	v := Value{Kind: KindObject, Obj: []Member{}}
	p.skipSpace()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return v, nil
	}
	seen := map[string]bool{}
	for {
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return Value{}, p.errf("expected object key")
		}
		key, err := p.str()
		if err != nil {
			return Value{}, err
		}
		if seen[key] {
			return Value{}, p.errf("duplicate object key %q", key)
		}
		seen[key] = true
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return Value{}, p.errf("expected ':' after object key")
		}
		p.pos++
		p.skipSpace()
		val, err := p.value(depth + 1)
		if err != nil {
			return Value{}, err
		}
		v.Obj = append(v.Obj, Member{Key: key, Value: val})
		p.skipSpace()
		if p.pos >= len(p.data) {
			return Value{}, p.errf("unexpected end of input in object")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return v, nil
		default:
			return Value{}, p.errf("expected ',' or '}' in object")
		}
	}
}

func (p *parser) array(depth int) (Value, error) {
	p.pos++ // '['
	v := Value{Kind: KindArray, Arr: []Value{}}
	p.skipSpace()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return v, nil
	}
	for {
		p.skipSpace()
		el, err := p.value(depth + 1)
		if err != nil {
			return Value{}, err
		}
		v.Arr = append(v.Arr, el)
		p.skipSpace()
		if p.pos >= len(p.data) {
			return Value{}, p.errf("unexpected end of input in array")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return v, nil
		default:
			return Value{}, p.errf("expected ',' or ']' in array")
		}
	}
}

// number reads a JSON number and admits only CCJ-1 integers.
func (p *parser) number() (Value, error) {
	start := p.pos
	neg := false
	if p.data[p.pos] == '-' {
		neg = true
		p.pos++
	}
	if p.pos >= len(p.data) || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
		return Value{}, p.errf("invalid number")
	}
	if p.data[p.pos] == '0' {
		p.pos++
		if p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			return Value{}, p.errf("leading zero in number")
		}
	} else {
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.data) {
		switch p.data[p.pos] {
		case '.', 'e', 'E':
			return Value{}, p.errf("non-integer number")
		}
	}
	digits := p.data[start:p.pos]
	if neg {
		digits = digits[1:]
	}
	if neg && len(digits) == 1 && digits[0] == '0' {
		return Value{}, p.errf("negative zero")
	}
	// 16 digits is the longest length that can still be within 2^53-1.
	if len(digits) > 16 {
		return Value{}, p.errf("integer outside the CCJ-1 range")
	}
	var n int64
	for _, d := range digits {
		n = n*10 + int64(d-'0')
	}
	if n > maxSafeInt {
		return Value{}, p.errf("integer outside the CCJ-1 range")
	}
	if neg {
		n = -n
	}
	return Value{Kind: KindInt, Int: n}, nil
}

// str reads a JSON string starting at the opening quote. Raw control
// characters are rejected; \u escapes must form scalar values, with a high
// surrogate immediately followed by a low surrogate escape and a lone
// surrogate of either half rejected.
func (p *parser) str() (string, error) {
	p.pos++ // opening quote
	var out []byte
	for {
		if p.pos >= len(p.data) {
			return "", p.errf("unterminated string")
		}
		c := p.data[p.pos]
		switch {
		case c == '"':
			p.pos++
			return string(out), nil
		case c < 0x20:
			return "", p.errf("raw control character in string")
		case c == '\\':
			p.pos++
			if p.pos >= len(p.data) {
				return "", p.errf("unterminated escape")
			}
			e := p.data[p.pos]
			p.pos++
			switch e {
			case '"', '\\', '/':
				out = append(out, e)
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				r, err := p.hex4()
				if err != nil {
					return "", err
				}
				switch {
				case r >= 0xD800 && r <= 0xDBFF:
					if p.pos+1 >= len(p.data) || p.data[p.pos] != '\\' || p.data[p.pos+1] != 'u' {
						return "", p.errf("lone high surrogate escape")
					}
					p.pos += 2
					lo, err := p.hex4()
					if err != nil {
						return "", err
					}
					if lo < 0xDC00 || lo > 0xDFFF {
						return "", p.errf("high surrogate escape not followed by a low surrogate")
					}
					r = 0x10000 + (r-0xD800)<<10 + (lo - 0xDC00)
				case r >= 0xDC00 && r <= 0xDFFF:
					return "", p.errf("lone low surrogate escape")
				}
				out = utf8.AppendRune(out, r)
			default:
				return "", p.errf("invalid escape \\%c", e)
			}
		default:
			// Input is already known to be valid UTF-8; copy the byte.
			out = append(out, c)
			p.pos++
		}
	}
}

func (p *parser) hex4() (rune, error) {
	if len(p.data)-p.pos < 4 {
		return 0, p.errf("truncated \\u escape")
	}
	var r rune
	for i := 0; i < 4; i++ {
		c := p.data[p.pos+i]
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, p.errf("invalid hex digit in \\u escape")
		}
		r = r<<4 | rune(d)
	}
	p.pos += 4
	return r, nil
}
