package managerschema

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// Copied from internal/fragment/json.go; launcher alignment tests guard drift.
// This file is the strict JSON reader behind CanonicalJSON. The
// standard encoding/json decoder is deliberately not used for the fragment:
// it accepts duplicate object keys (last wins), repairs invalid UTF-8 and
// lone surrogates into U+FFFD instead of rejecting them, and reads numbers
// through float64 — every one of which CCJ-1 (registry.md §1) requires a
// reader to reject before canonicalization. The reader below implements
// RFC 8259 JSON narrowed by those rules and produces an ordered tree, so the
// digest is computed from the parsed object (SPEC §4.1) and never from the
// bytes Curator printed.

// jsonKind is the JSON value kind of a jsonValue.
type jsonKind int

const (
	// kindNull is the JSON literal null.
	kindNull jsonKind = iota
	// kindBool is true or false.
	kindBool
	// kindInt is a CCJ-1 integer: no fraction, no exponent, no negative
	// zero, within ±(2^53-1).
	kindInt
	// kindString is a JSON string holding valid UTF-8.
	kindString
	// kindArray is a JSON array.
	kindArray
	// kindObject is a JSON object with unique keys in source order.
	kindObject
)

// String names the kind for diagnostics.
func (k jsonKind) String() string {
	switch k {
	case kindNull:
		return "null"
	case kindBool:
		return "boolean"
	case kindInt:
		return "integer"
	case kindString:
		return "string"
	case kindArray:
		return "array"
	case kindObject:
		return "object"
	}
	return fmt.Sprintf("jsonKind(%d)", int(k))
}

// jsonValue is one parsed JSON value. Exactly the members for its jsonKind are
// meaningful; the rest are zero.
type jsonValue struct {
	kind jsonKind
	Bool bool
	Int  int64
	Str  string
	// Arr holds array elements in order.
	Arr []jsonValue
	// Obj holds object members in source order with unique keys.
	Obj []jsonMember
}

// jsonMember is one object member.
type jsonMember struct {
	Key       string
	jsonValue jsonValue
}

// Get returns the member named key of an object value and whether it exists.
func (v jsonValue) Get(key string) (jsonValue, bool) {
	for _, m := range v.Obj {
		if m.Key == key {
			return m.jsonValue, true
		}
	}
	return jsonValue{}, false
}

// maxDepth bounds nesting so a hostile document cannot exhaust the stack.
const maxDepth = 64

// maxSafeInt is the CCJ-1 integer bound, 2^53-1.
const maxSafeInt = 9007199254740991

// syntaxError reports where and why the input is not CCJ-1-admissible JSON.
type syntaxError struct {
	Offset int
	Msg    string
}

func (e *syntaxError) Error() string {
	return fmt.Sprintf("json: %s at byte %d", e.Msg, e.Offset)
}

// errInvalidUTF8 is the CCJ-1 pre-canonicalization rejection of input that
// is not valid UTF-8.
var errInvalidUTF8 = errors.New("json: input is not valid UTF-8")

// parseJSON reads exactly one JSON value from data under the CCJ-1 reader
// rules: valid UTF-8 only, unique object keys, integers only (no fraction,
// exponent, or negative zero; magnitude at most 2^53-1), no lone surrogate
// escapes, no raw control characters in strings, and nothing but JSON
// whitespace after the value. Any violation is an error; nothing is
// repaired.
func parseJSON(data []byte) (jsonValue, error) {
	if !utf8.Valid(data) {
		return jsonValue{}, errInvalidUTF8
	}
	p := &parser{data: data}
	p.skipSpace()
	v, err := p.value(0)
	if err != nil {
		return jsonValue{}, err
	}
	p.skipSpace()
	if p.pos != len(p.data) {
		return jsonValue{}, p.errf("trailing content after the JSON value")
	}
	return v, nil
}

type parser struct {
	data []byte
	pos  int
}

func (p *parser) errf(format string, a ...any) error {
	return &syntaxError{Offset: p.pos, Msg: fmt.Sprintf(format, a...)}
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

func (p *parser) value(depth int) (jsonValue, error) {
	if depth >= maxDepth {
		return jsonValue{}, p.errf("nesting deeper than %d", maxDepth)
	}
	if p.pos >= len(p.data) {
		return jsonValue{}, p.errf("unexpected end of input")
	}
	switch c := p.data[p.pos]; {
	case c == '{':
		return p.object(depth)
	case c == '[':
		return p.array(depth)
	case c == '"':
		s, err := p.str()
		if err != nil {
			return jsonValue{}, err
		}
		return jsonValue{kind: kindString, Str: s}, nil
	case c == 't':
		return p.literal("true", jsonValue{kind: kindBool, Bool: true})
	case c == 'f':
		return p.literal("false", jsonValue{kind: kindBool})
	case c == 'n':
		return p.literal("null", jsonValue{kind: kindNull})
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	default:
		return jsonValue{}, p.errf("unexpected byte %q", c)
	}
}

func (p *parser) literal(word string, v jsonValue) (jsonValue, error) {
	if len(p.data)-p.pos < len(word) || string(p.data[p.pos:p.pos+len(word)]) != word {
		return jsonValue{}, p.errf("invalid literal")
	}
	p.pos += len(word)
	return v, nil
}

func (p *parser) object(depth int) (jsonValue, error) {
	p.pos++ // '{'
	v := jsonValue{kind: kindObject, Obj: []jsonMember{}}
	p.skipSpace()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return v, nil
	}
	seen := map[string]bool{}
	for {
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return jsonValue{}, p.errf("expected object key")
		}
		key, err := p.str()
		if err != nil {
			return jsonValue{}, err
		}
		if seen[key] {
			return jsonValue{}, p.errf("duplicate object key %q", key)
		}
		seen[key] = true
		p.skipSpace()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return jsonValue{}, p.errf("expected ':' after object key")
		}
		p.pos++
		p.skipSpace()
		val, err := p.value(depth + 1)
		if err != nil {
			return jsonValue{}, err
		}
		v.Obj = append(v.Obj, jsonMember{Key: key, jsonValue: val})
		p.skipSpace()
		if p.pos >= len(p.data) {
			return jsonValue{}, p.errf("unexpected end of input in object")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return v, nil
		default:
			return jsonValue{}, p.errf("expected ',' or '}' in object")
		}
	}
}

func (p *parser) array(depth int) (jsonValue, error) {
	p.pos++ // '['
	v := jsonValue{kind: kindArray, Arr: []jsonValue{}}
	p.skipSpace()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return v, nil
	}
	for {
		p.skipSpace()
		el, err := p.value(depth + 1)
		if err != nil {
			return jsonValue{}, err
		}
		v.Arr = append(v.Arr, el)
		p.skipSpace()
		if p.pos >= len(p.data) {
			return jsonValue{}, p.errf("unexpected end of input in array")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return v, nil
		default:
			return jsonValue{}, p.errf("expected ',' or ']' in array")
		}
	}
}

// number reads a JSON number and admits only CCJ-1 integers.
func (p *parser) number() (jsonValue, error) {
	start := p.pos
	neg := false
	if p.data[p.pos] == '-' {
		neg = true
		p.pos++
	}
	if p.pos >= len(p.data) || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
		return jsonValue{}, p.errf("invalid number")
	}
	if p.data[p.pos] == '0' {
		p.pos++
		if p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			return jsonValue{}, p.errf("leading zero in number")
		}
	} else {
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.pos < len(p.data) {
		switch p.data[p.pos] {
		case '.', 'e', 'E':
			return jsonValue{}, p.errf("non-integer number")
		}
	}
	digits := p.data[start:p.pos]
	if neg {
		digits = digits[1:]
	}
	if neg && len(digits) == 1 && digits[0] == '0' {
		return jsonValue{}, p.errf("negative zero")
	}
	// 16 digits is the longest length that can still be within 2^53-1.
	if len(digits) > 16 {
		return jsonValue{}, p.errf("integer outside the CCJ-1 range")
	}
	var n int64
	for _, d := range digits {
		n = n*10 + int64(d-'0')
	}
	if n > maxSafeInt {
		return jsonValue{}, p.errf("integer outside the CCJ-1 range")
	}
	if neg {
		n = -n
	}
	return jsonValue{kind: kindInt, Int: n}, nil
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
