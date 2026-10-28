package memberwire

import (
	"bytes"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const safeInteger uint64 = 9007199254740991

type token struct {
	kind   byte
	text   string
	number uint64
}
type faults struct{ bounds, schema bool }

// Tokenization and iterative grammar validation precede AST construction. Their
// storage is bounded by the already capped raw input, even for excessive depth
// or node counts. No recursive descent or decoded-value tree is built until
// the inclusive depth16/node8192/string4096 limits and grammar have passed.
func parseObject(raw []byte) (map[string]any, *Error) {
	if len(raw) > RequestLimit {
		return nil, parseError("bounds_exceeded")
	}
	if !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) || bytes.IndexByte(raw, 0) >= 0 {
		return nil, parseError("encoding_invalid")
	}
	tokens, f, ok := tokenize(raw)
	if !ok {
		return nil, parseError("encoding_invalid")
	}
	if !grammar(tokens, &f) {
		return nil, parseError("encoding_invalid")
	}
	if f.bounds {
		return nil, parseError("bounds_exceeded")
	}
	if f.schema {
		return nil, parseError("schema_invalid")
	}
	i := 0
	return buildValue(tokens, &i).(map[string]any), nil
}
func whitespace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }
func delimiter(c byte) bool {
	return whitespace(c) || strings.ContainsRune("{}[]:,", rune(c)) || c == '/'
}
func tokenize(raw []byte) ([]token, faults, bool) {
	var out []token
	var f faults
	for i := 0; i < len(raw); {
		c := raw[i]
		if whitespace(c) {
			i++
			continue
		}
		if strings.ContainsRune("{}[]:,", rune(c)) {
			out = append(out, token{kind: c})
			i++
			continue
		}
		if c == '"' {
			s, end, ok := stringToken(raw, i)
			if !ok {
				return nil, f, false
			}
			if len(s) > 4096 {
				f.bounds = true
			}
			out = append(out, token{kind: 's', text: s})
			i = end
			continue
		}
		// Comments are forbidden schema extensions, not accepted whitespace.
		// Scanning them permits encoding/grammar failures to retain phase priority.
		if c == '/' && i+1 < len(raw) {
			if raw[i+1] == '/' {
				f.schema = true
				i += 2
				for i < len(raw) && raw[i] != '\n' {
					i++
				}
				continue
			}
			if raw[i+1] == '*' {
				end := bytes.Index(raw[i+2:], []byte("*/"))
				if end < 0 {
					return nil, f, false
				}
				f.schema = true
				i += end + 4
				continue
			}
		}
		start := i
		for i < len(raw) && !delimiter(raw[i]) {
			i++
		}
		if start == i {
			return nil, f, false
		}
		s := string(raw[start:i])
		switch s {
		case "true":
			out = append(out, token{kind: 't'})
		case "false":
			out = append(out, token{kind: 'f'})
		case "null":
			out = append(out, token{kind: 'z'})
		default:
			if !(c >= '0' && c <= '9' || c == '-' || c == '+' || c == '.' || s == "NaN" || s == "Infinity") {
				return nil, f, false
			}
			n, lexical, overflow := numberToken(s)
			f.schema = f.schema || !lexical
			f.bounds = f.bounds || overflow
			out = append(out, token{kind: 'n', number: n})
		}
	}
	return out, f, true
}
func numberToken(s string) (uint64, bool, bool) {
	if len(s) == 0 || len(s) > 1 && s[0] == '0' {
		return 0, false, false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return 0, false, false
		}
	}
	var n uint64
	for i := range len(s) {
		d := uint64(s[i] - '0')
		if n > (safeInteger-d)/10 {
			return 0, true, true
		}
		n = n*10 + d
	}
	return n, true, false
}
func hexQuad(raw []byte, i int) (uint16, bool) {
	if i+4 > len(raw) {
		return 0, false
	}
	var n uint16
	for _, c := range raw[i : i+4] {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += uint16(c - '0')
		case c >= 'a' && c <= 'f':
			n += uint16(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			n += uint16(c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return n, true
}
func stringToken(raw []byte, start int) (string, int, bool) {
	var b strings.Builder
	for i := start + 1; i < len(raw); {
		c := raw[i]
		i++
		if c == '"' {
			return b.String(), i, true
		}
		if c < 0x20 {
			return "", 0, false
		}
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		if i >= len(raw) {
			return "", 0, false
		}
		c = raw[i]
		i++
		switch c {
		case '"', '\\', '/':
			b.WriteByte(c)
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			u, ok := hexQuad(raw, i)
			if !ok || u == 0 {
				return "", 0, false
			}
			i += 4
			r := rune(u)
			if u >= 0xd800 && u <= 0xdbff {
				if i+6 > len(raw) || raw[i] != '\\' || raw[i+1] != 'u' {
					return "", 0, false
				}
				low, ok := hexQuad(raw, i+2)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return "", 0, false
				}
				i += 6
				r = utf16.DecodeRune(r, rune(low))
			} else if u >= 0xdc00 && u <= 0xdfff {
				return "", 0, false
			}
			b.WriteRune(r)
		default:
			return "", 0, false
		}
	}
	return "", 0, false
}

type frame struct {
	kind, state byte
	names       map[string]bool
}

func grammar(tokens []token, f *faults) bool {
	if len(tokens) == 0 || tokens[0].kind != '{' {
		return false
	}
	stack := []frame{{kind: '{', names: make(map[string]bool)}}
	nodes, i := 1, 1 // The required root is one value node at container depth1.
	for len(stack) > 0 {
		if i >= len(tokens) {
			return false
		}
		t := tokens[i]
		top := &stack[len(stack)-1]
		switch top.state {
		case 0, 4: // Initial key/value or key/value after a comma.
			if top.kind == '{' {
				if t.kind == '}' && top.state == 0 {
					stack = stack[:len(stack)-1]
					i++
					continue
				}
				if t.kind != 's' {
					return false
				}
				if top.names[t.text] {
					f.schema = true
				}
				top.names[t.text] = true
				top.state = 1
				i++
				continue
			}
			if t.kind == ']' && top.state == 0 {
				stack = stack[:len(stack)-1]
				i++
				continue
			}
			top.state = 2 // Array value falls through to the common value step below.
		case 1:
			if t.kind != ':' {
				return false
			}
			top.state = 2
			i++
			continue
		case 3:
			if top.kind == '{' && t.kind == '}' || top.kind == '[' && t.kind == ']' {
				stack = stack[:len(stack)-1]
				i++
				continue
			}
			if t.kind != ',' {
				return false
			}
			top.state = 4
			i++
			continue
		}
		// Only values count as nodes; object member names were consumed above.
		top.state = 3
		i++
		nodes++
		switch t.kind {
		case '{', '[':
			child := frame{kind: t.kind}
			if t.kind == '{' {
				child.names = make(map[string]bool)
			}
			stack = append(stack, child)
			if len(stack) > 16 {
				f.bounds = true
			}
		case 's', 'n', 't', 'f', 'z':
		default:
			return false
		}
		if nodes > 8192 {
			f.bounds = true
		}
	}
	return i == len(tokens)
}

// Called only after grammar and all lexical bounds pass (depth at most16).
func buildValue(tokens []token, i *int) any {
	t := tokens[*i]
	*i++
	switch t.kind {
	case '{':
		o := make(map[string]any)
		for tokens[*i].kind != '}' {
			key := tokens[*i].text
			*i += 2
			o[key] = buildValue(tokens, i)
			if tokens[*i].kind == ',' {
				*i++
			}
		}
		*i++
		return o
	case '[':
		a := make([]any, 0)
		for tokens[*i].kind != ']' {
			a = append(a, buildValue(tokens, i))
			if tokens[*i].kind == ',' {
				*i++
			}
		}
		*i++
		return a
	case 's':
		return t.text
	case 'n':
		return t.number
	case 't':
		return true
	case 'f':
		return false
	default:
		return nil
	}
}
