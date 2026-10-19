package managerschema

import (
	"sort"
	"strconv"
)

// canonical returns the CCJ-1 bytes (curator-spec registry.md §1) of a
// parsed value. The rejection rules of §1 — duplicate keys, invalid UTF-8,
// non-integer numbers, negative zero, out-of-range integers, lone
// surrogates — are enforced by parseJSON before a jsonValue exists, so every
// jsonValue is canonicalizable. The emission rules, in the order §1 lists them:
//
//  1. the top-level member "sig" is removed; nested "sig" members remain
//     (a fragment never carries one — Parse rejects it as unknown — but
//     canonical is the general §1 function);
//  2. object keys are sorted by Unicode scalar value, which for valid UTF-8
//     is byte order;
//  3. no insignificant whitespace;
//  4. array order is preserved;
//  5. strings are UTF-8 with only ", \, \b, \f, \n, \r, \t escaped and the
//     remaining U+0000..U+001F as lowercase \u00xx;
//  6. /, <, >, &, and non-ASCII (U+2028 and U+2029 included) are emitted raw;
//  7. integers in shortest base-10 form, zero as 0;
//  8. literals as true, false, null.
func canonical(v jsonValue) []byte {
	if v.kind == kindObject {
		members := make([]jsonMember, 0, len(v.Obj))
		for _, m := range v.Obj {
			if m.Key != "sig" {
				members = append(members, m)
			}
		}
		v = jsonValue{kind: kindObject, Obj: members}
	}
	return appendCanonical(nil, v)
}

func appendCanonical(b []byte, v jsonValue) []byte {
	switch v.kind {
	case kindNull:
		return append(b, "null"...)
	case kindBool:
		if v.Bool {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case kindInt:
		return strconv.AppendInt(b, v.Int, 10)
	case kindString:
		return appendString(b, v.Str)
	case kindArray:
		b = append(b, '[')
		for i, el := range v.Arr {
			if i > 0 {
				b = append(b, ',')
			}
			b = appendCanonical(b, el)
		}
		return append(b, ']')
	case kindObject:
		keys := make([]string, 0, len(v.Obj))
		byKey := make(map[string]jsonValue, len(v.Obj))
		for _, m := range v.Obj {
			keys = append(keys, m.Key)
			byKey[m.Key] = m.jsonValue
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

// CanonicalJSON reads strict CCJ-1 JSON and returns its canonical bytes.
// Duplicate keys, invalid UTF-8, lone surrogates, fractions, exponents, negative
// zero, unsafe integers and trailing documents refuse before canonicalization.
// As in CCJ-1, a top-level sig member is omitted; nested sig members remain.
// This reader has no protocol byte bound; Validate applies Limit separately.
func CanonicalJSON(data []byte) ([]byte, error) {
	v, err := parseJSON(data)
	if err != nil {
		return nil, err
	}
	return canonical(v), nil
}
