package fragment

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
)

// Canonical returns the CCJ-1 bytes (curator-spec registry.md §1) of a
// parsed value. The rejection rules of §1 — duplicate keys, invalid UTF-8,
// non-integer numbers, negative zero, out-of-range integers, lone
// surrogates — are enforced by ParseJSON before a Value exists, so every
// Value is canonicalizable. The emission rules, in the order §1 lists them:
//
//  1. the top-level member "sig" is removed; nested "sig" members remain
//     (a fragment never carries one — Parse rejects it as unknown — but
//     Canonical is the general §1 function);
//  2. object keys are sorted by Unicode scalar value, which for valid UTF-8
//     is byte order;
//  3. no insignificant whitespace;
//  4. array order is preserved;
//  5. strings are UTF-8 with only ", \, \b, \f, \n, \r, \t escaped and the
//     remaining U+0000..U+001F as lowercase \u00xx;
//  6. /, <, >, &, and non-ASCII (U+2028 and U+2029 included) are emitted raw;
//  7. integers in shortest base-10 form, zero as 0;
//  8. literals as true, false, null.
func Canonical(v Value) []byte {
	if v.Kind == KindObject {
		members := make([]Member, 0, len(v.Obj))
		for _, m := range v.Obj {
			if m.Key != "sig" {
				members = append(members, m)
			}
		}
		v = Value{Kind: KindObject, Obj: members}
	}
	return appendCanonical(nil, v)
}

func appendCanonical(b []byte, v Value) []byte {
	switch v.Kind {
	case KindNull:
		return append(b, "null"...)
	case KindBool:
		if v.Bool {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case KindInt:
		return strconv.AppendInt(b, v.Int, 10)
	case KindString:
		return appendString(b, v.Str)
	case KindArray:
		b = append(b, '[')
		for i, el := range v.Arr {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendCanonical(b, el)
		}
		return append(b, ']')
	case KindObject:
		keys := make([]string, 0, len(v.Obj))
		byKey := make(map[string]Value, len(v.Obj))
		for _, m := range v.Obj {
			keys = append(keys, m.Key)
			byKey[m.Key] = m.Value
		}
		sort.Strings(keys)
		b = append(b, '{')
		for i, k := range keys {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendString(b, k)
			b = append(b, ':')
			b = appendCanonical(b, byKey[k])
		}
		return append(b, '}')
	}
	panic("fragment: unknown value kind")
}

const hexDigits = "0123456789abcdef"

func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\b':
			b = append(b, '\\', 'b')
		case '\f':
			b = append(b, '\\', 'f')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			if c < 0x20 {
				b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF])
			} else {
				b = append(b, c)
			}
		}
	}
	return append(b, '"')
}

// DigestPrefix is the prefix of every fragment digest (SPEC §4.1).
const DigestPrefix = "sha256:"

// Digest returns "sha256:<64 lowercase hex>" over the CCJ-1 bytes of v
// (SPEC §4.1; Decision 0013 D6.4). Environments.md §10.1 prints the
// fragment as exactly these bytes plus one LF, so the digest is comparable
// with one computed over Curator's printed line without its LF — but it is
// computed here from the parsed object, so a printer change is not drift.
func Digest(v Value) string {
	sum := sha256.Sum256(Canonical(v))
	return DigestPrefix + hex.EncodeToString(sum[:])
}
